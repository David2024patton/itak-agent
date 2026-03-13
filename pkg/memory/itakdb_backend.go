package memory

import (
	"fmt"
	"log"
	"path/filepath"
	"time"

	"github.com/David2024patton/iTaKAgent/pkg/debug"
	"github.com/David2024patton/iTaKDatabase/pkg/itakdb"
)

// ITakDBBackend implements GraphBackend using the embedded iTaK Database.
//
// What: Embedded graph database using pure Go storage.
// Why:  Zero Docker, zero network, zero VPS. Everything lives in a single file.
// How:  Wraps the iTakDB graph + vector engine behind the GraphBackend interface.
type ITakDBBackend struct {
	db *itakdb.DB
}

// NewITakDBBackend opens (or creates) an embedded iTaK Database.
func NewITakDBBackend(dataDir string) (*ITakDBBackend, error) {
	dbDir := filepath.Join(dataDir, "itakdb")
	db, err := itakdb.Open(dbDir)
	if err != nil {
		return nil, fmt.Errorf("itakdb backend: %w", err)
	}

	debug.Info("memory.itakdb", "Embedded graph+vector database ready at %s", dbDir)
	return &ITakDBBackend{db: db}, nil
}

func (b *ITakDBBackend) EnsureIndexes() {
	// iTakDB doesn't need explicit index creation -- bbolt has label indexes
	// built in and HNSW is always active.
	log.Printf("[itakdb] Indexes are automatic (bbolt label index + HNSW vector)")
}

func (b *ITakDBBackend) SyncFact(f Fact) {
	b.db.MergeNode("Fact", "key", f.Key, map[string]interface{}{
		"value":      f.Value,
		"category":   f.Category,
		"importance": f.Importance,
		"created_at": f.CreatedAt.Format(time.RFC3339),
	}, nil)
}

func (b *ITakDBBackend) SyncEntity(e Entity) {
	b.db.MergeNode("Entity", "name", e.Name, map[string]interface{}{
		"type":       e.Type,
		"first_seen": e.FirstSeen.Format(time.RFC3339),
		"last_seen":  e.LastSeen.Format(time.RFC3339),
	}, nil)
}

func (b *ITakDBBackend) LinkEntityToConversation(entityName string, convID int) {
	// Find entity node.
	entities, _ := b.db.Graph.FindByProperty("name", entityName)
	if len(entities) == 0 {
		return
	}
	entityNodeID := entities[0].ID

	// Find or create conversation node.
	convNodeID, _, _ := b.db.MergeNode("Conversation", "id", fmt.Sprintf("%d", convID), nil, nil)

	// Create MENTIONED_IN edge.
	b.db.Graph.CreateEdge("MENTIONED_IN", entityNodeID, convNodeID, nil)
}

func (b *ITakDBBackend) TrackSession(sessionID string) {
	b.db.MergeNode("Session", "session_id", sessionID, map[string]interface{}{
		"start_time": time.Now().Format(time.RFC3339),
		"type":       "agent",
	}, nil)
}

func (b *ITakDBBackend) TrackAction(sessionID, agent, tool, args, result string, embedding []float32) {
	// Create Action node.
	actID, _ := b.db.CreateNode([]string{"Action"}, map[string]interface{}{
		"agent":          agent,
		"tool":           tool,
		"args":           args,
		"result_summary": result,
		"timestamp":      time.Now().Format(time.RFC3339),
	}, embedding)

	// Link to session.
	sesNodes, _ := b.db.Graph.FindByProperty("session_id", sessionID)
	if len(sesNodes) > 0 {
		b.db.Graph.CreateEdge("PERFORMED", sesNodes[0].ID, actID, nil)
	}
}

func (b *ITakDBBackend) TrackPage(sessionID, url, title string, embedding []float32) {
	// MERGE the page (same URL may be visited multiple times).
	pageID, _, _ := b.db.MergeNode("Page", "url", url, map[string]interface{}{
		"title":        title,
		"last_visited": time.Now().Format(time.RFC3339),
	}, embedding)

	// Increment visit count.
	page, _ := b.db.Graph.GetNode(pageID)
	if page != nil {
		count := 1
		if existing, ok := page.Properties["visit_count"]; ok {
			if c, ok := existing.(float64); ok {
				count = int(c) + 1
			}
		}
		b.db.Graph.UpdateNode(pageID, map[string]interface{}{"visit_count": count})
	}

	// Link to session.
	sesNodes, _ := b.db.Graph.FindByProperty("session_id", sessionID)
	if len(sesNodes) > 0 {
		b.db.Graph.CreateEdge("VISITED", sesNodes[0].ID, pageID, map[string]interface{}{
			"timestamp": time.Now().Format(time.RFC3339),
		})
	}
}

func (b *ITakDBBackend) TrackSearch(sessionID, query string, resultCount int, source string, embedding []float32) {
	searchID, _ := b.db.CreateNode([]string{"Search"}, map[string]interface{}{
		"query":        query,
		"result_count": resultCount,
		"source":       source,
		"timestamp":    time.Now().Format(time.RFC3339),
	}, embedding)

	sesNodes, _ := b.db.Graph.FindByProperty("session_id", sessionID)
	if len(sesNodes) > 0 {
		b.db.Graph.CreateEdge("SEARCHED", sesNodes[0].ID, searchID, nil)
	}
}

func (b *ITakDBBackend) TrackMessage(sessionID, role, content, agent string, embedding []float32) {
	msgID, _ := b.db.CreateNode([]string{"Message"}, map[string]interface{}{
		"role":      role,
		"content":   content,
		"agent":     agent,
		"timestamp": time.Now().Format(time.RFC3339),
	}, embedding)

	sesNodes, _ := b.db.Graph.FindByProperty("session_id", sessionID)
	if len(sesNodes) > 0 {
		b.db.Graph.CreateEdge("INCLUDES", sesNodes[0].ID, msgID, nil)
	}
}

func (b *ITakDBBackend) TrackBrowserSession(sessionID, browserSessionID string, headed bool) {
	bsID, _ := b.db.CreateNode([]string{"BrowserSession"}, map[string]interface{}{
		"browser_session_id": browserSessionID,
		"headed":             headed,
		"start_time":         time.Now().Format(time.RFC3339),
	}, nil)

	sesNodes, _ := b.db.Graph.FindByProperty("session_id", sessionID)
	if len(sesNodes) > 0 {
		b.db.Graph.CreateEdge("USED_BROWSER", sesNodes[0].ID, bsID, nil)
	}
}

func (b *ITakDBBackend) SemanticSearch(queryEmbed []float32, limit int) ([]map[string]interface{}, error) {
	nodes, scores, err := b.db.SemanticSearch(queryEmbed, limit)
	if err != nil {
		return nil, err
	}

	var results []map[string]interface{}
	for i, node := range nodes {
		label := "Unknown"
		if len(node.Labels) > 0 {
			label = node.Labels[0]
		}
		results = append(results, map[string]interface{}{
			"type":  label,
			"score": scores[i],
			"node":  node.Properties,
		})
	}
	return results, nil
}

// ── DebugMemory ──────────────────────────────────────────────────

func (b *ITakDBBackend) StoreError(sessionID, errorMsg, errorType, source string, embedding []float32) uint64 {
	errID, _ := b.db.CreateNode([]string{"Error"}, map[string]interface{}{
		"message":    errorMsg,
		"error_type": errorType,
		"source":     source,
		"resolved":   false,
		"timestamp":  time.Now().Format(time.RFC3339),
	}, embedding)

	// Link to session.
	sesNodes, _ := b.db.Graph.FindByProperty("session_id", sessionID)
	if len(sesNodes) > 0 {
		b.db.Graph.CreateEdge("ENCOUNTERED", sesNodes[0].ID, errID, nil)
	}

	debug.Info("memory.debug", "Stored error [%s] from %s (node %d)", errorType, source, errID)
	return errID
}

func (b *ITakDBBackend) StoreFix(errorNodeID uint64, fixDescription, fixCode, fixAgent string, embedding []float32) {
	fixID, _ := b.db.CreateNode([]string{"Fix"}, map[string]interface{}{
		"description": fixDescription,
		"code":        fixCode,
		"agent":       fixAgent,
		"timestamp":   time.Now().Format(time.RFC3339),
	}, embedding)

	// Create RESOLVED_BY edge from Error to Fix.
	b.db.Graph.CreateEdge("RESOLVED_BY", errorNodeID, fixID, map[string]interface{}{
		"timestamp": time.Now().Format(time.RFC3339),
	})

	// Mark the error as resolved.
	b.db.Graph.UpdateNode(errorNodeID, map[string]interface{}{"resolved": true})

	debug.Info("memory.debug", "Stored fix for error %d by %s (fix %d)", errorNodeID, fixAgent, fixID)
}

func (b *ITakDBBackend) SearchErrors(queryEmbed []float32, limit int) ([]map[string]interface{}, error) {
	// Do a full semantic search, then filter to Error and Fix labels.
	all, scores, err := b.db.SemanticSearch(queryEmbed, limit*3)
	if err != nil {
		return nil, err
	}

	var results []map[string]interface{}
	for i, node := range all {
		if len(node.Labels) == 0 {
			continue
		}
		label := node.Labels[0]
		if label != "Error" && label != "Fix" {
			continue
		}

		entry := map[string]interface{}{
			"type":  label,
			"score": scores[i],
			"node":  node.Properties,
		}

		// If it's an Error, check for linked Fix nodes.
		if label == "Error" {
			edges, _ := b.db.Graph.GetEdgesFrom(node.ID)
			for _, edge := range edges {
				if edge.Type == "RESOLVED_BY" {
					fixNode, _ := b.db.Graph.GetNode(edge.TargetID)
					if fixNode != nil {
						entry["fix"] = fixNode.Properties
					}
				}
			}
		}

		results = append(results, entry)
		if len(results) >= limit {
			break
		}
	}

	return results, nil
}

// ── WebResearch ──────────────────────────────────────────────────

func (b *ITakDBBackend) StoreResearch(sessionID, url, domain, title, content, findings, topic string, embedding []float32) uint64 {
	// MERGE on URL so revisiting the same page updates it.
	researchID, _, _ := b.db.MergeNode("Research", "url", url, map[string]interface{}{
		"domain":       domain,
		"title":        title,
		"content":      content,
		"findings":     findings,
		"topic":        topic,
		"last_visited": time.Now().Format(time.RFC3339),
	}, embedding)

	// Link to session.
	sesNodes, _ := b.db.Graph.FindByProperty("session_id", sessionID)
	if len(sesNodes) > 0 {
		b.db.Graph.CreateEdge("RESEARCHED", sesNodes[0].ID, researchID, map[string]interface{}{
			"timestamp": time.Now().Format(time.RFC3339),
		})
	}

	// Link to a domain hub node for grouping.
	if domain != "" {
		domID, _, _ := b.db.MergeNode("Domain", "name", domain, map[string]interface{}{
			"last_seen": time.Now().Format(time.RFC3339),
		}, nil)
		b.db.Graph.CreateEdge("FROM_DOMAIN", researchID, domID, nil)
	}

	debug.Info("memory.research", "Stored research from %s: %s", domain, title)
	return researchID
}

func (b *ITakDBBackend) SearchResearch(queryEmbed []float32, limit int) ([]map[string]interface{}, error) {
	all, scores, err := b.db.SemanticSearch(queryEmbed, limit*3)
	if err != nil {
		return nil, err
	}

	var results []map[string]interface{}
	for i, node := range all {
		if len(node.Labels) == 0 {
			continue
		}
		if node.Labels[0] != "Research" {
			continue
		}
		results = append(results, map[string]interface{}{
			"type":  "Research",
			"score": scores[i],
			"node":  node.Properties,
		})
		if len(results) >= limit {
			break
		}
	}

	return results, nil
}

func (b *ITakDBBackend) Close() error {
	return b.db.Close()
}

// DB returns the underlying iTaK Database handle for API access.
func (b *ITakDBBackend) DB() *itakdb.DB {
	return b.db
}
