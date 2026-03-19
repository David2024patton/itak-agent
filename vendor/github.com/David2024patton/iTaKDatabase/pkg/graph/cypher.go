package graph

import (
	"encoding/json"
	"fmt"
	"strings"

	bolt "go.etcd.io/bbolt"
)

// ── Cypher-Lite Query Language ───────────────────────────────────
//
// What: A simplified subset of Neo4j's Cypher query language.
// Why:  Gives agents and API consumers a familiar, powerful way to query
//       the graph without writing Go code.
// How:  Recursive-descent parser that translates Cypher patterns into
//       Store method calls.
//
// Supported syntax:
//   MATCH (n:Label {prop: value}) RETURN n
//   MATCH (n:Label)-[:REL_TYPE]->(m) RETURN m
//   MATCH (n:Label)-[:REL_TYPE]->(m) WHERE n.prop = value RETURN m.name
//   CREATE (n:Label {prop: value}) RETURN n
//   MATCH (n:Label {name: 'foo'}) SET n.age = 30
//   MATCH (n:Label {name: 'foo'}) DELETE n
//   MATCH (n:Label) RETURN n LIMIT 10
//   MATCH (n:Label) RETURN COUNT(n)

// CypherResult holds the result of a Cypher query execution.
type CypherResult struct {
	Nodes   []Node                   `json:"nodes,omitempty"`
	Edges   []Edge                   `json:"edges,omitempty"`
	Data    []map[string]interface{} `json:"data,omitempty"`
	Count   int                      `json:"count"`
	Message string                   `json:"message"`
}

// cypherStatement is the parsed AST of a Cypher query.
type cypherStatement struct {
	Type       string // MATCH, CREATE, DELETE
	NodeVar    string // variable name (e.g., "n")
	NodeLabel  string // label filter
	NodeProps  map[string]interface{} // inline property filter
	EdgeType   string // relationship type filter
	EdgeDir    string // "out" (->), "in" (<-), "both" (--)
	TargetVar  string // target variable for patterns
	TargetLabel string
	Where      []cypherWhere
	Returns    []string // field names or "*"
	SetProps   map[string]interface{}
	IsDelete   bool
	IsCount    bool
	Limit      int
}

type cypherWhere struct {
	Variable string // e.g., "n"
	Field    string // e.g., "name"
	Op       string // "=", "!=", "<", ">", "<=", ">="
	Value    interface{}
}

// ExecuteCypher parses and executes a Cypher-lite query string.
func (s *Store) ExecuteCypher(query string) (*CypherResult, error) {
	stmt, err := parseCypher(query)
	if err != nil {
		return nil, fmt.Errorf("cypher parse error: %w", err)
	}

	switch stmt.Type {
	case "MATCH":
		return s.executeCypherMatch(stmt)
	case "CREATE":
		return s.executeCypherCreate(stmt)
	default:
		return nil, fmt.Errorf("unsupported cypher statement type: %s", stmt.Type)
	}
}

// ── Parser ───────────────────────────────────────────────────────

func parseCypher(query string) (*cypherStatement, error) {
	tokens := tokenizeCypher(query)
	if len(tokens) == 0 {
		return nil, fmt.Errorf("empty query")
	}

	stmt := &cypherStatement{
		NodeProps: make(map[string]interface{}),
		SetProps:  make(map[string]interface{}),
		Limit:    -1,
	}

	pos := 0
	peek := func() string {
		if pos >= len(tokens) {
			return ""
		}
		return tokens[pos]
	}
	advance := func() string {
		t := peek()
		pos++
		return t
	}
	expect := func(s string) error {
		t := advance()
		if !strings.EqualFold(t, s) {
			return fmt.Errorf("expected %q, got %q", s, t)
		}
		return nil
	}

	keyword := strings.ToUpper(advance())
	stmt.Type = keyword

	switch keyword {
	case "MATCH", "CREATE":
		// Parse node pattern: (n:Label {prop: value})
		if err := expect("("); err != nil {
			return nil, err
		}

		// Variable name.
		stmt.NodeVar = advance()

		// Optional label.
		if peek() == ":" {
			advance() // consume ":"
			stmt.NodeLabel = advance()
		}

		// Optional inline properties.
		if peek() == "{" {
			advance() // consume "{"
			for peek() != "}" && peek() != "" {
				key := advance()
				if err := expect(":"); err != nil {
					return nil, err
				}
				val := parseCypherValue(advance())
				stmt.NodeProps[key] = val
				if peek() == "," {
					advance()
				}
			}
			advance() // consume "}"
		}

		if err := expect(")"); err != nil {
			return nil, err
		}

		// Optional relationship pattern: -[:TYPE]->(m:Label)
		if peek() == "-" {
			advance() // consume "-"
			dir := "out"

			if peek() == "[" {
				advance() // consume "["
				if peek() == ":" {
					advance() // consume ":"
					stmt.EdgeType = advance()
				}
				if err := expect("]"); err != nil {
					return nil, err
				}
			}

			if peek() == "-" {
				advance()
				if peek() == ">" {
					advance()
					dir = "out"
				}
			} else if peek() == ">" {
				advance()
				dir = "out"
			}
			stmt.EdgeDir = dir

			// Target node.
			if peek() == "(" {
				advance()
				stmt.TargetVar = advance()
				if peek() == ":" {
					advance()
					stmt.TargetLabel = advance()
				}
				if err := expect(")"); err != nil {
					return nil, err
				}
			}
		}

		// WHERE clause.
		if strings.EqualFold(peek(), "WHERE") {
			advance()
			for {
				varField := advance() // "n.name"
				parts := strings.SplitN(varField, ".", 2)
				if len(parts) != 2 {
					return nil, fmt.Errorf("expected variable.field in WHERE, got %q", varField)
				}
				op := advance()
				val := parseCypherValue(advance())
				stmt.Where = append(stmt.Where, cypherWhere{
					Variable: parts[0], Field: parts[1], Op: op, Value: val,
				})
				if !strings.EqualFold(peek(), "AND") {
					break
				}
				advance() // consume AND
			}
		}

		// RETURN / SET / DELETE.
		if strings.EqualFold(peek(), "RETURN") {
			advance()
			if strings.EqualFold(peek(), "COUNT") {
				advance()
				if peek() == "(" {
					advance()
					advance() // variable
					advance() // ")"
				}
				stmt.IsCount = true
			} else {
				for {
					ret := advance()
					stmt.Returns = append(stmt.Returns, ret)
					if peek() != "," {
						break
					}
					advance()
				}
			}
		}

		if strings.EqualFold(peek(), "SET") {
			advance()
			for {
				varField := advance()
				parts := strings.SplitN(varField, ".", 2)
				if len(parts) != 2 {
					return nil, fmt.Errorf("expected variable.field in SET, got %q", varField)
				}
				if err := expect("="); err != nil {
					return nil, err
				}
				val := parseCypherValue(advance())
				stmt.SetProps[parts[1]] = val
				if peek() != "," {
					break
				}
				advance()
			}
		}

		if strings.EqualFold(peek(), "DELETE") {
			advance()
			advance() // variable name
			stmt.IsDelete = true
		}

		if strings.EqualFold(peek(), "LIMIT") {
			advance()
			limitStr := advance()
			fmt.Sscanf(limitStr, "%d", &stmt.Limit)
		}

	default:
		return nil, fmt.Errorf("unsupported keyword: %s", keyword)
	}

	return stmt, nil
}

func tokenizeCypher(q string) []string {
	var tokens []string
	q = strings.TrimSpace(q)
	i := 0
	for i < len(q) {
		ch := q[i]
		// Skip whitespace.
		if ch == ' ' || ch == '\t' || ch == '\n' || ch == '\r' {
			i++
			continue
		}
		// Single-char tokens.
		if ch == '(' || ch == ')' || ch == '{' || ch == '}' || ch == '[' || ch == ']' ||
			ch == ':' || ch == ',' || ch == '>' || ch == '-' {
			tokens = append(tokens, string(ch))
			i++
			continue
		}
		// Operators: =, !=, <, >, <=, >=
		if ch == '=' {
			tokens = append(tokens, "=")
			i++
			continue
		}
		if ch == '!' && i+1 < len(q) && q[i+1] == '=' {
			tokens = append(tokens, "!=")
			i += 2
			continue
		}
		if ch == '<' {
			if i+1 < len(q) && q[i+1] == '=' {
				tokens = append(tokens, "<=")
				i += 2
			} else {
				tokens = append(tokens, "<")
				i++
			}
			continue
		}
		// Quoted string.
		if ch == '\'' || ch == '"' {
			j := i + 1
			for j < len(q) && q[j] != ch {
				j++
			}
			tokens = append(tokens, q[i:j+1]) // include quotes
			i = j + 1
			continue
		}
		// Word/identifier (includes dots for n.property).
		j := i
		for j < len(q) && q[j] != ' ' && q[j] != '(' && q[j] != ')' && q[j] != '{' &&
			q[j] != '}' && q[j] != '[' && q[j] != ']' && q[j] != ':' && q[j] != ',' &&
			q[j] != '>' && q[j] != '-' && q[j] != '=' && q[j] != '<' && q[j] != '!' &&
			q[j] != '\t' && q[j] != '\n' && q[j] != '\r' {
			j++
		}
		if j > i {
			tokens = append(tokens, q[i:j])
			i = j
		} else {
			i++
		}
	}
	return tokens
}

func parseCypherValue(s string) interface{} {
	// Unquote strings.
	if (strings.HasPrefix(s, "'") && strings.HasSuffix(s, "'")) ||
		(strings.HasPrefix(s, "\"") && strings.HasSuffix(s, "\"")) {
		return s[1 : len(s)-1]
	}
	// Try number.
	var f float64
	if _, err := fmt.Sscanf(s, "%f", &f); err == nil {
		if f == float64(int(f)) {
			return int(f)
		}
		return f
	}
	// Boolean.
	if strings.EqualFold(s, "true") {
		return true
	}
	if strings.EqualFold(s, "false") {
		return false
	}
	if strings.EqualFold(s, "null") {
		return nil
	}
	return s
}

// ── Executor ─────────────────────────────────────────────────────

func (s *Store) executeCypherMatch(stmt *cypherStatement) (*CypherResult, error) {
	// Get candidate nodes.
	var candidates []Node

	if stmt.NodeLabel != "" {
		nodes, err := s.GetNodesByLabel(stmt.NodeLabel)
		if err != nil {
			return nil, err
		}
		candidates = nodes
	} else {
		candidates = s.AllNodes()
	}

	// Filter by inline properties.
	if len(stmt.NodeProps) > 0 {
		var filtered []Node
		for _, n := range candidates {
			if matchesProps(n.Properties, stmt.NodeProps) {
				filtered = append(filtered, n)
			}
		}
		candidates = filtered
	}

	// Filter by WHERE clause.
	if len(stmt.Where) > 0 {
		var filtered []Node
		for _, n := range candidates {
			if matchesCypherWhere(n, stmt.NodeVar, stmt.Where) {
				filtered = append(filtered, n)
			}
		}
		candidates = filtered
	}

	// Handle SET (update matched nodes).
	if len(stmt.SetProps) > 0 {
		updated := 0
		for _, n := range candidates {
			if err := s.UpdateNode(n.ID, stmt.SetProps); err == nil {
				updated++
			}
		}
		return &CypherResult{
			Count:   updated,
			Message: fmt.Sprintf("set properties on %d node(s)", updated),
		}, nil
	}

	// Handle DELETE.
	if stmt.IsDelete {
		deleted := 0
		for _, n := range candidates {
			if err := s.DeleteNode(n.ID); err == nil {
				deleted++
			}
		}
		return &CypherResult{
			Count:   deleted,
			Message: fmt.Sprintf("deleted %d node(s)", deleted),
		}, nil
	}

	// Handle relationship traversal.
	if stmt.EdgeType != "" || stmt.TargetVar != "" {
		var resultNodes []Node
		var resultEdges []Edge
		seen := make(map[uint64]bool)

		for _, n := range candidates {
			edges, err := s.GetEdgesFrom(n.ID)
			if err != nil {
				continue
			}
			for _, edge := range edges {
				if stmt.EdgeType != "" && edge.Type != stmt.EdgeType {
					continue
				}
				targetNode, err := s.GetNode(edge.TargetID)
				if err != nil {
					continue
				}
				if stmt.TargetLabel != "" {
					hasLabel := false
					for _, l := range targetNode.Labels {
						if l == stmt.TargetLabel {
							hasLabel = true
							break
						}
					}
					if !hasLabel {
						continue
					}
				}
				resultEdges = append(resultEdges, edge)
				if !seen[targetNode.ID] {
					seen[targetNode.ID] = true
					resultNodes = append(resultNodes, *targetNode)
				}
			}
		}

		if stmt.IsCount {
			return &CypherResult{Count: len(resultNodes), Message: fmt.Sprintf("%d node(s)", len(resultNodes))}, nil
		}

		if stmt.Limit > 0 && len(resultNodes) > stmt.Limit {
			resultNodes = resultNodes[:stmt.Limit]
		}

		// Project return fields.
		data := projectCypherReturn(resultNodes, stmt.TargetVar, stmt.Returns)

		return &CypherResult{Nodes: resultNodes, Edges: resultEdges, Data: data, Count: len(resultNodes)}, nil
	}

	// Simple MATCH (n) RETURN n.
	if stmt.IsCount {
		return &CypherResult{Count: len(candidates), Message: fmt.Sprintf("%d node(s)", len(candidates))}, nil
	}

	if stmt.Limit > 0 && len(candidates) > stmt.Limit {
		candidates = candidates[:stmt.Limit]
	}

	data := projectCypherReturn(candidates, stmt.NodeVar, stmt.Returns)

	return &CypherResult{Nodes: candidates, Data: data, Count: len(candidates)}, nil
}

func (s *Store) executeCypherCreate(stmt *cypherStatement) (*CypherResult, error) {
	labels := []string{}
	if stmt.NodeLabel != "" {
		labels = append(labels, stmt.NodeLabel)
	}

	id, err := s.CreateNode(labels, stmt.NodeProps, nil)
	if err != nil {
		return nil, err
	}

	node, _ := s.GetNode(id)
	var nodes []Node
	if node != nil {
		nodes = append(nodes, *node)
	}

	return &CypherResult{
		Nodes:   nodes,
		Count:   1,
		Message: fmt.Sprintf("created node #%d", id),
	}, nil
}

// ── Helpers ──────────────────────────────────────────────────────

func matchesProps(nodeProps, filterProps map[string]interface{}) bool {
	for k, v := range filterProps {
		nv, ok := nodeProps[k]
		if !ok {
			return false
		}
		if fmt.Sprintf("%v", nv) != fmt.Sprintf("%v", v) {
			return false
		}
	}
	return true
}

func matchesCypherWhere(n Node, nodeVar string, wheres []cypherWhere) bool {
	for _, w := range wheres {
		if w.Variable != nodeVar {
			continue
		}
		nv, ok := n.Properties[w.Field]
		if !ok {
			return false
		}
		nvStr := fmt.Sprintf("%v", nv)
		wvStr := fmt.Sprintf("%v", w.Value)
		switch w.Op {
		case "=":
			if nvStr != wvStr {
				return false
			}
		case "!=":
			if nvStr == wvStr {
				return false
			}
		case "<":
			if nvStr >= wvStr {
				return false
			}
		case ">":
			if nvStr <= wvStr {
				return false
			}
		case "<=":
			if nvStr > wvStr {
				return false
			}
		case ">=":
			if nvStr < wvStr {
				return false
			}
		}
	}
	return true
}

func projectCypherReturn(nodes []Node, varName string, returns []string) []map[string]interface{} {
	if len(returns) == 0 {
		return nil
	}

	var data []map[string]interface{}
	for _, n := range nodes {
		row := make(map[string]interface{})
		for _, r := range returns {
			if r == varName || r == "*" {
				// Return full node.
				row["id"] = n.ID
				row["labels"] = n.Labels
				for k, v := range n.Properties {
					row[k] = v
				}
			} else if strings.HasPrefix(r, varName+".") {
				field := strings.TrimPrefix(r, varName+".")
				if v, ok := n.Properties[field]; ok {
					row[field] = v
				}
			} else {
				// Treat as property name directly.
				if v, ok := n.Properties[r]; ok {
					row[r] = v
				}
			}
		}
		data = append(data, row)
	}
	return data
}

// GetNodesByLabel retrieves all nodes with a given label.
// This is a convenience method used by the Cypher executor.
func (s *Store) GetNodesByLabel(label string) ([]Node, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var nodes []Node
	err := s.db.View(func(tx *bolt.Tx) error {
		idx := tx.Bucket(bucketNodeIndex)
		data := idx.Get([]byte(label))
		if data == nil {
			return nil
		}
		var ids []uint64
		if err := json.Unmarshal(data, &ids); err != nil {
			return err
		}
		nodesBucket := tx.Bucket(bucketNodes)
		for _, id := range ids {
			raw := nodesBucket.Get(itob(id))
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
