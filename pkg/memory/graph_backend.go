package memory

// GraphBackend is the provider-agnostic interface for graph database operations.
//
// What: Abstraction layer for the iTaK Database engine.
// Why:  Decouples the memory subsystems from the storage implementation,
//       making the graph operations testable and swappable.
// How:  The iTakDBBackend implements this interface. All memory subsystems
//       (facts, entities, archive, activity tracker) use this interface.
type GraphBackend interface {
	// SyncFact creates or updates a Fact node.
	SyncFact(f Fact)

	// SyncEntity creates or updates an Entity node.
	SyncEntity(e Entity)

	// LinkEntityToConversation creates a MENTIONED_IN relationship.
	LinkEntityToConversation(entityName string, convID int)

	// TrackSession records a session start with a human-readable title.
	TrackSession(sessionID, title string)

	// TrackAction records a tool call.
	TrackAction(sessionID, agent, tool, args, result string, embedding []float32)

	// TrackPage records a visited URL.
	TrackPage(sessionID, url, title string, embedding []float32)

	// TrackSearch records a search query.
	TrackSearch(sessionID, query string, resultCount int, source string, embedding []float32)

	// TrackMessage records a conversation message.
	TrackMessage(sessionID, role, content, agent string, embedding []float32)

	// TrackBrowserSession records a browser session start.
	TrackBrowserSession(sessionID, browserSessionID string, headed bool)

	// SemanticSearch finds nodes similar to the query embedding.
	SemanticSearch(queryEmbed []float32, limit int) ([]map[string]interface{}, error)

	// ── DebugMemory ─────────────────────────────────────────────

	// StoreError records an error that occurred during agent operation.
	// Returns the node ID so a fix can be linked later.
	StoreError(sessionID, errorMsg, errorType, source string, embedding []float32) uint64

	// StoreFix records the fix that resolved an error, creating a RESOLVED_BY edge.
	StoreFix(errorNodeID uint64, fixDescription, fixCode, fixAgent string, embedding []float32)

	// SearchErrors finds previous errors similar to the query (for solution lookup).
	SearchErrors(queryEmbed []float32, limit int) ([]map[string]interface{}, error)

	// ── WebResearch ─────────────────────────────────────────────

	// StoreResearch records research from a website visit.
	StoreResearch(sessionID, url, domain, title, content, findings, topic string, embedding []float32) uint64

	// SearchResearch finds previous research by topic or domain.
	SearchResearch(queryEmbed []float32, limit int) ([]map[string]interface{}, error)

	// EnsureIndexes creates any needed indexes at startup.
	EnsureIndexes()

	// Close shuts down the backend.
	Close() error
}
