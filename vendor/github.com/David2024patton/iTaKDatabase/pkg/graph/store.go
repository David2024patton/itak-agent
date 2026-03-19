package graph

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
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
			bucketPropIndex, bucketConstraints, bucketIndexDefs,
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

// ── Edge CRUD (extended) ──────────────────────────────────────────

// GetEdge retrieves a single edge by ID.
func (s *Store) GetEdge(id uint64) (*Edge, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var edge Edge
	err := s.db.View(func(tx *bolt.Tx) error {
		data := tx.Bucket(bucketEdges).Get(itob(id))
		if data == nil {
			return fmt.Errorf("edge %d not found", id)
		}
		return json.Unmarshal(data, &edge)
	})
	if err != nil {
		return nil, err
	}
	return &edge, nil
}

// DeleteEdge removes a single edge by ID and cleans up the edge index.
func (s *Store) DeleteEdge(id uint64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.db.Update(func(tx *bolt.Tx) error {
		edges := tx.Bucket(bucketEdges)
		edgeIdx := tx.Bucket(bucketEdgeIndex)

		// Find the edge to get its source for index cleanup.
		data := edges.Get(itob(id))
		if data == nil {
			return fmt.Errorf("edge %d not found", id)
		}

		var edge Edge
		if err := json.Unmarshal(data, &edge); err != nil {
			return err
		}

		// Remove from edge index (source -> []edgeIDs).
		srcKey := itob(edge.SourceID)
		if existing := edgeIdx.Get(srcKey); existing != nil {
			var ids []uint64
			json.Unmarshal(existing, &ids)
			filtered := make([]uint64, 0, len(ids))
			for _, eid := range ids {
				if eid != id {
					filtered = append(filtered, eid)
				}
			}
			if len(filtered) > 0 {
				data, _ := json.Marshal(filtered)
				edgeIdx.Put(srcKey, data)
			} else {
				edgeIdx.Delete(srcKey)
			}
		}

		return edges.Delete(itob(id))
	})
}

// UpdateEdge updates an edge's properties. Merges with existing.
func (s *Store) UpdateEdge(id uint64, props map[string]interface{}) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.db.Update(func(tx *bolt.Tx) error {
		edges := tx.Bucket(bucketEdges)
		data := edges.Get(itob(id))
		if data == nil {
			return fmt.Errorf("edge %d not found", id)
		}

		var edge Edge
		if err := json.Unmarshal(data, &edge); err != nil {
			return err
		}

		if edge.Properties == nil {
			edge.Properties = make(map[string]interface{})
		}
		for k, v := range props {
			edge.Properties[k] = v
		}

		updated, err := json.Marshal(edge)
		if err != nil {
			return err
		}
		return edges.Put(itob(id), updated)
	})
}

// FindEdgesBetween returns all edges between two nodes (either direction).
func (s *Store) FindEdgesBetween(nodeA, nodeB uint64) ([]Edge, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []Edge
	err := s.db.View(func(tx *bolt.Tx) error {
		return tx.Bucket(bucketEdges).ForEach(func(k, v []byte) error {
			var edge Edge
			if err := json.Unmarshal(v, &edge); err == nil {
				if (edge.SourceID == nodeA && edge.TargetID == nodeB) ||
					(edge.SourceID == nodeB && edge.TargetID == nodeA) {
					result = append(result, edge)
				}
			}
			return nil
		})
	})
	return result, err
}

// ── Shortest Path ─────────────────────────────────────────────────

// PathResult holds the shortest path between two nodes.
type PathResult struct {
	Found    bool     `json:"found"`
	Distance int      `json:"distance"`
	NodeIDs  []uint64 `json:"node_ids"`
	Edges    []uint64 `json:"edge_ids,omitempty"`
}

// ShortestPath finds the shortest path between two nodes using BFS.
// edgeType can be empty to follow all edge types.
func (s *Store) ShortestPath(fromID, toID uint64, edgeType string) (*PathResult, error) {
	if fromID == toID {
		return &PathResult{Found: true, Distance: 0, NodeIDs: []uint64{fromID}}, nil
	}

	type queueItem struct {
		nodeID uint64
		path   []uint64
		edges  []uint64
	}

	visited := map[uint64]bool{fromID: true}
	queue := []queueItem{{nodeID: fromID, path: []uint64{fromID}}}

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		outEdges, err := s.GetEdgesFrom(current.nodeID)
		if err != nil {
			continue
		}

		for _, edge := range outEdges {
			if edgeType != "" && edge.Type != edgeType {
				continue
			}

			if edge.TargetID == toID {
				newPath := make([]uint64, len(current.path)+1)
				copy(newPath, current.path)
				newPath[len(current.path)] = toID

				edgePath := make([]uint64, len(current.edges)+1)
				copy(edgePath, current.edges)
				edgePath[len(current.edges)] = edge.ID

				return &PathResult{
					Found:    true,
					Distance: len(newPath) - 1,
					NodeIDs:  newPath,
					Edges:    edgePath,
				}, nil
			}

			if !visited[edge.TargetID] {
				visited[edge.TargetID] = true
				newPath := make([]uint64, len(current.path)+1)
				copy(newPath, current.path)
				newPath[len(current.path)] = edge.TargetID

				edgePath := make([]uint64, len(current.edges)+1)
				copy(edgePath, current.edges)
				edgePath[len(current.edges)] = edge.ID

				queue = append(queue, queueItem{
					nodeID: edge.TargetID,
					path:   newPath,
					edges:  edgePath,
				})
			}
		}
	}

	return &PathResult{Found: false}, nil
}

// ── Bidirectional Traversal ───────────────────────────────────────

// TraverseBidirectional does a BFS that follows both outgoing AND incoming edges.
func (s *Store) TraverseBidirectional(startID uint64, maxDepth int, edgeType string) ([]TraversalResult, error) {
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

		// Outgoing edges.
		outEdges, _ := s.GetEdgesFrom(current.nodeID)
		for _, edge := range outEdges {
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

		// Incoming edges.
		inEdges, _ := s.GetEdgesTo(current.nodeID)
		for _, edge := range inEdges {
			if edgeType != "" && edge.Type != edgeType {
				continue
			}
			if !visited[edge.SourceID] {
				visited[edge.SourceID] = true
				newPath := make([]uint64, len(current.path)+1)
				copy(newPath, current.path)
				newPath[len(current.path)] = edge.SourceID
				queue = append(queue, queueItem{
					nodeID: edge.SourceID,
					depth:  current.depth + 1,
					path:   newPath,
				})
			}
		}
	}

	return results, nil
}

// ── Batch Operations ──────────────────────────────────────────────

// BatchNodeInput holds data for batch node creation.
type BatchNodeInput struct {
	Labels     []string               `json:"labels"`
	Properties map[string]interface{} `json:"properties,omitempty"`
	Embedding  []float32              `json:"embedding,omitempty"`
}

// BatchCreateNodes inserts multiple nodes in a single transaction.
func (s *Store) BatchCreateNodes(inputs []BatchNodeInput) ([]uint64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	ids := make([]uint64, 0, len(inputs))

	err := s.db.Update(func(tx *bolt.Tx) error {
		nodes := tx.Bucket(bucketNodes)
		idx := tx.Bucket(bucketNodeIndex)

		for _, input := range inputs {
			id, err := nodes.NextSequence()
			if err != nil {
				return err
			}

			now := time.Now()
			node := Node{
				ID:         id,
				Labels:     input.Labels,
				Properties: input.Properties,
				Embedding:  input.Embedding,
				CreatedAt:  now,
				UpdatedAt:  now,
			}

			data, err := json.Marshal(node)
			if err != nil {
				return err
			}

			if err := nodes.Put(itob(id), data); err != nil {
				return err
			}

			// Update label index.
			for _, label := range input.Labels {
				key := []byte(label)
				existing := idx.Get(key)
				var labelIDs []uint64
				if existing != nil {
					json.Unmarshal(existing, &labelIDs)
				}
				labelIDs = append(labelIDs, id)
				idData, _ := json.Marshal(labelIDs)
				idx.Put(key, idData)
			}

			ids = append(ids, id)
		}
		return nil
	})

	return ids, err
}

// BatchEdgeInput holds data for batch edge creation.
type BatchEdgeInput struct {
	Type       string                 `json:"type"`
	SourceID   uint64                 `json:"source_id"`
	TargetID   uint64                 `json:"target_id"`
	Properties map[string]interface{} `json:"properties,omitempty"`
}

// BatchCreateEdges inserts multiple edges in a single transaction.
func (s *Store) BatchCreateEdges(inputs []BatchEdgeInput) ([]uint64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	ids := make([]uint64, 0, len(inputs))

	err := s.db.Update(func(tx *bolt.Tx) error {
		edges := tx.Bucket(bucketEdges)
		edgeIdx := tx.Bucket(bucketEdgeIndex)

		for _, input := range inputs {
			id, err := edges.NextSequence()
			if err != nil {
				return err
			}

			edge := Edge{
				ID:         id,
				Type:       input.Type,
				SourceID:   input.SourceID,
				TargetID:   input.TargetID,
				Properties: input.Properties,
				CreatedAt:  time.Now(),
			}

			data, err := json.Marshal(edge)
			if err != nil {
				return err
			}

			if err := edges.Put(itob(id), data); err != nil {
				return err
			}

			srcKey := itob(input.SourceID)
			existing := edgeIdx.Get(srcKey)
			var edgeIDs []uint64
			if existing != nil {
				json.Unmarshal(existing, &edgeIDs)
			}
			edgeIDs = append(edgeIDs, id)
			idData, _ := json.Marshal(edgeIDs)
			edgeIdx.Put(srcKey, idData)

			ids = append(ids, id)
		}
		return nil
	})

	return ids, err
}

// ── Counting ──────────────────────────────────────────────────────

// NodeCountByLabel returns the count of nodes with the given label.
func (s *Store) NodeCountByLabel(label string) int {
	s.mu.RLock()
	defer s.mu.RUnlock()

	count := 0
	s.db.View(func(tx *bolt.Tx) error {
		idx := tx.Bucket(bucketNodeIndex)
		idData := idx.Get([]byte(label))
		if idData == nil {
			return nil
		}
		var ids []uint64
		if err := json.Unmarshal(idData, &ids); err != nil {
			return nil
		}
		count = len(ids)
		return nil
	})
	return count
}

// AllNodes returns every node in the database.
func (s *Store) AllNodes() []Node {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var nodes []Node
	s.db.View(func(tx *bolt.Tx) error {
		return tx.Bucket(bucketNodes).ForEach(func(k, v []byte) error {
			var node Node
			if err := json.Unmarshal(v, &node); err == nil {
				nodes = append(nodes, node)
			}
			return nil
		})
	})
	return nodes
}

// ── Export / Import ───────────────────────────────────────────────

// GraphExport holds the full serialized graph state.
type GraphExport struct {
	Nodes []Node `json:"nodes"`
	Edges []Edge `json:"edges"`
}

// ExportJSON writes the entire graph (nodes + edges) as JSON to the writer.
func (s *Store) ExportJSON(w io.Writer) error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var export GraphExport

	s.db.View(func(tx *bolt.Tx) error {
		tx.Bucket(bucketNodes).ForEach(func(k, v []byte) error {
			var node Node
			if err := json.Unmarshal(v, &node); err == nil {
				export.Nodes = append(export.Nodes, node)
			}
			return nil
		})
		tx.Bucket(bucketEdges).ForEach(func(k, v []byte) error {
			var edge Edge
			if err := json.Unmarshal(v, &edge); err == nil {
				export.Edges = append(export.Edges, edge)
			}
			return nil
		})
		return nil
	})

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(export)
}

// ImportJSON reads a GraphExport from the reader and creates all nodes/edges.
func (s *Store) ImportJSON(r io.Reader) (int, int, error) {
	var export GraphExport
	if err := json.NewDecoder(r).Decode(&export); err != nil {
		return 0, 0, fmt.Errorf("decode graph export: %w", err)
	}

	nodeCount := 0
	edgeCount := 0

	s.mu.Lock()
	defer s.mu.Unlock()

	err := s.db.Update(func(tx *bolt.Tx) error {
		nodes := tx.Bucket(bucketNodes)
		edges := tx.Bucket(bucketEdges)
		idx := tx.Bucket(bucketNodeIndex)
		edgeIdx := tx.Bucket(bucketEdgeIndex)

		for _, node := range export.Nodes {
			data, err := json.Marshal(node)
			if err != nil {
				continue
			}
			key := itob(node.ID)
			if err := nodes.Put(key, data); err != nil {
				return err
			}

			// Update label index.
			for _, label := range node.Labels {
				lk := []byte(label)
				existing := idx.Get(lk)
				var ids []uint64
				if existing != nil {
					json.Unmarshal(existing, &ids)
				}
				ids = append(ids, node.ID)
				idData, _ := json.Marshal(ids)
				idx.Put(lk, idData)
			}
			nodeCount++
		}

		for _, edge := range export.Edges {
			data, err := json.Marshal(edge)
			if err != nil {
				continue
			}
			key := itob(edge.ID)
			if err := edges.Put(key, data); err != nil {
				return err
			}

			srcKey := itob(edge.SourceID)
			existing := edgeIdx.Get(srcKey)
			var ids []uint64
			if existing != nil {
				json.Unmarshal(existing, &ids)
			}
			ids = append(ids, edge.ID)
			idData, _ := json.Marshal(ids)
			edgeIdx.Put(srcKey, idData)
			edgeCount++
		}

		return nil
	})

	return nodeCount, edgeCount, err
}

// ── Backup ────────────────────────────────────────────────────────

// Backup creates a consistent snapshot of the graph database.
func (s *Store) Backup(destPath string) error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.db.View(func(tx *bolt.Tx) error {
		return tx.CopyFile(destPath, 0600)
	})
}
