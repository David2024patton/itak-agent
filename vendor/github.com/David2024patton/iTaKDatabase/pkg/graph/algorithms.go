package graph

import (
	"container/heap"
	"fmt"
	"math"
	"sort"
)

// ── Result Types ─────────────────────────────────────────────────

// RankResult holds a node ID and its computed score from a ranking algorithm.
type RankResult struct {
	NodeID uint64  `json:"node_id"`
	Score  float64 `json:"score"`
}

// DegreeResult holds in-degree, out-degree, and total for a node.
type DegreeResult struct {
	NodeID    uint64 `json:"node_id"`
	InDegree  int    `json:"in_degree"`
	OutDegree int    `json:"out_degree"`
	Total     int    `json:"total"`
}

// ComponentResult groups node IDs into a connected component.
type ComponentResult struct {
	ComponentID int      `json:"component_id"`
	NodeIDs     []uint64 `json:"node_ids"`
	Size        int      `json:"size"`
}

// WeightedPathResult holds a shortest path with total weight.
type WeightedPathResult struct {
	NodeIDs  []uint64 `json:"node_ids"`
	Distance float64  `json:"distance"`
}

// ── PageRank ─────────────────────────────────────────────────────

// PageRank computes the PageRank of every node in the graph.
//
// What: Ranks each node by importance based on link structure.
// Why:  Identifies the most influential nodes in the knowledge graph,
//       helping prioritize which sessions, entities, or facts matter most.
// How:  Iterative power method with damping factor (typically 0.85).
func (s *Store) PageRank(dampingFactor float64, iterations int) ([]RankResult, error) {
	nodes := s.AllNodes()
	edges := s.AllEdges()

	if len(nodes) == 0 {
		return nil, nil
	}

	// Build adjacency: outgoing edges per node.
	outgoing := make(map[uint64][]uint64)
	for _, e := range edges {
		outgoing[e.SourceID] = append(outgoing[e.SourceID], e.TargetID)
	}

	// Initialize scores uniformly.
	n := float64(len(nodes))
	scores := make(map[uint64]float64)
	for _, node := range nodes {
		scores[node.ID] = 1.0 / n
	}

	// Iterate.
	for i := 0; i < iterations; i++ {
		newScores := make(map[uint64]float64)
		// Base score from random jumps.
		base := (1.0 - dampingFactor) / n

		for _, node := range nodes {
			newScores[node.ID] = base
		}

		// Distribute scores through edges.
		for _, node := range nodes {
			out := outgoing[node.ID]
			if len(out) == 0 {
				// Dangling node: distribute equally to all nodes.
				share := dampingFactor * scores[node.ID] / n
				for _, nd := range nodes {
					newScores[nd.ID] += share
				}
			} else {
				share := dampingFactor * scores[node.ID] / float64(len(out))
				for _, targetID := range out {
					newScores[targetID] += share
				}
			}
		}

		scores = newScores
	}

	// Sort by score descending.
	results := make([]RankResult, 0, len(scores))
	for id, score := range scores {
		results = append(results, RankResult{NodeID: id, Score: score})
	}
	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	return results, nil
}

// ── Betweenness Centrality ───────────────────────────────────────

// BetweennessCentrality computes the betweenness centrality of each node.
//
// What: Measures how often a node appears on shortest paths between others.
// Why:  Identifies bridge nodes that connect disparate parts of the graph.
// How:  Brandes' algorithm, O(V*E) for unweighted graphs.
func (s *Store) BetweennessCentrality() ([]RankResult, error) {
	nodes := s.AllNodes()
	edges := s.AllEdges()

	if len(nodes) == 0 {
		return nil, nil
	}

	// Build adjacency list.
	adj := make(map[uint64][]uint64)
	for _, e := range edges {
		adj[e.SourceID] = append(adj[e.SourceID], e.TargetID)
	}

	cb := make(map[uint64]float64) // centrality scores

	for _, src := range nodes {
		// BFS from src.
		stack := []uint64{}
		pred := make(map[uint64][]uint64)
		sigma := make(map[uint64]float64)
		dist := make(map[uint64]int)

		for _, n := range nodes {
			dist[n.ID] = -1
		}
		sigma[src.ID] = 1
		dist[src.ID] = 0

		queue := []uint64{src.ID}

		for len(queue) > 0 {
			v := queue[0]
			queue = queue[1:]
			stack = append(stack, v)

			for _, w := range adj[v] {
				// First visit?
				if dist[w] < 0 {
					dist[w] = dist[v] + 1
					queue = append(queue, w)
				}
				// Shortest path via v?
				if dist[w] == dist[v]+1 {
					sigma[w] += sigma[v]
					pred[w] = append(pred[w], v)
				}
			}
		}

		// Accumulate.
		delta := make(map[uint64]float64)
		for len(stack) > 0 {
			w := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			for _, v := range pred[w] {
				delta[v] += (sigma[v] / sigma[w]) * (1 + delta[w])
			}
			if w != src.ID {
				cb[w] += delta[w]
			}
		}
	}

	results := make([]RankResult, 0, len(cb))
	for id, score := range cb {
		results = append(results, RankResult{NodeID: id, Score: score})
	}
	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	return results, nil
}

// ── Degree Centrality ────────────────────────────────────────────

// DegreeCentrality computes in-degree, out-degree, and total degree for each node.
//
// What: Counts incoming and outgoing edges per node.
// Why:  Quickly identifies hubs (high out-degree) and authorities (high in-degree).
func (s *Store) DegreeCentrality() ([]DegreeResult, error) {
	nodes := s.AllNodes()
	edges := s.AllEdges()

	inDeg := make(map[uint64]int)
	outDeg := make(map[uint64]int)

	for _, e := range edges {
		outDeg[e.SourceID]++
		inDeg[e.TargetID]++
	}

	results := make([]DegreeResult, 0, len(nodes))
	for _, n := range nodes {
		results = append(results, DegreeResult{
			NodeID:    n.ID,
			InDegree:  inDeg[n.ID],
			OutDegree: outDeg[n.ID],
			Total:     inDeg[n.ID] + outDeg[n.ID],
		})
	}
	sort.Slice(results, func(i, j int) bool {
		return results[i].Total > results[j].Total
	})

	return results, nil
}

// ── Connected Components ─────────────────────────────────────────

// ConnectedComponents finds all weakly connected components in the graph.
//
// What: Groups nodes into isolated subgraphs.
// Why:  Detects disconnected clusters (e.g., separate user sessions,
//       orphaned entities, unreachable knowledge).
// How:  Union-Find (disjoint set) on undirected edge projection.
func (s *Store) ConnectedComponents() ([]ComponentResult, error) {
	nodes := s.AllNodes()
	edges := s.AllEdges()

	if len(nodes) == 0 {
		return nil, nil
	}

	// Union-Find.
	parent := make(map[uint64]uint64)
	rank := make(map[uint64]int)
	for _, n := range nodes {
		parent[n.ID] = n.ID
	}

	var find func(uint64) uint64
	find = func(x uint64) uint64 {
		if parent[x] != x {
			parent[x] = find(parent[x])
		}
		return parent[x]
	}

	union := func(a, b uint64) {
		ra, rb := find(a), find(b)
		if ra == rb {
			return
		}
		if rank[ra] < rank[rb] {
			ra, rb = rb, ra
		}
		parent[rb] = ra
		if rank[ra] == rank[rb] {
			rank[ra]++
		}
	}

	for _, e := range edges {
		if _, ok := parent[e.SourceID]; !ok {
			continue
		}
		if _, ok := parent[e.TargetID]; !ok {
			continue
		}
		union(e.SourceID, e.TargetID)
	}

	// Group by root.
	groups := make(map[uint64][]uint64)
	for _, n := range nodes {
		root := find(n.ID)
		groups[root] = append(groups[root], n.ID)
	}

	results := make([]ComponentResult, 0, len(groups))
	cid := 0
	for _, ids := range groups {
		results = append(results, ComponentResult{
			ComponentID: cid,
			NodeIDs:     ids,
			Size:        len(ids),
		})
		cid++
	}
	sort.Slice(results, func(i, j int) bool {
		return results[i].Size > results[j].Size
	})

	return results, nil
}

// ── Label Propagation ────────────────────────────────────────────

// LabelPropagation detects communities by iteratively assigning each node
// the most frequent label among its neighbors.
//
// What: Fast community detection via label spreading.
// Why:  Groups related nodes (e.g., nodes in the same project, topic cluster).
// How:  Each node starts with its own label, then adopts the majority label
//       of its neighbors. Converges in a few iterations.
func (s *Store) LabelPropagation(iterations int) ([]ComponentResult, error) {
	nodes := s.AllNodes()
	edges := s.AllEdges()

	if len(nodes) == 0 {
		return nil, nil
	}

	// Build undirected adjacency.
	adj := make(map[uint64][]uint64)
	for _, e := range edges {
		adj[e.SourceID] = append(adj[e.SourceID], e.TargetID)
		adj[e.TargetID] = append(adj[e.TargetID], e.SourceID)
	}

	// Initialize: each node's label = its own ID.
	labels := make(map[uint64]uint64)
	for _, n := range nodes {
		labels[n.ID] = n.ID
	}

	for iter := 0; iter < iterations; iter++ {
		changed := false
		for _, n := range nodes {
			neighbors := adj[n.ID]
			if len(neighbors) == 0 {
				continue
			}
			// Count neighbor labels.
			freq := make(map[uint64]int)
			for _, nb := range neighbors {
				freq[labels[nb]]++
			}
			// Find max frequency label.
			maxFreq := 0
			maxLabel := labels[n.ID]
			for lbl, cnt := range freq {
				if cnt > maxFreq {
					maxFreq = cnt
					maxLabel = lbl
				}
			}
			if labels[n.ID] != maxLabel {
				labels[n.ID] = maxLabel
				changed = true
			}
		}
		if !changed {
			break
		}
	}

	// Group by label.
	groups := make(map[uint64][]uint64)
	for _, n := range nodes {
		lbl := labels[n.ID]
		groups[lbl] = append(groups[lbl], n.ID)
	}

	results := make([]ComponentResult, 0, len(groups))
	cid := 0
	for _, ids := range groups {
		results = append(results, ComponentResult{
			ComponentID: cid,
			NodeIDs:     ids,
			Size:        len(ids),
		})
		cid++
	}
	sort.Slice(results, func(i, j int) bool {
		return results[i].Size > results[j].Size
	})

	return results, nil
}

// ── Louvain Community Detection ──────────────────────────────────

// LouvainCommunity detects hierarchical communities using the Louvain method.
//
// What: Optimizes modularity to find tightly connected groups.
// Why:  Produces higher quality communities than label propagation,
//       especially for large graphs with hierarchical structure.
// How:  Phase 1: greedily assign nodes to maximize modularity. Phase 2:
//       collapse communities into super-nodes and repeat.
func (s *Store) LouvainCommunity() ([]ComponentResult, error) {
	nodes := s.AllNodes()
	edges := s.AllEdges()

	if len(nodes) == 0 {
		return nil, nil
	}

	// Build weighted adjacency (weight = 1 for each edge, accumulated).
	weight := make(map[uint64]map[uint64]float64)
	degree := make(map[uint64]float64)
	totalWeight := 0.0

	for _, e := range edges {
		if weight[e.SourceID] == nil {
			weight[e.SourceID] = make(map[uint64]float64)
		}
		if weight[e.TargetID] == nil {
			weight[e.TargetID] = make(map[uint64]float64)
		}
		weight[e.SourceID][e.TargetID] += 1.0
		weight[e.TargetID][e.SourceID] += 1.0
		degree[e.SourceID] += 1.0
		degree[e.TargetID] += 1.0
		totalWeight += 1.0
	}

	if totalWeight == 0 {
		// No edges, each node is its own community.
		results := make([]ComponentResult, len(nodes))
		for i, n := range nodes {
			results[i] = ComponentResult{ComponentID: i, NodeIDs: []uint64{n.ID}, Size: 1}
		}
		return results, nil
	}

	m2 := totalWeight * 2.0 // 2*m for modularity

	// Initialize: each node in its own community.
	community := make(map[uint64]uint64)
	for _, n := range nodes {
		community[n.ID] = n.ID
	}

	// Phase 1: Local modularity optimization.
	improved := true
	for improved {
		improved = false
		for _, n := range nodes {
			bestComm := community[n.ID]
			bestGain := 0.0

			// Remove n from its community temporarily.
			oldComm := community[n.ID]

			// Compute modularity gain for each neighbor community.
			neighborComms := make(map[uint64]float64)
			for nb, w := range weight[n.ID] {
				c := community[nb]
				neighborComms[c] += w
			}

			ki := degree[n.ID]
			for c, kiIn := range neighborComms {
				if c == oldComm {
					continue
				}
				// Compute sum of degrees in community c.
				sigTot := 0.0
				for _, nd := range nodes {
					if community[nd.ID] == c {
						sigTot += degree[nd.ID]
					}
				}
				// Modularity gain.
				gain := (kiIn/m2) - (sigTot*ki)/(m2*m2)*2.0
				if gain > bestGain {
					bestGain = gain
					bestComm = c
				}
			}

			if bestComm != oldComm {
				community[n.ID] = bestComm
				improved = true
			}
		}
	}

	// Group by community.
	groups := make(map[uint64][]uint64)
	for _, n := range nodes {
		c := community[n.ID]
		groups[c] = append(groups[c], n.ID)
	}

	results := make([]ComponentResult, 0, len(groups))
	cid := 0
	for _, ids := range groups {
		results = append(results, ComponentResult{
			ComponentID: cid,
			NodeIDs:     ids,
			Size:        len(ids),
		})
		cid++
	}
	sort.Slice(results, func(i, j int) bool {
		return results[i].Size > results[j].Size
	})

	return results, nil
}

// ── Dijkstra's Shortest Path ─────────────────────────────────────

// dijkstraItem is a priority queue item for Dijkstra's algorithm.
type dijkstraItem struct {
	nodeID uint64
	dist   float64
	path   []uint64
	index  int
}

type dijkstraHeap []*dijkstraItem

func (h dijkstraHeap) Len() int            { return len(h) }
func (h dijkstraHeap) Less(i, j int) bool  { return h[i].dist < h[j].dist }
func (h dijkstraHeap) Swap(i, j int)       { h[i], h[j] = h[j], h[i]; h[i].index = i; h[j].index = j }
func (h *dijkstraHeap) Push(x interface{}) { item := x.(*dijkstraItem); item.index = len(*h); *h = append(*h, item) }
func (h *dijkstraHeap) Pop() interface{}   { old := *h; n := len(old); item := old[n-1]; old[n-1] = nil; item.index = -1; *h = old[:n-1]; return item }

// DijkstraShortestPath finds the shortest weighted path between two nodes.
//
// What: Weighted shortest path using edge property as weight.
// Why:  Path cost matters when edges have different weights (e.g., latency, cost).
// How:  Priority queue-based Dijkstra. Weight defaults to 1.0 if property missing.
func (s *Store) DijkstraShortestPath(fromID, toID uint64, weightProp string) (*WeightedPathResult, error) {
	edges := s.AllEdges()

	// Build weighted adjacency.
	adj := make(map[uint64][]struct {
		target uint64
		weight float64
	})
	for _, e := range edges {
		w := 1.0
		if weightProp != "" {
			if val, ok := e.Properties[weightProp]; ok {
				switch v := val.(type) {
				case float64:
					w = v
				case int:
					w = float64(v)
				}
			}
		}
		adj[e.SourceID] = append(adj[e.SourceID], struct {
			target uint64
			weight float64
		}{e.TargetID, w})
	}

	// Dijkstra.
	dist := make(map[uint64]float64)
	dist[fromID] = 0

	h := &dijkstraHeap{}
	heap.Init(h)
	heap.Push(h, &dijkstraItem{nodeID: fromID, dist: 0, path: []uint64{fromID}})

	for h.Len() > 0 {
		current := heap.Pop(h).(*dijkstraItem)

		if current.nodeID == toID {
			return &WeightedPathResult{
				NodeIDs:  current.path,
				Distance: current.dist,
			}, nil
		}

		if d, ok := dist[current.nodeID]; ok && current.dist > d {
			continue
		}

		for _, neighbor := range adj[current.nodeID] {
			newDist := current.dist + neighbor.weight
			if d, ok := dist[neighbor.target]; !ok || newDist < d {
				dist[neighbor.target] = newDist
				newPath := make([]uint64, len(current.path)+1)
				copy(newPath, current.path)
				newPath[len(current.path)] = neighbor.target
				heap.Push(h, &dijkstraItem{nodeID: neighbor.target, dist: newDist, path: newPath})
			}
		}
	}

	return nil, fmt.Errorf("no path from %d to %d", fromID, toID)
}

// ── All Shortest Paths ───────────────────────────────────────────

// AllShortestPaths finds all shortest paths between two nodes.
//
// What: Returns every shortest path (same distance) between two nodes.
// Why:  Sometimes multiple equally short paths exist; knowing all of them
//       reveals alternative routes through the knowledge graph.
func (s *Store) AllShortestPaths(fromID, toID uint64) ([]WeightedPathResult, error) {
	edges := s.AllEdges()

	// Build adjacency.
	adj := make(map[uint64][]uint64)
	for _, e := range edges {
		adj[e.SourceID] = append(adj[e.SourceID], e.TargetID)
	}

	// BFS to find shortest distance first.
	dist := make(map[uint64]int)
	dist[fromID] = 0
	queue := []uint64{fromID}
	found := false

	for len(queue) > 0 && !found {
		v := queue[0]
		queue = queue[1:]
		for _, nb := range adj[v] {
			if _, ok := dist[nb]; !ok {
				dist[nb] = dist[v] + 1
				queue = append(queue, nb)
				if nb == toID {
					found = true
				}
			}
		}
	}

	if !found {
		return nil, fmt.Errorf("no path from %d to %d", fromID, toID)
	}

	// DFS to collect all paths of exactly shortest distance.
	targetDist := dist[toID]
	var results []WeightedPathResult

	var dfs func(current uint64, path []uint64, depth int)
	dfs = func(current uint64, path []uint64, depth int) {
		if depth > targetDist {
			return
		}
		if current == toID && depth == targetDist {
			pathCopy := make([]uint64, len(path))
			copy(pathCopy, path)
			results = append(results, WeightedPathResult{
				NodeIDs:  pathCopy,
				Distance: float64(targetDist),
			})
			return
		}
		for _, nb := range adj[current] {
			if d, ok := dist[nb]; ok && d == depth+1 {
				dfs(nb, append(path, nb), depth+1)
			}
		}
	}
	dfs(fromID, []uint64{fromID}, 0)

	return results, nil
}

// ── Depth-First Search ───────────────────────────────────────────

// DFSVisitor is a callback invoked for each node during DFS traversal.
type DFSVisitor func(node Node, depth int) bool // return false to stop

// DepthFirstSearch traverses the graph depth-first from a starting node.
//
// What: DFS traversal with visitor pattern.
// Why:  Flexible traversal for custom logic (e.g., find cycles, reachability).
func (s *Store) DepthFirstSearch(startID uint64, maxDepth int, visitor DFSVisitor) error {
	edges := s.AllEdges()

	adj := make(map[uint64][]uint64)
	for _, e := range edges {
		adj[e.SourceID] = append(adj[e.SourceID], e.TargetID)
	}

	visited := make(map[uint64]bool)

	var dfs func(nodeID uint64, depth int) bool
	dfs = func(nodeID uint64, depth int) bool {
		if visited[nodeID] || depth > maxDepth {
			return true
		}
		visited[nodeID] = true

		node, err := s.GetNode(nodeID)
		if err != nil {
			return true
		}

		if !visitor(*node, depth) {
			return false
		}

		for _, nb := range adj[nodeID] {
			if !dfs(nb, depth+1) {
				return false
			}
		}
		return true
	}

	dfs(startID, 0)
	return nil
}

// ── Topological Sort ─────────────────────────────────────────────

// TopologicalSort returns nodes in topological order (dependencies first).
//
// What: Linear ordering where each node comes before nodes it points to.
// Why:  Essential for dependency resolution (e.g., file import order,
//       build pipeline ordering).
// How:  Kahn's algorithm using in-degree counting.
func (s *Store) TopologicalSort() ([]uint64, error) {
	nodes := s.AllNodes()
	edges := s.AllEdges()

	inDeg := make(map[uint64]int)
	adj := make(map[uint64][]uint64)

	for _, n := range nodes {
		inDeg[n.ID] = 0
	}
	for _, e := range edges {
		adj[e.SourceID] = append(adj[e.SourceID], e.TargetID)
		inDeg[e.TargetID]++
	}

	// Collect nodes with no incoming edges.
	var queue []uint64
	for _, n := range nodes {
		if inDeg[n.ID] == 0 {
			queue = append(queue, n.ID)
		}
	}

	var sorted []uint64
	for len(queue) > 0 {
		v := queue[0]
		queue = queue[1:]
		sorted = append(sorted, v)

		for _, nb := range adj[v] {
			inDeg[nb]--
			if inDeg[nb] == 0 {
				queue = append(queue, nb)
			}
		}
	}

	// If sorted doesn't contain all nodes, the graph has cycles.
	if len(sorted) != len(nodes) {
		return sorted, fmt.Errorf("graph contains cycles; partial sort returned %d of %d nodes", len(sorted), len(nodes))
	}

	return sorted, nil
}

// ── Triangle Count ───────────────────────────────────────────────

// TriangleCount counts the number of triangles in the graph.
//
// What: Counts sets of three mutually connected nodes.
// Why:  High triangle count indicates dense clustering (tight communities).
//       The clustering coefficient = 3*triangles / connected_triples.
func (s *Store) TriangleCount() (int, map[uint64]int, error) {
	edges := s.AllEdges()

	// Build undirected neighbor set.
	neighbors := make(map[uint64]map[uint64]bool)
	for _, e := range edges {
		if neighbors[e.SourceID] == nil {
			neighbors[e.SourceID] = make(map[uint64]bool)
		}
		if neighbors[e.TargetID] == nil {
			neighbors[e.TargetID] = make(map[uint64]bool)
		}
		neighbors[e.SourceID][e.TargetID] = true
		neighbors[e.TargetID][e.SourceID] = true
	}

	totalTriangles := 0
	nodeTriangles := make(map[uint64]int)

	for u, uNeighbors := range neighbors {
		for v := range uNeighbors {
			if v <= u {
				continue // avoid double counting
			}
			for w := range neighbors[v] {
				if w <= v {
					continue
				}
				if uNeighbors[w] {
					totalTriangles++
					nodeTriangles[u]++
					nodeTriangles[v]++
					nodeTriangles[w]++
				}
			}
		}
	}

	return totalTriangles, nodeTriangles, nil
}

// ── Common Neighbors ─────────────────────────────────────────────

// CommonNeighbors returns the set of nodes that are neighbors of both a and b.
//
// What: Intersection of neighbor sets.
// Why:  Link prediction -- nodes with many common neighbors are likely related.
func (s *Store) CommonNeighbors(aID, bID uint64) ([]uint64, error) {
	edges := s.AllEdges()

	// Build undirected neighbor sets.
	neighbors := make(map[uint64]map[uint64]bool)
	for _, e := range edges {
		if neighbors[e.SourceID] == nil {
			neighbors[e.SourceID] = make(map[uint64]bool)
		}
		if neighbors[e.TargetID] == nil {
			neighbors[e.TargetID] = make(map[uint64]bool)
		}
		neighbors[e.SourceID][e.TargetID] = true
		neighbors[e.TargetID][e.SourceID] = true
	}

	aNb := neighbors[aID]
	bNb := neighbors[bID]
	if aNb == nil || bNb == nil {
		return nil, nil
	}

	var common []uint64
	for nb := range aNb {
		if bNb[nb] {
			common = append(common, nb)
		}
	}
	return common, nil
}

// ── Jaccard Similarity ───────────────────────────────────────────

// JaccardSimilarity computes the Jaccard coefficient of the neighbor sets of two nodes.
//
// What: |intersection| / |union| of neighbor sets.
// Why:  Normalized similarity metric (0 to 1). Higher = more similar neighborhoods.
func (s *Store) JaccardSimilarity(aID, bID uint64) (float64, error) {
	edges := s.AllEdges()

	neighbors := make(map[uint64]map[uint64]bool)
	for _, e := range edges {
		if neighbors[e.SourceID] == nil {
			neighbors[e.SourceID] = make(map[uint64]bool)
		}
		if neighbors[e.TargetID] == nil {
			neighbors[e.TargetID] = make(map[uint64]bool)
		}
		neighbors[e.SourceID][e.TargetID] = true
		neighbors[e.TargetID][e.SourceID] = true
	}

	aNb := neighbors[aID]
	bNb := neighbors[bID]

	if len(aNb) == 0 && len(bNb) == 0 {
		return 0, nil
	}

	// Union and intersection.
	union := make(map[uint64]bool)
	intersection := 0
	for nb := range aNb {
		union[nb] = true
	}
	for nb := range bNb {
		if union[nb] {
			intersection++
		}
		union[nb] = true
	}

	if len(union) == 0 {
		return 0, nil
	}

	return float64(intersection) / float64(len(union)), nil
}

// Ensure math import is used (for potential future use).
var _ = math.MaxFloat64
