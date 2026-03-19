package graph

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	bolt "go.etcd.io/bbolt"
)

// ── Bucket names for indexes and constraints ──────────────────────

var (
	bucketPropIndex   = []byte("prop_index")   // "Label:Property:Value" -> []nodeID
	bucketConstraints = []byte("constraints")  // "Label:Property" -> Constraint JSON
	bucketIndexDefs   = []byte("index_defs")   // "Label:Property" -> IndexDef JSON
)

// ── Index Types ──────────────────────────────────────────────────

// IndexDef describes a secondary property index.
type IndexDef struct {
	Label    string    `json:"label"`
	Property string    `json:"property"`
	Created  time.Time `json:"created"`
	Count    int       `json:"count"` // approximate indexed entry count
}

// ConstraintType defines the kind of constraint.
type ConstraintType string

const (
	ConstraintUnique  ConstraintType = "UNIQUE"
	ConstraintNotNull ConstraintType = "NOT_NULL"
	ConstraintExists  ConstraintType = "EXISTS"
)

// Constraint defines a schema constraint on a node property.
type Constraint struct {
	Label    string         `json:"label"`
	Property string         `json:"property"`
	Type     ConstraintType `json:"type"`
	Created  time.Time      `json:"created"`
}

// SchemaInfo holds the full schema of the graph database.
type SchemaInfo struct {
	Labels        []string         `json:"labels"`
	RelTypes      []string         `json:"relationship_types"`
	PropertyKeys  []string         `json:"property_keys"`
	Indexes       []IndexDef       `json:"indexes"`
	Constraints   []Constraint     `json:"constraints"`
	NodeCount     int              `json:"node_count"`
	EdgeCount     int              `json:"edge_count"`
	LabelCounts   map[string]int   `json:"label_counts"`
	RelTypeCounts map[string]int   `json:"rel_type_counts"`
}

// ChangeEvent represents a CDC (Change Data Capture) event.
type ChangeEvent struct {
	Type      string                 `json:"type"`       // "node_created", "node_updated", "node_deleted", "edge_created", "edge_deleted"
	Timestamp time.Time              `json:"timestamp"`
	NodeID    uint64                 `json:"node_id,omitempty"`
	EdgeID    uint64                 `json:"edge_id,omitempty"`
	Labels    []string               `json:"labels,omitempty"`
	Props     map[string]interface{} `json:"props,omitempty"`
}

// ChangeListener is a callback for CDC events.
type ChangeListener func(event ChangeEvent)

// changeListeners holds registered CDC listeners.
var (
	changeListeners   []ChangeListener
	changeListenersMu sync.RWMutex
)

// ── Initialize Buckets ───────────────────────────────────────────

// EnsureIndexBuckets creates the index and constraint buckets if they
// don't already exist. Called on store initialization.
func (s *Store) EnsureIndexBuckets() error {
	return s.db.Update(func(tx *bolt.Tx) error {
		for _, bucket := range [][]byte{bucketPropIndex, bucketConstraints, bucketIndexDefs} {
			if _, err := tx.CreateBucketIfNotExists(bucket); err != nil {
				return fmt.Errorf("create bucket %s: %w", string(bucket), err)
			}
		}
		return nil
	})
}

// ── Property Indexes ─────────────────────────────────────────────

// CreateIndex builds a secondary index on a node property for a given label.
//
// What: Creates an index mapping property values to node IDs.
// Why:  Turns O(N) scans into O(1) lookups for property-based queries.
// How:  Scans all nodes with the label, extracts the property value,
//       and stores a value -> []nodeID mapping in the prop_index bucket.
func (s *Store) CreateIndex(label, property string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.db.Update(func(tx *bolt.Tx) error {
		propIdx := tx.Bucket(bucketPropIndex)
		if propIdx == nil {
			var err error
			propIdx, err = tx.CreateBucketIfNotExists(bucketPropIndex)
			if err != nil {
				return err
			}
		}
		idxDefs := tx.Bucket(bucketIndexDefs)
		if idxDefs == nil {
			var err error
			idxDefs, err = tx.CreateBucketIfNotExists(bucketIndexDefs)
			if err != nil {
				return err
			}
		}

		// Scan all nodes with this label.
		nodeIdx := tx.Bucket(bucketNodeIndex)
		idsData := nodeIdx.Get([]byte(label))
		if idsData == nil {
			// No nodes with this label yet; just register the index.
			def := IndexDef{Label: label, Property: property, Created: time.Now(), Count: 0}
			defData, _ := json.Marshal(def)
			return idxDefs.Put([]byte(label+":"+property), defData)
		}

		var nodeIDs []uint64
		json.Unmarshal(idsData, &nodeIDs)

		// Build index entries.
		nodesBucket := tx.Bucket(bucketNodes)
		count := 0
		for _, nid := range nodeIDs {
			raw := nodesBucket.Get(itob(nid))
			if raw == nil {
				continue
			}
			var node Node
			if err := json.Unmarshal(raw, &node); err != nil {
				continue
			}
			val, ok := node.Properties[property]
			if !ok {
				continue
			}
			key := indexKey(label, property, val)
			existing := propIdx.Get(key)
			var ids []uint64
			if existing != nil {
				json.Unmarshal(existing, &ids)
			}
			ids = append(ids, nid)
			idsJSON, _ := json.Marshal(ids)
			propIdx.Put(key, idsJSON)
			count++
		}

		// Register index definition.
		def := IndexDef{Label: label, Property: property, Created: time.Now(), Count: count}
		defData, _ := json.Marshal(def)
		return idxDefs.Put([]byte(label+":"+property), defData)
	})
}

// DropIndex removes a secondary property index.
func (s *Store) DropIndex(label, property string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.db.Update(func(tx *bolt.Tx) error {
		idxDefs := tx.Bucket(bucketIndexDefs)
		if idxDefs == nil {
			return nil
		}
		idxDefs.Delete([]byte(label + ":" + property))

		// Remove all index entries with this prefix.
		propIdx := tx.Bucket(bucketPropIndex)
		if propIdx == nil {
			return nil
		}
		prefix := []byte(label + ":" + property + ":")
		c := propIdx.Cursor()
		for k, _ := c.Seek(prefix); k != nil && len(k) >= len(prefix) && string(k[:len(prefix)]) == string(prefix); k, _ = c.Next() {
			propIdx.Delete(k)
		}
		return nil
	})
}

// ListIndexes returns all registered indexes.
func (s *Store) ListIndexes() ([]IndexDef, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var indexes []IndexDef
	err := s.db.View(func(tx *bolt.Tx) error {
		idxDefs := tx.Bucket(bucketIndexDefs)
		if idxDefs == nil {
			return nil
		}
		return idxDefs.ForEach(func(k, v []byte) error {
			var def IndexDef
			if err := json.Unmarshal(v, &def); err == nil {
				indexes = append(indexes, def)
			}
			return nil
		})
	})
	return indexes, err
}

// FindByIndex performs an O(1) lookup using a property index.
//
// What: Retrieves nodes matching a specific property value via the index.
// Why:  Avoids full scans. Critical for production performance.
func (s *Store) FindByIndex(label, property string, value interface{}) ([]Node, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var nodes []Node
	err := s.db.View(func(tx *bolt.Tx) error {
		propIdx := tx.Bucket(bucketPropIndex)
		if propIdx == nil {
			return fmt.Errorf("no property indexes exist")
		}
		key := indexKey(label, property, value)
		data := propIdx.Get(key)
		if data == nil {
			return nil // No matches.
		}
		var nodeIDs []uint64
		if err := json.Unmarshal(data, &nodeIDs); err != nil {
			return err
		}
		nodesBucket := tx.Bucket(bucketNodes)
		for _, nid := range nodeIDs {
			raw := nodesBucket.Get(itob(nid))
			if raw != nil {
				var n Node
				if err := json.Unmarshal(raw, &n); err == nil {
					nodes = append(nodes, n)
				}
			}
		}
		return nil
	})
	return nodes, err
}

// NodesByProperty finds nodes by any property value (full scan, no index required).
func (s *Store) NodesByProperty(key string, value interface{}) ([]Node, error) {
	allNodes := s.AllNodes()
	valStr := fmt.Sprintf("%v", value)
	var matches []Node
	for _, n := range allNodes {
		if v, ok := n.Properties[key]; ok {
			if fmt.Sprintf("%v", v) == valStr {
				matches = append(matches, n)
			}
		}
	}
	return matches, nil
}

// ── Constraints ──────────────────────────────────────────────────

// CreateConstraint registers a schema constraint.
//
// What: Enforces data integrity rules on node properties.
// Why:  Prevents duplicate values (UNIQUE), missing properties (NOT_NULL),
//       or absent properties (EXISTS).
func (s *Store) CreateConstraint(label, property string, cType ConstraintType) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.db.Update(func(tx *bolt.Tx) error {
		constraints := tx.Bucket(bucketConstraints)
		if constraints == nil {
			var err error
			constraints, err = tx.CreateBucketIfNotExists(bucketConstraints)
			if err != nil {
				return err
			}
		}

		c := Constraint{
			Label:    label,
			Property: property,
			Type:     cType,
			Created:  time.Now(),
		}
		data, err := json.Marshal(c)
		if err != nil {
			return err
		}
		return constraints.Put([]byte(label+":"+property), data)
	})
}

// DropConstraint removes a schema constraint.
func (s *Store) DropConstraint(label, property string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.db.Update(func(tx *bolt.Tx) error {
		constraints := tx.Bucket(bucketConstraints)
		if constraints == nil {
			return nil
		}
		return constraints.Delete([]byte(label + ":" + property))
	})
}

// ListConstraints returns all registered constraints.
func (s *Store) ListConstraints() ([]Constraint, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var constraints []Constraint
	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketConstraints)
		if b == nil {
			return nil
		}
		return b.ForEach(func(k, v []byte) error {
			var c Constraint
			if err := json.Unmarshal(v, &c); err == nil {
				constraints = append(constraints, c)
			}
			return nil
		})
	})
	return constraints, err
}

// ValidateConstraints checks all existing data against registered constraints.
// Returns a list of violations as error strings.
func (s *Store) ValidateConstraints() ([]string, error) {
	constraints, err := s.ListConstraints()
	if err != nil {
		return nil, err
	}

	allNodes := s.AllNodes()
	var violations []string

	for _, c := range constraints {
		// Get nodes with this label.
		var labelNodes []Node
		for _, n := range allNodes {
			for _, l := range n.Labels {
				if l == c.Label {
					labelNodes = append(labelNodes, n)
					break
				}
			}
		}

		switch c.Type {
		case ConstraintNotNull, ConstraintExists:
			for _, n := range labelNodes {
				val, ok := n.Properties[c.Property]
				if !ok || val == nil {
					violations = append(violations, fmt.Sprintf(
						"node #%d [%s]: property %q is required (%s constraint)",
						n.ID, c.Label, c.Property, c.Type))
				}
			}

		case ConstraintUnique:
			seen := make(map[string]uint64)
			for _, n := range labelNodes {
				val, ok := n.Properties[c.Property]
				if !ok {
					continue
				}
				valStr := fmt.Sprintf("%v", val)
				if prevID, exists := seen[valStr]; exists {
					violations = append(violations, fmt.Sprintf(
						"nodes #%d and #%d [%s]: duplicate value %q for %q (UNIQUE constraint)",
						prevID, n.ID, c.Label, valStr, c.Property))
				}
				seen[valStr] = n.ID
			}
		}
	}

	return violations, nil
}

// ── Schema Introspection ─────────────────────────────────────────

// Schema returns a comprehensive view of the graph's structure.
//
// What: Returns all labels, relationship types, property keys, indexes,
//       constraints, and count statistics.
// Why:  Essential for tooling, visualization, and query planning.
func (s *Store) Schema() (*SchemaInfo, error) {
	nodes := s.AllNodes()
	edges := s.AllEdges()

	labelSet := make(map[string]bool)
	labelCounts := make(map[string]int)
	relTypeSet := make(map[string]bool)
	relTypeCounts := make(map[string]int)
	propKeySet := make(map[string]bool)

	for _, n := range nodes {
		for _, l := range n.Labels {
			labelSet[l] = true
			labelCounts[l]++
		}
		for k := range n.Properties {
			propKeySet[k] = true
		}
	}
	for _, e := range edges {
		relTypeSet[e.Type] = true
		relTypeCounts[e.Type]++
		for k := range e.Properties {
			propKeySet[k] = true
		}
	}

	labels := mapKeys(labelSet)
	relTypes := mapKeys(relTypeSet)
	propKeys := mapKeys(propKeySet)

	indexes, _ := s.ListIndexes()
	constraints, _ := s.ListConstraints()

	return &SchemaInfo{
		Labels:        labels,
		RelTypes:      relTypes,
		PropertyKeys:  propKeys,
		Indexes:       indexes,
		Constraints:   constraints,
		NodeCount:     len(nodes),
		EdgeCount:     len(edges),
		LabelCounts:   labelCounts,
		RelTypeCounts: relTypeCounts,
	}, nil
}

// CountByRelType returns the number of edges for each relationship type.
func (s *Store) CountByRelType() (map[string]int, error) {
	edges := s.AllEdges()
	counts := make(map[string]int)
	for _, e := range edges {
		counts[e.Type]++
	}
	return counts, nil
}

// ── Change Data Capture (CDC) ────────────────────────────────────

// WatchChanges registers a listener for graph change events.
//
// What: Callback-based CDC for real-time data integration.
// Why:  Enables downstream systems (search, analytics, replication)
//       to react to graph mutations in real time.
func (s *Store) WatchChanges(listener ChangeListener) {
	changeListenersMu.Lock()
	defer changeListenersMu.Unlock()
	changeListeners = append(changeListeners, listener)
}

// emitChange broadcasts a CDC event to all registered listeners.
func emitChange(event ChangeEvent) {
	changeListenersMu.RLock()
	defer changeListenersMu.RUnlock()
	for _, l := range changeListeners {
		go l(event) // Non-blocking dispatch.
	}
}

// ── Helpers ──────────────────────────────────────────────────────

func indexKey(label, property string, value interface{}) []byte {
	return []byte(fmt.Sprintf("%s:%s:%v", label, property, value))
}

func mapKeys(m map[string]bool) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// Ensure strings import is used.
var _ = strings.TrimSpace
