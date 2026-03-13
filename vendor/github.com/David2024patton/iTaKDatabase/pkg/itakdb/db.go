// Package itakdb provides a unified interface to the iTaK Database,
// combining graph storage (bbolt), vector search (HNSW), SQL-like tables,
// and full-text search into a single embeddable database.
package itakdb

import (
	"fmt"
	"log"
	"path/filepath"

	"github.com/David2024patton/iTaKDatabase/pkg/graph"
	"github.com/David2024patton/iTaKDatabase/pkg/search"
	"github.com/David2024patton/iTaKDatabase/pkg/table"
	"github.com/David2024patton/iTaKDatabase/pkg/vector"
)

// DB is the top-level iTaK Database handle.
//
// What: A unified graph + vector + table + full-text database.
// Why:  One embedded Go binary covers everything an AI agent needs:
//       graph relationships, vector similarity, structured tables,
//       and keyword search. Zero Docker. Zero network.
// How:  All engines share a single bbolt file for durability.
//       The HNSW vector index lives in memory, rebuilt from persisted
//       graph embeddings on startup.
type DB struct {
	Graph  *graph.Store
	Vector *vector.Index
	Table  *table.Engine
	Search *search.Engine
	path   string
}

// Open creates or opens an iTaK Database at the given directory.
// Creates the directory if it doesn't exist.
func Open(dir string) (*DB, error) {
	graphPath := filepath.Join(dir, "graph.db")

	g, err := graph.Open(graphPath)
	if err != nil {
		return nil, fmt.Errorf("itakdb: graph open: %w", err)
	}

	// All engines share the same bbolt instance.
	boltDB := g.BoltDB()

	// Create HNSW index (dimensions auto-detected on first insert).
	v := vector.NewIndex(0)

	// SQL-like table engine.
	t := table.NewEngine(boltDB)

	// Full-text search engine (loads stats from disk on init).
	s := search.NewEngine(boltDB)

	db := &DB{
		Graph:  g,
		Vector: v,
		Table:  t,
		Search: s,
		path:   dir,
	}

	// Rebuild vector index from persisted graph embeddings.
	rebuilt := db.rebuildVectorIndex()
	if rebuilt > 0 {
		log.Printf("[itakdb] Rebuilt vector index with %d embeddings", rebuilt)
	}

	stats := g.Stats()
	log.Printf("[itakdb] Ready (nodes: %d, edges: %d, vectors: %d, tables: %d, fts docs: %d, path: %s)",
		stats.NodeCount, stats.EdgeCount, v.Size(), tableCount(t), s.DocumentCount(), dir)

	return db, nil
}

// Close shuts down the database and flushes all data.
func (db *DB) Close() error {
	return db.Graph.Close()
}

// CreateNode creates a node with optional embedding, and indexes the vector.
func (db *DB) CreateNode(labels []string, props map[string]interface{}, embedding []float32) (uint64, error) {
	id, err := db.Graph.CreateNode(labels, props, embedding)
	if err != nil {
		return 0, err
	}

	// Index the embedding if present.
	if len(embedding) > 0 {
		db.Vector.Insert(id, embedding)
	}

	return id, nil
}

// MergeNode does MERGE semantics and updates the vector index.
func (db *DB) MergeNode(label, matchKey, matchValue string, props map[string]interface{}, embedding []float32) (uint64, bool, error) {
	id, created, err := db.Graph.MergeNode(label, matchKey, matchValue, props, embedding)
	if err != nil {
		return id, created, err
	}

	// Update vector index.
	if len(embedding) > 0 {
		if !created {
			db.Vector.Delete(id) // remove old vector
		}
		db.Vector.Insert(id, embedding)
	}

	return id, created, nil
}

// SemanticSearch finds the K nodes most similar to the query vector.
func (db *DB) SemanticSearch(query []float32, k int) ([]graph.Node, []float64, error) {
	results := db.Vector.Search(query, k)
	if len(results) == 0 {
		return nil, nil, nil
	}

	nodes := make([]graph.Node, 0, len(results))
	scores := make([]float64, 0, len(results))

	for _, r := range results {
		node, err := db.Graph.GetNode(r.ID)
		if err != nil {
			continue
		}
		nodes = append(nodes, *node)
		scores = append(scores, r.Score)
	}

	return nodes, scores, nil
}

// Stats returns combined database statistics.
type Stats struct {
	graph.Stats
	VectorCount int `json:"vector_count"`
	TableCount  int `json:"table_count"`
	FTSDocCount int `json:"fts_doc_count"`
}

func (db *DB) Stats() Stats {
	gs := db.Graph.Stats()
	return Stats{
		Stats:       gs,
		VectorCount: db.Vector.Size(),
		TableCount:  tableCount(db.Table),
		FTSDocCount: db.Search.DocumentCount(),
	}
}

// rebuildVectorIndex loads all embeddings from the graph and inserts them into HNSW.
func (db *DB) rebuildVectorIndex() int {
	count := 0

	labels := []string{"Action", "Page", "Search", "Message", "Fact", "Entity", "Session", "BrowserSession"}
	for _, label := range labels {
		nodes, err := db.Graph.FindByLabel(label)
		if err != nil {
			continue
		}
		for _, node := range nodes {
			if len(node.Embedding) > 0 {
				db.Vector.Insert(node.ID, node.Embedding)
				count++
			}
		}
	}

	return count
}

func tableCount(t *table.Engine) int {
	tables, _ := t.ListTables()
	return len(tables)
}
