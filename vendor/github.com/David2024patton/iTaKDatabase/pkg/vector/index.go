package vector

import (
	"container/heap"
	"math"
	"math/rand"
	"sync"
)

// Index is a pure-Go HNSW (Hierarchical Navigable Small World) vector index.
//
// What: In-memory approximate nearest neighbor search for embedding vectors.
// Why:  The agent needs fast semantic search across thousands of graph nodes.
//       HNSW provides O(log N) search with >95% recall on typical workloads.
// How:  Multi-layer skip-list graph where each layer is a navigable small world.
//       Vectors are connected to their nearest neighbors at insertion time.
//       Search descends from top layer to bottom, greedily navigating toward
//       the query vector at each layer.
type Index struct {
	mu         sync.RWMutex
	nodes      map[uint64]*hnode    // nodeID -> vector + connections
	dimensions int
	maxLevel   int
	efConstruction int              // ef parameter for construction
	efSearch   int                  // ef parameter for search
	mMax       int                  // max connections per layer
	mMax0      int                  // max connections at layer 0
	entryPoint uint64
	hasEntry   bool
	rng        *rand.Rand
}

type hnode struct {
	id         uint64
	vector     []float32
	level      int
	neighbors  [][]uint64           // neighbors[layer] = list of neighbor IDs
}

// NewIndex creates a new HNSW vector index.
func NewIndex(dimensions int) *Index {
	return &Index{
		nodes:          make(map[uint64]*hnode),
		dimensions:     dimensions,
		maxLevel:       0,
		efConstruction: 200,
		efSearch:       50,
		mMax:           16,
		mMax0:          32,
		rng:            rand.New(rand.NewSource(42)),
	}
}

// Insert adds a vector with the given ID to the index.
func (idx *Index) Insert(id uint64, vector []float32) {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	if len(vector) != idx.dimensions && idx.dimensions > 0 {
		return // dimension mismatch
	}
	if idx.dimensions == 0 {
		idx.dimensions = len(vector)
	}

	// Assign a random level using exponential distribution.
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

	if !idx.hasEntry {
		idx.entryPoint = id
		idx.hasEntry = true
		idx.maxLevel = level
		return
	}

	// Find closest nodes from top layer down to level+1.
	ep := idx.entryPoint
	for l := idx.maxLevel; l > level; l-- {
		ep = idx.greedyClosest(vector, ep, l)
	}

	// For each layer from min(level, maxLevel) down to 0, find and connect neighbors.
	for l := min(level, idx.maxLevel); l >= 0; l-- {
		candidates := idx.searchLayer(vector, ep, idx.efConstruction, l)

		maxConn := idx.mMax
		if l == 0 {
			maxConn = idx.mMax0
		}

		// Select M closest candidates.
		selected := selectNearest(candidates, vector, maxConn, idx.nodes)

		node.neighbors[l] = selected

		// Add bidirectional connections.
		for _, neighborID := range selected {
			neighbor, ok := idx.nodes[neighborID]
			if !ok || l >= len(neighbor.neighbors) {
				continue
			}
			neighbor.neighbors[l] = append(neighbor.neighbors[l], id)

			// Prune if too many connections.
			if len(neighbor.neighbors[l]) > maxConn {
				neighbor.neighbors[l] = selectNearest(neighbor.neighbors[l], neighbor.vector, maxConn, idx.nodes)
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

// SearchResult holds a search result with its similarity score.
type SearchResult struct {
	ID    uint64  `json:"id"`
	Score float64 `json:"score"`  // cosine similarity (0.0 to 1.0)
}

// Search finds the K nearest neighbors to the query vector.
func (idx *Index) Search(query []float32, k int) []SearchResult {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	if !idx.hasEntry || len(idx.nodes) == 0 {
		return nil
	}

	// Navigate from top layer to layer 1.
	ep := idx.entryPoint
	for l := idx.maxLevel; l > 0; l-- {
		ep = idx.greedyClosest(query, ep, l)
	}

	// Search layer 0 with ef parameter.
	candidates := idx.searchLayer(query, ep, max(idx.efSearch, k), 0)

	// Build results sorted by score.
	var results []SearchResult
	for _, candidateID := range candidates {
		node, ok := idx.nodes[candidateID]
		if !ok {
			continue
		}
		score := cosineSimilarity(query, node.vector)
		results = append(results, SearchResult{
			ID:    candidateID,
			Score: score,
		})
	}

	// Sort by score descending.
	sortResults(results)

	if len(results) > k {
		results = results[:k]
	}
	return results
}

// Size returns the number of vectors in the index.
func (idx *Index) Size() int {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	return len(idx.nodes)
}

// Delete removes a vector from the index.
func (idx *Index) Delete(id uint64) {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	node, ok := idx.nodes[id]
	if !ok {
		return
	}

	// Remove from neighbors' lists.
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

	// Update entry point if needed.
	if idx.entryPoint == id {
		if len(idx.nodes) == 0 {
			idx.hasEntry = false
			idx.maxLevel = 0
		} else {
			// Pick any remaining node.
			for nid := range idx.nodes {
				idx.entryPoint = nid
				break
			}
		}
	}
}

// ── Internal HNSW algorithms ──────────────────────────────────────

func (idx *Index) greedyClosest(query []float32, ep uint64, level int) uint64 {
	current := ep
	currentDist := cosineDistance(query, idx.nodes[current].vector)

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
			dist := cosineDistance(query, neighbor.vector)
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
	visited := map[uint64]bool{ep: true}

	candidates := &distHeap{}
	results := &distHeap{}

	epDist := cosineDistance(query, idx.nodes[ep].vector)
	heap.Push(candidates, distItem{id: ep, dist: epDist})
	heap.Push(results, distItem{id: ep, dist: -epDist}) // max-heap for results

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

			dist := cosineDistance(query, neighbor.vector)

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

	// Extract result IDs.
	ids := make([]uint64, results.Len())
	for i := results.Len() - 1; i >= 0; i-- {
		item := heap.Pop(results).(distItem)
		ids[i] = item.id
	}
	return ids
}

// ── Math helpers ──────────────────────────────────────────────────

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

func selectNearest(candidates []uint64, query []float32, k int, nodes map[uint64]*hnode) []uint64 {
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
		items = append(items, scored{id: id, dist: cosineDistance(query, node.vector)})
	}

	// Simple selection sort for small k.
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
	// Simple insertion sort (results are usually small).
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
