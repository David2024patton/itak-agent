package vector

import (
	"container/heap"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"math/rand"
	"sync"

	bolt "go.etcd.io/bbolt"
)

// ── Distance Metrics ──────────────────────────────────────────────

// DistanceMetric defines how similarity is calculated.
type DistanceMetric int

const (
	Cosine    DistanceMetric = iota // Default: cosine similarity
	Euclidean                       // L2 distance
	DotProduct                      // Inner product
)

// ── Index ─────────────────────────────────────────────────────────

// Index is a pure-Go HNSW (Hierarchical Navigable Small World) vector index.
//
// What: Approximate nearest neighbor search for embedding vectors.
// Why:  The agent needs fast semantic search across thousands of graph nodes.
//       HNSW provides O(log N) search with >95% recall on typical workloads.
// How:  Multi-layer skip-list graph where each layer is a navigable small world.
//       Supports persistence via bbolt, multiple distance metrics, and metadata filtering.
type Index struct {
	mu             sync.RWMutex
	nodes          map[uint64]*hnode
	metadata       map[uint64]map[string]interface{} // per-vector metadata for filtered search
	dimensions     int
	maxLevel       int
	efConstruction int
	efSearch       int
	mMax           int
	mMax0          int
	entryPoint     uint64
	hasEntry       bool
	rng            *rand.Rand
	metric         DistanceMetric
}

type hnode struct {
	id        uint64
	vector    []float32
	level     int
	neighbors [][]uint64
}

// NewIndex creates a new HNSW vector index with cosine similarity.
func NewIndex(dimensions int) *Index {
	return &Index{
		nodes:          make(map[uint64]*hnode),
		metadata:       make(map[uint64]map[string]interface{}),
		dimensions:     dimensions,
		maxLevel:       0,
		efConstruction: 200,
		efSearch:       50,
		mMax:           16,
		mMax0:          32,
		rng:            rand.New(rand.NewSource(42)),
		metric:         Cosine,
	}
}

// NewIndexWithMetric creates a new HNSW index with a specific distance metric.
func NewIndexWithMetric(dimensions int, metric DistanceMetric) *Index {
	idx := NewIndex(dimensions)
	idx.metric = metric
	return idx
}

// ── Insert / Update / Delete ──────────────────────────────────────

// Insert adds a vector with the given ID to the index.
func (idx *Index) Insert(id uint64, vector []float32) {
	idx.InsertWithMeta(id, vector, nil)
}

// InsertWithMeta adds a vector with metadata for filtered search.
func (idx *Index) InsertWithMeta(id uint64, vector []float32, meta map[string]interface{}) {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	if len(vector) != idx.dimensions && idx.dimensions > 0 {
		return
	}
	if idx.dimensions == 0 {
		idx.dimensions = len(vector)
	}

	level := 0
	for idx.rng.Float64() < 0.5 && level < 16 {
		level++
	}

	node := &hnode{
		id:        id,
		vector:    vector,
		level:     level,
		neighbors: make([][]uint64, level+1),
	}
	for i := range node.neighbors {
		node.neighbors[i] = make([]uint64, 0)
	}

	idx.nodes[id] = node
	if meta != nil {
		idx.metadata[id] = meta
	}

	if !idx.hasEntry {
		idx.entryPoint = id
		idx.hasEntry = true
		idx.maxLevel = level
		return
	}

	ep := idx.entryPoint
	for l := idx.maxLevel; l > level; l-- {
		ep = idx.greedyClosest(vector, ep, l)
	}

	for l := min(level, idx.maxLevel); l >= 0; l-- {
		candidates := idx.searchLayer(vector, ep, idx.efConstruction, l)

		maxConn := idx.mMax
		if l == 0 {
			maxConn = idx.mMax0
		}

		selected := selectNearest(candidates, vector, maxConn, idx.nodes, idx.distanceFunc())

		node.neighbors[l] = selected

		for _, neighborID := range selected {
			neighbor, ok := idx.nodes[neighborID]
			if !ok || l >= len(neighbor.neighbors) {
				continue
			}
			neighbor.neighbors[l] = append(neighbor.neighbors[l], id)

			if len(neighbor.neighbors[l]) > maxConn {
				neighbor.neighbors[l] = selectNearest(neighbor.neighbors[l], neighbor.vector, maxConn, idx.nodes, idx.distanceFunc())
			}
		}

		if len(candidates) > 0 {
			ep = candidates[0]
		}
	}

	if level > idx.maxLevel {
		idx.maxLevel = level
		idx.entryPoint = id
	}
}

// Update replaces a vector's data and metadata while keeping the same ID.
func (idx *Index) Update(id uint64, vector []float32, meta map[string]interface{}) {
	idx.Delete(id)
	idx.InsertWithMeta(id, vector, meta)
}

// BatchInsert adds multiple vectors in one call.
func (idx *Index) BatchInsert(ids []uint64, vectors [][]float32) {
	for i := range ids {
		if i < len(vectors) {
			idx.Insert(ids[i], vectors[i])
		}
	}
}

// BatchInsertWithMeta adds multiple vectors with metadata.
func (idx *Index) BatchInsertWithMeta(ids []uint64, vectors [][]float32, metas []map[string]interface{}) {
	for i := range ids {
		var meta map[string]interface{}
		if i < len(metas) {
			meta = metas[i]
		}
		if i < len(vectors) {
			idx.InsertWithMeta(ids[i], vectors[i], meta)
		}
	}
}

// Delete removes a vector from the index.
func (idx *Index) Delete(id uint64) {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	node, ok := idx.nodes[id]
	if !ok {
		return
	}

	for l := 0; l <= node.level && l < len(node.neighbors); l++ {
		for _, neighborID := range node.neighbors[l] {
			neighbor, nok := idx.nodes[neighborID]
			if !nok || l >= len(neighbor.neighbors) {
				continue
			}
			filtered := make([]uint64, 0, len(neighbor.neighbors[l]))
			for _, nid := range neighbor.neighbors[l] {
				if nid != id {
					filtered = append(filtered, nid)
				}
			}
			neighbor.neighbors[l] = filtered
		}
	}

	delete(idx.nodes, id)
	delete(idx.metadata, id)

	if idx.entryPoint == id {
		if len(idx.nodes) == 0 {
			idx.hasEntry = false
			idx.maxLevel = 0
		} else {
			for nid := range idx.nodes {
				idx.entryPoint = nid
				break
			}
		}
	}
}

// ── Search ────────────────────────────────────────────────────────

// SearchResult holds a search result with its similarity score.
type SearchResult struct {
	ID       uint64                 `json:"id"`
	Score    float64                `json:"score"`
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

// Search finds the K nearest neighbors to the query vector.
func (idx *Index) Search(query []float32, k int) []SearchResult {
	return idx.SearchFiltered(query, k, nil)
}

// SearchFiltered finds the K nearest neighbors with optional metadata filter.
// The filter function receives metadata for each candidate and returns true to include.
func (idx *Index) SearchFiltered(query []float32, k int, filter func(map[string]interface{}) bool) []SearchResult {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	if !idx.hasEntry || len(idx.nodes) == 0 {
		return nil
	}

	distFn := idx.distanceFunc()

	ep := idx.entryPoint
	for l := idx.maxLevel; l > 0; l-- {
		ep = idx.greedyClosest(query, ep, l)
	}

	candidates := idx.searchLayer(query, ep, max(idx.efSearch, k*2), 0)

	var results []SearchResult
	for _, candidateID := range candidates {
		node, ok := idx.nodes[candidateID]
		if !ok {
			continue
		}

		// Apply metadata filter if provided.
		meta := idx.metadata[candidateID]
		if filter != nil && !filter(meta) {
			continue
		}

		score := 1.0 - distFn(query, node.vector)
		results = append(results, SearchResult{
			ID:       candidateID,
			Score:    score,
			Metadata: meta,
		})
	}

	sortResults(results)

	if len(results) > k {
		results = results[:k]
	}
	return results
}

// RangeSearch returns all vectors within the given distance threshold.
func (idx *Index) RangeSearch(query []float32, threshold float64) []SearchResult {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	if !idx.hasEntry || len(idx.nodes) == 0 {
		return nil
	}

	distFn := idx.distanceFunc()

	// Brute-force scan for range search (HNSW doesn't natively support it).
	var results []SearchResult
	for id, node := range idx.nodes {
		score := 1.0 - distFn(query, node.vector)
		if score >= threshold {
			results = append(results, SearchResult{
				ID:       id,
				Score:    score,
				Metadata: idx.metadata[id],
			})
		}
	}

	sortResults(results)
	return results
}

// ── Stats ─────────────────────────────────────────────────────────

// IndexStats holds statistics about the vector index.
type IndexStats struct {
	Size       int            `json:"size"`
	Dimensions int            `json:"dimensions"`
	MaxLevel   int            `json:"max_level"`
	Metric     string         `json:"metric"`
	MemoryEst  int64          `json:"memory_est_bytes"`
}

// Stats returns statistics about the index.
func (idx *Index) Stats() IndexStats {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	metricName := "cosine"
	switch idx.metric {
	case Euclidean:
		metricName = "euclidean"
	case DotProduct:
		metricName = "dot_product"
	}

	// Rough memory estimate: vector data + neighbor lists.
	memEst := int64(len(idx.nodes)) * int64(idx.dimensions) * 4 // float32 = 4 bytes

	return IndexStats{
		Size:       len(idx.nodes),
		Dimensions: idx.dimensions,
		MaxLevel:   idx.maxLevel,
		Metric:     metricName,
		MemoryEst:  memEst,
	}
}

// Size returns the number of vectors in the index.
func (idx *Index) Size() int {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	return len(idx.nodes)
}

// GetVector returns the raw vector for a given ID.
func (idx *Index) GetVector(id uint64) ([]float32, bool) {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	node, ok := idx.nodes[id]
	if !ok {
		return nil, false
	}
	return node.vector, true
}

// GetMetadata returns the metadata for a given ID.
func (idx *Index) GetMetadata(id uint64) (map[string]interface{}, bool) {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	meta, ok := idx.metadata[id]
	return meta, ok
}

// ── Persistence ───────────────────────────────────────────────────

// vectorRecord is the serialized form of a vector node.
type vectorRecord struct {
	ID        uint64                 `json:"id"`
	Vector    []float32              `json:"vector"`
	Level     int                    `json:"level"`
	Neighbors [][]uint64             `json:"neighbors"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
}

// Save persists the entire index to a bbolt database under the "_vector_index" bucket.
func (idx *Index) Save(db *bolt.DB) error {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	return db.Update(func(tx *bolt.Tx) error {
		// Delete existing bucket and recreate.
		tx.DeleteBucket([]byte("_vector_index"))
		bkt, err := tx.CreateBucket([]byte("_vector_index"))
		if err != nil {
			return err
		}

		// Store index metadata.
		metaBkt, err := tx.CreateBucketIfNotExists([]byte("_vector_meta"))
		if err != nil {
			return err
		}

		metaData, _ := json.Marshal(map[string]interface{}{
			"dimensions":     idx.dimensions,
			"max_level":      idx.maxLevel,
			"entry_point":    idx.entryPoint,
			"has_entry":      idx.hasEntry,
			"metric":         int(idx.metric),
			"ef_construction": idx.efConstruction,
			"ef_search":      idx.efSearch,
			"m_max":          idx.mMax,
			"m_max0":         idx.mMax0,
		})
		metaBkt.Put([]byte("config"), metaData)

		// Store each node.
		for id, node := range idx.nodes {
			rec := vectorRecord{
				ID:        id,
				Vector:    node.vector,
				Level:     node.level,
				Neighbors: node.neighbors,
				Metadata:  idx.metadata[id],
			}

			data, err := json.Marshal(rec)
			if err != nil {
				return err
			}

			key := make([]byte, 8)
			binary.BigEndian.PutUint64(key, id)
			if err := bkt.Put(key, data); err != nil {
				return err
			}
		}

		return nil
	})
}

// Load restores the index from a bbolt database.
func (idx *Index) Load(db *bolt.DB) error {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	return db.View(func(tx *bolt.Tx) error {
		// Load metadata.
		metaBkt := tx.Bucket([]byte("_vector_meta"))
		if metaBkt != nil {
			if configData := metaBkt.Get([]byte("config")); configData != nil {
				var config map[string]interface{}
				if err := json.Unmarshal(configData, &config); err == nil {
					if v, ok := config["dimensions"].(float64); ok {
						idx.dimensions = int(v)
					}
					if v, ok := config["max_level"].(float64); ok {
						idx.maxLevel = int(v)
					}
					if v, ok := config["entry_point"].(float64); ok {
						idx.entryPoint = uint64(v)
					}
					if v, ok := config["has_entry"].(bool); ok {
						idx.hasEntry = v
					}
					if v, ok := config["metric"].(float64); ok {
						idx.metric = DistanceMetric(int(v))
					}
					if v, ok := config["ef_construction"].(float64); ok {
						idx.efConstruction = int(v)
					}
					if v, ok := config["ef_search"].(float64); ok {
						idx.efSearch = int(v)
					}
					if v, ok := config["m_max"].(float64); ok {
						idx.mMax = int(v)
					}
					if v, ok := config["m_max0"].(float64); ok {
						idx.mMax0 = int(v)
					}
				}
			}
		}

		// Load vectors.
		bkt := tx.Bucket([]byte("_vector_index"))
		if bkt == nil {
			return nil // No vectors saved.
		}

		idx.nodes = make(map[uint64]*hnode)
		idx.metadata = make(map[uint64]map[string]interface{})

		return bkt.ForEach(func(k, v []byte) error {
			var rec vectorRecord
			if err := json.Unmarshal(v, &rec); err != nil {
				return nil // skip corrupt records
			}

			idx.nodes[rec.ID] = &hnode{
				id:        rec.ID,
				vector:    rec.Vector,
				level:     rec.Level,
				neighbors: rec.Neighbors,
			}
			if rec.Metadata != nil {
				idx.metadata[rec.ID] = rec.Metadata
			}
			return nil
		})
	})
}

// ── Export / Import ───────────────────────────────────────────────

// ExportJSON writes the entire index as JSON to the writer.
func (idx *Index) ExportJSON(w io.Writer) error {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	var records []vectorRecord
	for id, node := range idx.nodes {
		records = append(records, vectorRecord{
			ID:        id,
			Vector:    node.vector,
			Level:     node.level,
			Neighbors: node.neighbors,
			Metadata:  idx.metadata[id],
		})
	}

	enc := json.NewEncoder(w)
	return enc.Encode(records)
}

// ImportJSON reads vectors from a JSON reader and inserts them.
func (idx *Index) ImportJSON(r io.Reader) (int, error) {
	var records []vectorRecord
	if err := json.NewDecoder(r).Decode(&records); err != nil {
		return 0, fmt.Errorf("decode vectors: %w", err)
	}

	count := 0
	for _, rec := range records {
		idx.InsertWithMeta(rec.ID, rec.Vector, rec.Metadata)
		count++
	}
	return count, nil
}

// ── Internal HNSW algorithms ──────────────────────────────────────

func (idx *Index) distanceFunc() func([]float32, []float32) float64 {
	switch idx.metric {
	case Euclidean:
		return euclideanDistance
	case DotProduct:
		return dotProductDistance
	default:
		return cosineDistance
	}
}

func (idx *Index) greedyClosest(query []float32, ep uint64, level int) uint64 {
	distFn := idx.distanceFunc()
	current := ep
	currentDist := distFn(query, idx.nodes[current].vector)

	for {
		changed := false
		node := idx.nodes[current]
		if level >= len(node.neighbors) {
			break
		}

		for _, neighborID := range node.neighbors[level] {
			neighbor, ok := idx.nodes[neighborID]
			if !ok {
				continue
			}
			dist := distFn(query, neighbor.vector)
			if dist < currentDist {
				current = neighborID
				currentDist = dist
				changed = true
			}
		}

		if !changed {
			break
		}
	}
	return current
}

func (idx *Index) searchLayer(query []float32, ep uint64, ef int, level int) []uint64 {
	distFn := idx.distanceFunc()
	visited := map[uint64]bool{ep: true}

	candidates := &distHeap{}
	results := &distHeap{}

	epDist := distFn(query, idx.nodes[ep].vector)
	heap.Push(candidates, distItem{id: ep, dist: epDist})
	heap.Push(results, distItem{id: ep, dist: -epDist})

	for candidates.Len() > 0 {
		closest := heap.Pop(candidates).(distItem)

		if results.Len() >= ef {
			farthest := (*results)[0]
			if closest.dist > -farthest.dist {
				break
			}
		}

		node, ok := idx.nodes[closest.id]
		if !ok || level >= len(node.neighbors) {
			continue
		}

		for _, neighborID := range node.neighbors[level] {
			if visited[neighborID] {
				continue
			}
			visited[neighborID] = true

			neighbor, nok := idx.nodes[neighborID]
			if !nok {
				continue
			}

			dist := distFn(query, neighbor.vector)

			if results.Len() < ef {
				heap.Push(candidates, distItem{id: neighborID, dist: dist})
				heap.Push(results, distItem{id: neighborID, dist: -dist})
			} else {
				farthest := (*results)[0]
				if dist < -farthest.dist {
					heap.Push(candidates, distItem{id: neighborID, dist: dist})
					heap.Push(results, distItem{id: neighborID, dist: -dist})
					if results.Len() > ef {
						heap.Pop(results)
					}
				}
			}
		}
	}

	ids := make([]uint64, results.Len())
	for i := results.Len() - 1; i >= 0; i-- {
		item := heap.Pop(results).(distItem)
		ids[i] = item.id
	}
	return ids
}

// ── Distance functions ────────────────────────────────────────────

func cosineSimilarity(a, b []float32) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}
	var dot, normA, normB float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
		normA += float64(a[i]) * float64(a[i])
		normB += float64(b[i]) * float64(b[i])
	}
	if normA == 0 || normB == 0 {
		return 0
	}
	return dot / (math.Sqrt(normA) * math.Sqrt(normB))
}

func cosineDistance(a, b []float32) float64 {
	return 1.0 - cosineSimilarity(a, b)
}

func euclideanDistance(a, b []float32) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return math.MaxFloat64
	}
	var sum float64
	for i := range a {
		d := float64(a[i]) - float64(b[i])
		sum += d * d
	}
	return math.Sqrt(sum)
}

func dotProductDistance(a, b []float32) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 1
	}
	var dot float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
	}
	return 1.0 - dot // Convert to distance (lower = more similar).
}

// ── Helpers ───────────────────────────────────────────────────────

func selectNearest(candidates []uint64, query []float32, k int, nodes map[uint64]*hnode, distFn func([]float32, []float32) float64) []uint64 {
	if len(candidates) <= k {
		return candidates
	}

	type scored struct {
		id   uint64
		dist float64
	}

	items := make([]scored, 0, len(candidates))
	for _, id := range candidates {
		node, ok := nodes[id]
		if !ok {
			continue
		}
		items = append(items, scored{id: id, dist: distFn(query, node.vector)})
	}

	for i := 0; i < k && i < len(items); i++ {
		minIdx := i
		for j := i + 1; j < len(items); j++ {
			if items[j].dist < items[minIdx].dist {
				minIdx = j
			}
		}
		items[i], items[minIdx] = items[minIdx], items[i]
	}

	result := make([]uint64, 0, k)
	for i := 0; i < k && i < len(items); i++ {
		result = append(result, items[i].id)
	}
	return result
}

func sortResults(results []SearchResult) {
	for i := 1; i < len(results); i++ {
		key := results[i]
		j := i - 1
		for j >= 0 && results[j].Score < key.Score {
			results[j+1] = results[j]
			j--
		}
		results[j+1] = key
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// ── Heap for HNSW search ──────────────────────────────────────────

type distItem struct {
	id   uint64
	dist float64
}

type distHeap []distItem

func (h distHeap) Len() int           { return len(h) }
func (h distHeap) Less(i, j int) bool { return h[i].dist < h[j].dist }
func (h distHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }

func (h *distHeap) Push(x interface{}) {
	*h = append(*h, x.(distItem))
}

func (h *distHeap) Pop() interface{} {
	old := *h
	n := len(old)
	item := old[n-1]
	*h = old[:n-1]
	return item
}
