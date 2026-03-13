package graph

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	bolt "go.etcd.io/bbolt"
)

// ── Bucket names ──────────────────────────────────────────────────

var (
	bucketNodes      = []byte("nodes")
	bucketEdges      = []byte("edges")
	bucketNodeIndex  = []byte("node_index")  // label -> []nodeID
	bucketEdgeIndex  = []byte("edge_index")  // srcID -> []edgeID
	bucketMetadata   = []byte("metadata")
)

// ── Core types ────────────────────────────────────────────────────

// Node represents a vertex in the graph with typed properties.
//
// What: A labeled entity with arbitrary key-value properties.
// Why:  Everything the agent tracks becomes a node -- sessions, actions,
//       pages, searches, messages, entities, facts.
// How:  Stored as JSON in bbolt, keyed by auto-increment uint64 ID.
type Node struct {
	ID         uint64                 `json:"id"`
	Labels     []string               `json:"labels"`               // e.g., ["Action", "ToolCall"]
	Properties map[string]interface{} `json:"properties,omitempty"`
	Embedding  []float32              `json:"embedding,omitempty"`   // vector for similarity search
	CreatedAt  time.Time              `json:"created_at"`
	UpdatedAt  time.Time              `json:"updated_at"`
}

// Edge represents a directed relationship between two nodes.
//
// What: A typed, directed connection between two nodes.
// Why:  Graph relationships are the core value -- "Session PERFORMED Action",
//       "Action VISITED Page", "Session INCLUDES Message".
// How:  Stored as JSON in bbolt with source/target IDs and properties.
type Edge struct {
	ID         uint64                 `json:"id"`
	Type       string                 `json:"type"`                  // e.g., "PERFORMED", "VISITED"
	SourceID   uint64                 `json:"source_id"`
	TargetID   uint64                 `json:"target_id"`
	Properties map[string]interface{} `json:"properties,omitempty"`
	CreatedAt  time.Time              `json:"created_at"`
}

// ── Store ─────────────────────────────────────────────────────────

// Store is the core graph database engine backed by bbolt.
//
// What: An embedded, pure-Go graph database.
// Why:  Replaces Neo4j entirely. Ships inside itakagent.exe. Zero external
//       dependencies, zero Docker, zero network calls.
// How:  bbolt provides a battle-tested B+tree KV store. We layer graph
//       semantics (nodes, edges, labels, traversal) on top of it.
type Store struct {
	db   *bolt.DB
	mu   sync.RWMutex
	path string
}

// Open creates or opens an iTaK Database at the given file path.
func Open(path string) (*Store, error) {
	// Ensure parent directory exists.
	if dir := filepath.Dir(path); dir != "" {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, fmt.Errorf("itakdb: create dir %s: %w", dir, err)
		}
	}

	db, err := bolt.Open(path, 0600, &bolt.Options{
		Timeout:      1 * time.Second,
		NoGrowSync:   false,
		FreelistType: bolt.FreelistMapType,
	})
	if err != nil {
		return nil, fmt.Errorf("itakdb: open %s: %w", path, err)
	}

	// Create buckets if they don't exist.
	err = db.Update(func(tx *bolt.Tx) error {
		for _, bucket := range [][]byte{
			bucketNodes, bucketEdges, bucketNodeIndex, bucketEdgeIndex, bucketMetadata,
		} {
			if _, err := tx.CreateBucketIfNotExists(bucket); err != nil {
				return fmt.Errorf("create bucket %s: %w", string(bucket), err)
			}
		}

		// Write DB version metadata.
		meta := tx.Bucket(bucketMetadata)
		if meta.Get([]byte("version")) == nil {
			meta.Put([]byte("version"), []byte("1.0.0"))
			meta.Put([]byte("engine"), []byte("iTaK Database"))
			meta.Put([]byte("created"), []byte(time.Now().Format(time.RFC3339)))
		}
		return nil
	})
	if err != nil {
		db.Close()
		return nil, err
	}

	log.Printf("[itakdb] Opened graph database at %s", path)
	return &Store{db: db, path: path}, nil
}

// Close shuts down the database.
func (s *Store) Close() error {
	log.Printf("[itakdb] Closing graph database at %s", s.path)
	return s.db.Close()
}

// Path returns the database file path.
func (s *Store) Path() string { return s.path }

// BoltDB returns the underlying bbolt handle for shared access
// by the table engine and full-text search engine.
func (s *Store) BoltDB() *bolt.DB { return s.db }

// ── Node CRUD ─────────────────────────────────────────────────────

// CreateNode inserts a new node and returns its auto-generated ID.
func (s *Store) CreateNode(labels []string, props map[string]interface{}, embedding []float32) (uint64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var nodeID uint64

	err := s.db.Update(func(tx *bolt.Tx) error {
		nodes := tx.Bucket(bucketNodes)
		idx := tx.Bucket(bucketNodeIndex)

		// Auto-increment ID.
		id, err := nodes.NextSequence()
		if err != nil {
			return err
		}
		nodeID = id

		now := time.Now()
		node := Node{
			ID:         id,
			Labels:     labels,
			Properties: props,
			Embedding:  embedding,
			CreatedAt:  now,
			UpdatedAt:  now,
		}

		data, err := json.Marshal(node)
		if err != nil {
			return fmt.Errorf("marshal node: %w", err)
		}

		if err := nodes.Put(itob(id), data); err != nil {
			return err
		}

		// Update label index: for each label, append this node ID.
		for _, label := range labels {
			key := []byte(label)
			existing := idx.Get(key)
			var ids []uint64
			if existing != nil {
				json.Unmarshal(existing, &ids)
			}
			ids = append(ids, id)
			idData, _ := json.Marshal(ids)
			idx.Put(key, idData)
		}

		return nil
	})

	return nodeID, err
}

// GetNode retrieves a node by ID.
func (s *Store) GetNode(id uint64) (*Node, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var node Node
	err := s.db.View(func(tx *bolt.Tx) error {
		data := tx.Bucket(bucketNodes).Get(itob(id))
		if data == nil {
			return fmt.Errorf("node %d not found", id)
		}
		return json.Unmarshal(data, &node)
	})
	if err != nil {
		return nil, err
	}
	return &node, nil
}

// UpdateNode updates a node's properties. Merges with existing.
func (s *Store) UpdateNode(id uint64, props map[string]interface{}) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.db.Update(func(tx *bolt.Tx) error {
		nodes := tx.Bucket(bucketNodes)
		data := nodes.Get(itob(id))
		if data == nil {
			return fmt.Errorf("node %d not found", id)
		}

		var node Node
		if err := json.Unmarshal(data, &node); err != nil {
			return err
		}

		if node.Properties == nil {
			node.Properties = make(map[string]interface{})
		}
		for k, v := range props {
			node.Properties[k] = v
		}
		node.UpdatedAt = time.Now()

		updated, err := json.Marshal(node)
		if err != nil {
			return err
		}
		return nodes.Put(itob(id), updated)
	})
}

// DeleteNode removes a node and all its connected edges.
func (s *Store) DeleteNode(id uint64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.db.Update(func(tx *bolt.Tx) error {
		nodes := tx.Bucket(bucketNodes)
		edges := tx.Bucket(bucketEdges)

		// Delete connected edges.
		edges.ForEach(func(k, v []byte) error {
			var edge Edge
			if err := json.Unmarshal(v, &edge); err == nil {
				if edge.SourceID == id || edge.TargetID == id {
					edges.Delete(k)
				}
			}
			return nil
		})

		return nodes.Delete(itob(id))
	})
}

// FindByLabel returns all nodes with the given label.
func (s *Store) FindByLabel(label string) ([]Node, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var nodes []Node
	err := s.db.View(func(tx *bolt.Tx) error {
		idx := tx.Bucket(bucketNodeIndex)
		nodeBucket := tx.Bucket(bucketNodes)

		idData := idx.Get([]byte(label))
		if idData == nil {
			return nil // no nodes with this label
		}

		var ids []uint64
		if err := json.Unmarshal(idData, &ids); err != nil {
			return err
		}

		for _, id := range ids {
			data := nodeBucket.Get(itob(id))
			if data != nil {
				var node Node
				if err := json.Unmarshal(data, &node); err == nil {
					nodes = append(nodes, node)
				}
			}
		}
		return nil
	})
	return nodes, err
}

// FindByProperty returns nodes where a specific property matches the value.
func (s *Store) FindByProperty(key string, value interface{}) ([]Node, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var results []Node
	valStr := fmt.Sprintf("%v", value)

	err := s.db.View(func(tx *bolt.Tx) error {
		return tx.Bucket(bucketNodes).ForEach(func(k, v []byte) error {
			var node Node
			if err := json.Unmarshal(v, &node); err == nil {
				if node.Properties != nil {
					if propVal, ok := node.Properties[key]; ok {
						if fmt.Sprintf("%v", propVal) == valStr {
							results = append(results, node)
						}
					}
				}
			}
			return nil
		})
	})
	return results, err
}

// MergeNode does MERGE semantics: find by label+key property, create if not found, update if found.
// This is the equivalent of Neo4j's MERGE operation.
func (s *Store) MergeNode(label, matchKey, matchValue string, props map[string]interface{}, embedding []float32) (uint64, bool, error) {
	// First try to find existing.
	existing, err := s.FindByProperty(matchKey, matchValue)
	if err != nil {
		return 0, false, err
	}

	// Filter by label.
	for _, node := range existing {
		for _, l := range node.Labels {
			if l == label {
				// Found. Update properties.
				if err := s.UpdateNode(node.ID, props); err != nil {
					return node.ID, false, err
				}
				// Update embedding if provided.
				if embedding != nil {
					s.updateEmbedding(node.ID, embedding)
				}
				return node.ID, false, nil // created=false
			}
		}
	}

	// Not found. Create.
	if props == nil {
		props = make(map[string]interface{})
	}
	props[matchKey] = matchValue
	id, err := s.CreateNode([]string{label}, props, embedding)
	return id, true, err // created=true
}

// ── Edge CRUD ─────────────────────────────────────────────────────

// CreateEdge inserts a directed edge between two nodes.
func (s *Store) CreateEdge(edgeType string, sourceID, targetID uint64, props map[string]interface{}) (uint64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var edgeID uint64

	err := s.db.Update(func(tx *bolt.Tx) error {
		edges := tx.Bucket(bucketEdges)
		edgeIdx := tx.Bucket(bucketEdgeIndex)

		id, err := edges.NextSequence()
		if err != nil {
			return err
		}
		edgeID = id

		edge := Edge{
			ID:         id,
			Type:       edgeType,
			SourceID:   sourceID,
			TargetID:   targetID,
			Properties: props,
			CreatedAt:  time.Now(),
		}

		data, err := json.Marshal(edge)
		if err != nil {
			return fmt.Errorf("marshal edge: %w", err)
		}

		if err := edges.Put(itob(id), data); err != nil {
			return err
		}

		// Update edge index: source -> []edgeIDs
		srcKey := itob(sourceID)
		existing := edgeIdx.Get(srcKey)
		var ids []uint64
		if existing != nil {
			json.Unmarshal(existing, &ids)
		}
		ids = append(ids, id)
		idData, _ := json.Marshal(ids)
		edgeIdx.Put(srcKey, idData)

		return nil
	})

	return edgeID, err
}

// GetEdgesFrom returns all edges originating from a node.
func (s *Store) GetEdgesFrom(nodeID uint64) ([]Edge, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var edges []Edge
	err := s.db.View(func(tx *bolt.Tx) error {
		edgeIdx := tx.Bucket(bucketEdgeIndex)
		edgeBucket := tx.Bucket(bucketEdges)

		idData := edgeIdx.Get(itob(nodeID))
		if idData == nil {
			return nil
		}

		var ids []uint64
		if err := json.Unmarshal(idData, &ids); err != nil {
			return err
		}

		for _, id := range ids {
			data := edgeBucket.Get(itob(id))
			if data != nil {
				var edge Edge
				if err := json.Unmarshal(data, &edge); err == nil {
					edges = append(edges, edge)
				}
			}
		}
		return nil
	})
	return edges, err
}

// GetEdgesTo returns all edges pointing to a node.
func (s *Store) GetEdgesTo(nodeID uint64) ([]Edge, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var edges []Edge
	err := s.db.View(func(tx *bolt.Tx) error {
		return tx.Bucket(bucketEdges).ForEach(func(k, v []byte) error {
			var edge Edge
			if err := json.Unmarshal(v, &edge); err == nil {
				if edge.TargetID == nodeID {
					edges = append(edges, edge)
				}
			}
			return nil
		})
	})
	return edges, err
}

// AllEdges returns every edge in the database (for visualization).
func (s *Store) AllEdges() []Edge {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var edges []Edge
	s.db.View(func(tx *bolt.Tx) error {
		return tx.Bucket(bucketEdges).ForEach(func(k, v []byte) error {
			var edge Edge
			if err := json.Unmarshal(v, &edge); err == nil {
				edges = append(edges, edge)
			}
			return nil
		})
	})
	return edges
}

// ── Traversal ─────────────────────────────────────────────────────

// TraversalResult holds a node and the path taken to reach it.
type TraversalResult struct {
	Node  Node     `json:"node"`
	Depth int      `json:"depth"`
	Path  []uint64 `json:"path"` // node IDs in the path
}

// Traverse does a breadth-first traversal from a starting node up to maxDepth hops.
// Optionally filter by edge type.
func (s *Store) Traverse(startID uint64, maxDepth int, edgeType string) ([]TraversalResult, error) {
	visited := make(map[uint64]bool)
	var results []TraversalResult

	type queueItem struct {
		nodeID uint64
		depth  int
		path   []uint64
	}

	queue := []queueItem{{nodeID: startID, depth: 0, path: []uint64{startID}}}
	visited[startID] = true

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		node, err := s.GetNode(current.nodeID)
		if err != nil {
			continue
		}

		results = append(results, TraversalResult{
			Node:  *node,
			Depth: current.depth,
			Path:  current.path,
		})

		if current.depth >= maxDepth {
			continue
		}

		edges, err := s.GetEdgesFrom(current.nodeID)
		if err != nil {
			continue
		}

		for _, edge := range edges {
			if edgeType != "" && edge.Type != edgeType {
				continue
			}
			if !visited[edge.TargetID] {
				visited[edge.TargetID] = true
				newPath := make([]uint64, len(current.path)+1)
				copy(newPath, current.path)
				newPath[len(current.path)] = edge.TargetID
				queue = append(queue, queueItem{
					nodeID: edge.TargetID,
					depth:  current.depth + 1,
					path:   newPath,
				})
			}
		}
	}

	return results, nil
}

// ── Stats ─────────────────────────────────────────────────────────

// Stats returns database statistics.
type Stats struct {
	NodeCount int    `json:"node_count"`
	EdgeCount int    `json:"edge_count"`
	FileSize  int64  `json:"file_size_bytes"`
	Version   string `json:"version"`
}

func (s *Store) Stats() Stats {
	var stats Stats

	s.db.View(func(tx *bolt.Tx) error {
		stats.NodeCount = tx.Bucket(bucketNodes).Stats().KeyN
		stats.EdgeCount = tx.Bucket(bucketEdges).Stats().KeyN
		meta := tx.Bucket(bucketMetadata)
		if v := meta.Get([]byte("version")); v != nil {
			stats.Version = string(v)
		}
		return nil
	})

	return stats
}

// ── Internal helpers ──────────────────────────────────────────────

// itob converts a uint64 to an 8-byte big-endian byte slice.
func itob(v uint64) []byte {
	b := make([]byte, 8)
	binary.BigEndian.PutUint64(b, v)
	return b
}

// btoi converts an 8-byte big-endian byte slice to a uint64.
func btoi(b []byte) uint64 {
	return binary.BigEndian.Uint64(b)
}

func (s *Store) updateEmbedding(id uint64, embedding []float32) {
	s.db.Update(func(tx *bolt.Tx) error {
		nodes := tx.Bucket(bucketNodes)
		data := nodes.Get(itob(id))
		if data == nil {
			return nil
		}
		var node Node
		if err := json.Unmarshal(data, &node); err != nil {
			return err
		}
		node.Embedding = embedding
		node.UpdatedAt = time.Now()
		updated, err := json.Marshal(node)
		if err != nil {
			return err
		}
		return nodes.Put(itob(id), updated)
	})
}
