// Package sql provides a SQL string parser and executor for the iTaK Database.
//
// What: A pure-Go SQL parser that translates SQL text into table engine operations.
// Why:  Agents and humans can operate on structured data with familiar SQL syntax
//       instead of building Condition structs by hand.
// How:  A recursive-descent parser tokenizes the SQL string, builds an AST,
//       then the executor maps it to table.Engine method calls.
//
// Supported statements:
//   CREATE TABLE name (col TYPE, col TYPE NOT NULL, ...)
//   CREATE VIEW name AS SELECT ...
//   INSERT INTO name (col, col) VALUES (val, val)
//   INSERT OR UPDATE INTO name (col, col) VALUES (val, val) ON CONFLICT col
//   SELECT [DISTINCT] col, col FROM name WHERE ... GROUP BY col HAVING ... ORDER BY col ASC/DESC LIMIT n
//   SELECT agg(col) FROM name WHERE ...   (COUNT, SUM, AVG, MIN, MAX)
//   SELECT ... FROM a [INNER|LEFT] JOIN b ON a.col = b.col WHERE ...
//   UPDATE name SET col = val, col = val WHERE ...
//   DELETE FROM name WHERE ...
//   ALTER TABLE name ADD COLUMN col TYPE / DROP COLUMN col
//   DROP TABLE name
//   DROP VIEW name
//   SHOW TABLES
//   DESCRIBE name
//   BEGIN / COMMIT / ROLLBACK
//
// WHERE supports: AND, OR, =, !=, <, >, <=, >=, LIKE, IN, IS NULL, IS NOT NULL, BETWEEN
// Subqueries: WHERE col IN (SELECT col FROM ...)
package sql

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"
)

// ── Token types ──────────────────────────────────────────────────

type tokenKind int

const (
	tkEOF tokenKind = iota
	tkIdent
	tkNumber
	tkString
	tkComma
	tkLParen
	tkRParen
	tkStar
	tkSemicolon
	tkDot
	tkOp // =, !=, <, >, <=, >=
	tkQuestion // ? for prepared statements
)

type token struct {
	Kind  tokenKind
	Value string
}

// ── Tokenizer ────────────────────────────────────────────────────

func tokenize(input string) []token {
	var tokens []token
	runes := []rune(input)
	i := 0

	for i < len(runes) {
		ch := runes[i]

		if unicode.IsSpace(ch) {
			i++
			continue
		}

		switch ch {
		case ',':
			tokens = append(tokens, token{tkComma, ","})
			i++
			continue
		case '(':
			tokens = append(tokens, token{tkLParen, "("})
			i++
			continue
		case ')':
			tokens = append(tokens, token{tkRParen, ")"})
			i++
			continue
		case '*':
			tokens = append(tokens, token{tkStar, "*"})
			i++
			continue
		case ';':
			tokens = append(tokens, token{tkSemicolon, ";"})
			i++
			continue
		case '.':
			tokens = append(tokens, token{tkDot, "."})
			i++
			continue
		case '?':
			tokens = append(tokens, token{tkQuestion, "?"})
			i++
			continue
		}

		if ch == '!' && i+1 < len(runes) && runes[i+1] == '=' {
			tokens = append(tokens, token{tkOp, "!="})
			i += 2
			continue
		}
		if ch == '<' && i+1 < len(runes) && runes[i+1] == '=' {
			tokens = append(tokens, token{tkOp, "<="})
			i += 2
			continue
		}
		if ch == '>' && i+1 < len(runes) && runes[i+1] == '=' {
			tokens = append(tokens, token{tkOp, ">="})
			i += 2
			continue
		}
		if ch == '<' || ch == '>' || ch == '=' {
			tokens = append(tokens, token{tkOp, string(ch)})
			i++
			continue
		}

		if ch == '\'' || ch == '"' {
			quote := ch
			i++
			start := i
			for i < len(runes) && runes[i] != quote {
				if runes[i] == '\\' && i+1 < len(runes) {
					i++
				}
				i++
			}
			tokens = append(tokens, token{tkString, string(runes[start:i])})
			if i < len(runes) {
				i++
			}
			continue
		}

		if unicode.IsDigit(ch) || (ch == '-' && i+1 < len(runes) && unicode.IsDigit(runes[i+1])) {
			start := i
			if ch == '-' {
				i++
			}
			for i < len(runes) && (unicode.IsDigit(runes[i]) || runes[i] == '.') {
				i++
			}
			tokens = append(tokens, token{tkNumber, string(runes[start:i])})
			continue
		}

		if unicode.IsLetter(ch) || ch == '_' {
			start := i
			for i < len(runes) && (unicode.IsLetter(runes[i]) || unicode.IsDigit(runes[i]) || runes[i] == '_') {
				i++
			}
			tokens = append(tokens, token{tkIdent, string(runes[start:i])})
			continue
		}

		i++
	}

	tokens = append(tokens, token{tkEOF, ""})
	return tokens
}

// ── AST Nodes ────────────────────────────────────────────────────

type StatementType int

const (
	StmtSelect StatementType = iota
	StmtInsert
	StmtUpdate
	StmtDelete
	StmtCreateTable
	StmtDropTable
	StmtShowTables
	StmtDescribe
	StmtAlterTable
	StmtCreateView
	StmtDropView
	StmtBegin
	StmtCommit
	StmtRollback
	StmtUpsert
)

type AggFunc struct {
	Name   string
	Column string
}

type JoinClause struct {
	Table    string
	ColA     string
	ColB     string
	JoinType string // "INNER" or "LEFT"
}

type WhereNode struct {
	Column   string
	Op       string
	Value    interface{}
	Logic    string // "AND" or "OR"
	Subquery *Statement // For IN (SELECT ...)
}

// OrderByItem holds a single ORDER BY column and direction.
type OrderByItem struct {
	Column string
	Dir    string // "ASC" or "DESC"
}

type Statement struct {
	Type        StatementType
	Table       string
	Columns     []string
	Values      []interface{}
	Set         map[string]interface{}
	Where       []WhereClause
	WhereTree   []WhereNode
	OrderBy     string // Single column (backward compat)
	OrderDir    string
	OrderByList []OrderByItem // Multi-column ORDER BY
	Limit       int
	Offset      int
	ColDefs     []ColumnDef
	CountMode   bool
	Agg         *AggFunc
	Join        *JoinClause
	AddCols     []ColumnDef
	DropCols    []string
	Distinct    bool     // SELECT DISTINCT
	GroupBy     []string // GROUP BY columns
	Having      []WhereClause // HAVING conditions
	ViewName    string   // CREATE VIEW name
	ViewQuery   string   // CREATE VIEW AS query
	ConflictCol string   // UPSERT ON CONFLICT column
	Params      []int    // Prepared statement ? parameter positions
}

type WhereClause struct {
	Column string
	Op     string
	Value  interface{}
}

type ColumnDef struct {
	Name     string
	Type     string
	Nullable bool
}

// ── Parser ───────────────────────────────────────────────────────

type parser struct {
	tokens []token
	pos    int
}

func (p *parser) peek() token {
	if p.pos < len(p.tokens) {
		return p.tokens[p.pos]
	}
	return token{tkEOF, ""}
}

func (p *parser) advance() token {
	t := p.peek()
	p.pos++
	return t
}

func (p *parser) expect(kind tokenKind) (token, error) {
	t := p.advance()
	if t.Kind != kind {
		return t, fmt.Errorf("expected token type %d, got %q", kind, t.Value)
	}
	return t, nil
}

func (p *parser) expectKeyword(kw string) error {
	t := p.advance()
	if t.Kind != tkIdent || !strings.EqualFold(t.Value, kw) {
		return fmt.Errorf("expected keyword %s, got %q", kw, t.Value)
	}
	return nil
}

func (p *parser) isKeyword(kw string) bool {
	t := p.peek()
	return t.Kind == tkIdent && strings.EqualFold(t.Value, kw)
}

// Parse takes a SQL string and returns a parsed Statement.
func Parse(sql string) (*Statement, error) {
	tokens := tokenize(sql)
	p := &parser{tokens: tokens, pos: 0}

	first := p.peek()
	if first.Kind != tkIdent {
		return nil, fmt.Errorf("expected SQL keyword, got %q", first.Value)
	}

	switch strings.ToUpper(first.Value) {
	case "SELECT":
		return p.parseSelect()
	case "INSERT":
		return p.parseInsert()
	case "UPDATE":
		return p.parseUpdate()
	case "DELETE":
		return p.parseDelete()
	case "CREATE":
		return p.parseCreate()
	case "DROP":
		return p.parseDrop()
	case "SHOW":
		return p.parseShowTables()
	case "DESCRIBE", "DESC":
		return p.parseDescribe()
	case "ALTER":
		return p.parseAlterTable()
	case "BEGIN":
		p.advance()
		return &Statement{Type: StmtBegin}, nil
	case "COMMIT":
		p.advance()
		return &Statement{Type: StmtCommit}, nil
	case "ROLLBACK":
		p.advance()
		return &Statement{Type: StmtRollback}, nil
	default:
		return nil, fmt.Errorf("unsupported statement: %s", first.Value)
	}
}

// PrepareSQL parses SQL with ? placeholders and returns statement + param positions.
func PrepareSQL(sql string) (*Statement, error) {
	stmt, err := Parse(sql)
	if err != nil {
		return nil, err
	}
	// Count ? tokens in the original SQL for parameter binding.
	count := 0
	for _, r := range sql {
		if r == '?' {
			count++
		}
	}
	if count > 0 {
		stmt.Params = make([]int, count)
		for i := range stmt.Params {
			stmt.Params[i] = i
		}
	}
	return stmt, nil
}

// ── CREATE (TABLE or VIEW) ───────────────────────────────────────

func (p *parser) parseCreate() (*Statement, error) {
	p.advance() // consume CREATE

	if p.isKeyword("VIEW") {
		return p.parseCreateView()
	}
	return p.parseCreateTable()
}

// ── DROP (TABLE or VIEW) ─────────────────────────────────────────

func (p *parser) parseDrop() (*Statement, error) {
	p.advance() // consume DROP

	if p.isKeyword("VIEW") {
		p.advance()
		name, err := p.expect(tkIdent)
		if err != nil {
			return nil, err
		}
		return &Statement{Type: StmtDropView, ViewName: name.Value}, nil
	}

	if err := p.expectKeyword("TABLE"); err != nil {
		return nil, err
	}
	tbl, err := p.expect(tkIdent)
	if err != nil {
		return nil, err
	}
	return &Statement{Type: StmtDropTable, Table: tbl.Value}, nil
}

// ── SELECT ───────────────────────────────────────────────────────

func (p *parser) parseSelect() (*Statement, error) {
	p.advance() // consume SELECT

	stmt := &Statement{Type: StmtSelect}

	// DISTINCT
	if p.isKeyword("DISTINCT") {
		p.advance()
		stmt.Distinct = true
	}

	// Aggregate functions or columns.
	if p.isAggFunc() {
		agg, err := p.parseAggFunc()
		if err != nil {
			return nil, err
		}
		if agg.Name == "COUNT" {
			stmt.CountMode = true
		}
		stmt.Agg = agg
	} else if p.peek().Kind == tkStar {
		p.advance()
		stmt.Columns = nil
	} else {
		for {
			col, err := p.expect(tkIdent)
			if err != nil {
				return nil, fmt.Errorf("expected column name: %w", err)
			}
			colName := col.Value
			if p.peek().Kind == tkDot {
				p.advance()
				next, err := p.expect(tkIdent)
				if err != nil {
					return nil, err
				}
				colName = colName + "." + next.Value
			}
			stmt.Columns = append(stmt.Columns, colName)
			if p.peek().Kind != tkComma {
				break
			}
			p.advance()
		}
	}

	// FROM
	if err := p.expectKeyword("FROM"); err != nil {
		return nil, err
	}
	tbl, err := p.expect(tkIdent)
	if err != nil {
		return nil, fmt.Errorf("expected table name: %w", err)
	}
	stmt.Table = tbl.Value

	// Optional JOIN (INNER or LEFT)
	if p.isKeyword("INNER") || p.isKeyword("LEFT") || p.isKeyword("JOIN") {
		joinType := "INNER"
		if p.isKeyword("LEFT") {
			joinType = "LEFT"
			p.advance()
		} else if p.isKeyword("INNER") {
			p.advance()
		}
		if err := p.expectKeyword("JOIN"); err != nil {
			return nil, err
		}
		joinTable, err := p.expect(tkIdent)
		if err != nil {
			return nil, err
		}
		if err := p.expectKeyword("ON"); err != nil {
			return nil, err
		}

		colATable, err := p.expect(tkIdent)
		if err != nil {
			return nil, err
		}
		if _, err := p.expect(tkDot); err != nil {
			return nil, err
		}
		colAName, err := p.expect(tkIdent)
		if err != nil {
			return nil, err
		}
		if _, err := p.expect(tkOp); err != nil {
			return nil, err
		}
		_, err = p.expect(tkIdent)
		if err != nil {
			return nil, err
		}
		if _, err := p.expect(tkDot); err != nil {
			return nil, err
		}
		colBName, err := p.expect(tkIdent)
		if err != nil {
			return nil, err
		}

		colA := colAName.Value
		colB := colBName.Value
		if !strings.EqualFold(colATable.Value, stmt.Table) {
			colA, colB = colB, colA
		}

		stmt.Join = &JoinClause{
			Table:    joinTable.Value,
			ColA:     colA,
			ColB:     colB,
			JoinType: joinType,
		}
	}

	// WHERE
	if p.isKeyword("WHERE") {
		p.advance()
		tree, simple, err := p.parseWhereTree()
		if err != nil {
			return nil, err
		}
		stmt.WhereTree = tree
		stmt.Where = simple
	}

	// GROUP BY
	if p.isKeyword("GROUP") {
		p.advance()
		if err := p.expectKeyword("BY"); err != nil {
			return nil, err
		}
		for {
			col, err := p.expect(tkIdent)
			if err != nil {
				return nil, err
			}
			stmt.GroupBy = append(stmt.GroupBy, col.Value)
			if p.peek().Kind != tkComma {
				break
			}
			p.advance()
		}
	}

	// HAVING
	if p.isKeyword("HAVING") {
		p.advance()
		for {
			clause, err := p.parseHavingCondition()
			if err != nil {
				return nil, err
			}
			stmt.Having = append(stmt.Having, clause)
			if !p.isKeyword("AND") {
				break
			}
			p.advance()
		}
	}

	// ORDER BY (supports multiple columns: ORDER BY col1 ASC, col2 DESC)
	if p.isKeyword("ORDER") {
		p.advance()
		if err := p.expectKeyword("BY"); err != nil {
			return nil, err
		}
		for {
			col, err := p.expect(tkIdent)
			if err != nil {
				return nil, err
			}
			dir := "ASC"
			if p.isKeyword("DESC") {
				p.advance()
				dir = "DESC"
			} else if p.isKeyword("ASC") {
				p.advance()
			}
			stmt.OrderByList = append(stmt.OrderByList, OrderByItem{Column: col.Value, Dir: dir})
			if p.peek().Kind != tkComma {
				break
			}
			p.advance()
		}
		// Backward compatibility: set single OrderBy.
		if len(stmt.OrderByList) > 0 {
			stmt.OrderBy = stmt.OrderByList[0].Column
			stmt.OrderDir = stmt.OrderByList[0].Dir
		}
	}

	// LIMIT
	if p.isKeyword("LIMIT") {
		p.advance()
		num, err := p.expect(tkNumber)
		if err != nil {
			return nil, err
		}
		stmt.Limit, _ = strconv.Atoi(num.Value)
	}

	// OFFSET
	if p.isKeyword("OFFSET") {
		p.advance()
		num, err := p.expect(tkNumber)
		if err != nil {
			return nil, err
		}
		stmt.Offset, _ = strconv.Atoi(num.Value)
	}

	return stmt, nil
}

// ── HAVING condition parsing ─────────────────────────────────────

func (p *parser) parseHavingCondition() (WhereClause, error) {
	// HAVING supports: agg(col) op val  OR  col op val
	if p.isAggFunc() {
		agg, err := p.parseAggFunc()
		if err != nil {
			return WhereClause{}, err
		}
		op, err := p.expect(tkOp)
		if err != nil {
			return WhereClause{}, err
		}
		val, err := p.parseValue()
		if err != nil {
			return WhereClause{}, err
		}
		return WhereClause{
			Column: agg.Name + "(" + agg.Column + ")",
			Op:     op.Value,
			Value:  val,
		}, nil
	}

	col, err := p.expect(tkIdent)
	if err != nil {
		return WhereClause{}, err
	}
	op, err := p.expect(tkOp)
	if err != nil {
		return WhereClause{}, err
	}
	val, err := p.parseValue()
	if err != nil {
		return WhereClause{}, err
	}
	return WhereClause{Column: col.Value, Op: op.Value, Value: val}, nil
}

// ── Aggregate function detection and parsing ─────────────────────

func (p *parser) isAggFunc() bool {
	t := p.peek()
	if t.Kind != tkIdent {
		return false
	}
	upper := strings.ToUpper(t.Value)
	return upper == "COUNT" || upper == "SUM" || upper == "AVG" || upper == "MIN" || upper == "MAX"
}

func (p *parser) parseAggFunc() (*AggFunc, error) {
	funcName := strings.ToUpper(p.advance().Value)

	if _, err := p.expect(tkLParen); err != nil {
		return nil, err
	}

	var col string
	if p.peek().Kind == tkStar {
		p.advance()
		col = "*"
	} else {
		t, err := p.expect(tkIdent)
		if err != nil {
			return nil, err
		}
		col = t.Value
	}

	if _, err := p.expect(tkRParen); err != nil {
		return nil, err
	}

	return &AggFunc{Name: funcName, Column: col}, nil
}

// ── INSERT / UPSERT ──────────────────────────────────────────────

func (p *parser) parseInsert() (*Statement, error) {
	p.advance() // consume INSERT

	stmt := &Statement{Type: StmtInsert}

	// Check for INSERT OR UPDATE
	if p.isKeyword("OR") {
		p.advance()
		if err := p.expectKeyword("UPDATE"); err != nil {
			return nil, err
		}
		stmt.Type = StmtUpsert
	}

	if err := p.expectKeyword("INTO"); err != nil {
		return nil, err
	}

	tbl, err := p.expect(tkIdent)
	if err != nil {
		return nil, err
	}
	stmt.Table = tbl.Value

	// Column list.
	if _, err := p.expect(tkLParen); err != nil {
		return nil, err
	}
	for {
		col, err := p.expect(tkIdent)
		if err != nil {
			return nil, err
		}
		stmt.Columns = append(stmt.Columns, col.Value)
		if p.peek().Kind != tkComma {
			break
		}
		p.advance()
	}
	if _, err := p.expect(tkRParen); err != nil {
		return nil, err
	}

	// VALUES
	if err := p.expectKeyword("VALUES"); err != nil {
		return nil, err
	}
	if _, err := p.expect(tkLParen); err != nil {
		return nil, err
	}
	for {
		val, err := p.parseValue()
		if err != nil {
			return nil, err
		}
		stmt.Values = append(stmt.Values, val)
		if p.peek().Kind != tkComma {
			break
		}
		p.advance()
	}
	if _, err := p.expect(tkRParen); err != nil {
		return nil, err
	}

	if len(stmt.Columns) != len(stmt.Values) {
		return nil, fmt.Errorf("column count (%d) does not match value count (%d)", len(stmt.Columns), len(stmt.Values))
	}

	// ON CONFLICT col (for UPSERT)
	if p.isKeyword("ON") {
		p.advance()
		if err := p.expectKeyword("CONFLICT"); err != nil {
			return nil, err
		}
		col, err := p.expect(tkIdent)
		if err != nil {
			return nil, err
		}
		stmt.ConflictCol = col.Value
		stmt.Type = StmtUpsert
	}

	return stmt, nil
}

// ── UPDATE ───────────────────────────────────────────────────────

func (p *parser) parseUpdate() (*Statement, error) {
	p.advance()

	tbl, err := p.expect(tkIdent)
	if err != nil {
		return nil, err
	}

	stmt := &Statement{Type: StmtUpdate, Table: tbl.Value, Set: make(map[string]interface{})}

	if err := p.expectKeyword("SET"); err != nil {
		return nil, err
	}

	for {
		col, err := p.expect(tkIdent)
		if err != nil {
			return nil, err
		}
		if _, err := p.expect(tkOp); err != nil {
			return nil, err
		}
		val, err := p.parseValue()
		if err != nil {
			return nil, err
		}
		stmt.Set[col.Value] = val
		if p.peek().Kind != tkComma {
			break
		}
		p.advance()
	}

	if p.isKeyword("WHERE") {
		p.advance()
		tree, simple, err := p.parseWhereTree()
		if err != nil {
			return nil, err
		}
		stmt.WhereTree = tree
		stmt.Where = simple
	}

	return stmt, nil
}

// ── DELETE ───────────────────────────────────────────────────────

func (p *parser) parseDelete() (*Statement, error) {
	p.advance()

	if err := p.expectKeyword("FROM"); err != nil {
		return nil, err
	}

	tbl, err := p.expect(tkIdent)
	if err != nil {
		return nil, err
	}
	stmt := &Statement{Type: StmtDelete, Table: tbl.Value}

	if p.isKeyword("WHERE") {
		p.advance()
		tree, simple, err := p.parseWhereTree()
		if err != nil {
			return nil, err
		}
		stmt.WhereTree = tree
		stmt.Where = simple
	}

	return stmt, nil
}

// ── CREATE TABLE ─────────────────────────────────────────────────

func (p *parser) parseCreateTable() (*Statement, error) {
	if err := p.expectKeyword("TABLE"); err != nil {
		return nil, err
	}

	tbl, err := p.expect(tkIdent)
	if err != nil {
		return nil, err
	}

	stmt := &Statement{Type: StmtCreateTable, Table: tbl.Value}

	if _, err := p.expect(tkLParen); err != nil {
		return nil, err
	}

	for {
		colName, err := p.expect(tkIdent)
		if err != nil {
			return nil, err
		}
		colType, err := p.expect(tkIdent)
		if err != nil {
			return nil, err
		}

		def := ColumnDef{
			Name:     colName.Value,
			Type:     strings.ToUpper(colType.Value),
			Nullable: true,
		}

		if p.isKeyword("NOT") {
			p.advance()
			if err := p.expectKeyword("NULL"); err != nil {
				return nil, err
			}
			def.Nullable = false
		}

		stmt.ColDefs = append(stmt.ColDefs, def)

		if p.peek().Kind != tkComma {
			break
		}
		p.advance()
	}

	if _, err := p.expect(tkRParen); err != nil {
		return nil, err
	}

	return stmt, nil
}

// ── CREATE VIEW ──────────────────────────────────────────────────

func (p *parser) parseCreateView() (*Statement, error) {
	p.advance() // consume VIEW

	name, err := p.expect(tkIdent)
	if err != nil {
		return nil, err
	}

	if err := p.expectKeyword("AS"); err != nil {
		return nil, err
	}

	// Capture the rest of the tokens as the view query.
	startPos := p.pos
	// Find the raw SQL by reconstructing from tokens.
	var queryParts []string
	for p.peek().Kind != tkEOF && p.peek().Kind != tkSemicolon {
		t := p.advance()
		queryParts = append(queryParts, t.Value)
	}
	_ = startPos

	return &Statement{
		Type:      StmtCreateView,
		ViewName:  name.Value,
		ViewQuery: strings.Join(queryParts, " "),
	}, nil
}

// ── ALTER TABLE ──────────────────────────────────────────────────

func (p *parser) parseAlterTable() (*Statement, error) {
	p.advance() // consume ALTER

	if err := p.expectKeyword("TABLE"); err != nil {
		return nil, err
	}

	tbl, err := p.expect(tkIdent)
	if err != nil {
		return nil, err
	}

	stmt := &Statement{Type: StmtAlterTable, Table: tbl.Value}

	for {
		if p.isKeyword("ADD") {
			p.advance()
			if p.isKeyword("COLUMN") {
				p.advance()
			}
			colName, err := p.expect(tkIdent)
			if err != nil {
				return nil, err
			}
			colType, err := p.expect(tkIdent)
			if err != nil {
				return nil, err
			}
			def := ColumnDef{Name: colName.Value, Type: strings.ToUpper(colType.Value), Nullable: true}
			if p.isKeyword("NOT") {
				p.advance()
				if err := p.expectKeyword("NULL"); err != nil {
					return nil, err
				}
				def.Nullable = false
			}
			stmt.AddCols = append(stmt.AddCols, def)
		} else if p.isKeyword("DROP") {
			p.advance()
			if p.isKeyword("COLUMN") {
				p.advance()
			}
			colName, err := p.expect(tkIdent)
			if err != nil {
				return nil, err
			}
			stmt.DropCols = append(stmt.DropCols, colName.Value)
		} else {
			break
		}

		if p.peek().Kind == tkComma {
			p.advance()
			continue
		}
		break
	}

	if len(stmt.AddCols) == 0 && len(stmt.DropCols) == 0 {
		return nil, fmt.Errorf("ALTER TABLE requires ADD or DROP clause")
	}

	return stmt, nil
}

// ── SHOW TABLES / DESCRIBE ───────────────────────────────────────

func (p *parser) parseShowTables() (*Statement, error) {
	p.advance()
	if err := p.expectKeyword("TABLES"); err != nil {
		return nil, err
	}
	return &Statement{Type: StmtShowTables}, nil
}

func (p *parser) parseDescribe() (*Statement, error) {
	p.advance()
	tbl, err := p.expect(tkIdent)
	if err != nil {
		return nil, err
	}
	return &Statement{Type: StmtDescribe, Table: tbl.Value}, nil
}

// ── WHERE parsing with AND/OR/BETWEEN/subquery ───────────────────

func (p *parser) parseWhereTree() ([]WhereNode, []WhereClause, error) {
	var nodes []WhereNode
	var simple []WhereClause

	for {
		col, err := p.expect(tkIdent)
		if err != nil {
			return nil, nil, fmt.Errorf("expected column in WHERE: %w", err)
		}

		colName := col.Value
		if p.peek().Kind == tkDot {
			p.advance()
			next, err := p.expect(tkIdent)
			if err != nil {
				return nil, nil, err
			}
			colName = colName + "." + next.Value
		}

		var node WhereNode
		node.Column = colName

		if p.isKeyword("IS") {
			p.advance()
			if p.isKeyword("NOT") {
				p.advance()
				if err := p.expectKeyword("NULL"); err != nil {
					return nil, nil, err
				}
				node.Op = "!="
				node.Value = nil
			} else if p.isKeyword("NULL") {
				p.advance()
				node.Op = "="
				node.Value = nil
			}
		} else if p.isKeyword("NOT") {
			p.advance()
			if p.isKeyword("LIKE") {
				p.advance()
				val, err := p.parseValue()
				if err != nil {
					return nil, nil, err
				}
				node.Op = "NOT LIKE"
				node.Value = val
			} else if p.isKeyword("BETWEEN") {
				p.advance()
				low, err := p.parseValue()
				if err != nil {
					return nil, nil, err
				}
				if err := p.expectKeyword("AND"); err != nil {
					return nil, nil, fmt.Errorf("expected AND in NOT BETWEEN: %w", err)
				}
				high, err := p.parseValue()
				if err != nil {
					return nil, nil, err
				}
				node.Op = "NOT BETWEEN"
				node.Value = []interface{}{low, high}
			} else if p.isKeyword("IN") {
				p.advance()
				if _, err := p.expect(tkLParen); err != nil {
					return nil, nil, err
				}
				var vals []interface{}
				for {
					val, err := p.parseValue()
					if err != nil {
						return nil, nil, err
					}
					vals = append(vals, val)
					if p.peek().Kind != tkComma {
						break
					}
					p.advance()
				}
				if _, err := p.expect(tkRParen); err != nil {
					return nil, nil, err
				}
				node.Op = "NOT IN"
				node.Value = vals
			} else {
				return nil, nil, fmt.Errorf("expected LIKE, BETWEEN, or IN after NOT")
			}
		} else if p.isKeyword("LIKE") {
			p.advance()
			val, err := p.parseValue()
			if err != nil {
				return nil, nil, err
			}
			node.Op = "LIKE"
			node.Value = val
		} else if p.isKeyword("BETWEEN") {
			p.advance()
			low, err := p.parseValue()
			if err != nil {
				return nil, nil, err
			}
			if err := p.expectKeyword("AND"); err != nil {
				return nil, nil, fmt.Errorf("expected AND in BETWEEN: %w", err)
			}
			high, err := p.parseValue()
			if err != nil {
				return nil, nil, err
			}
			node.Op = "BETWEEN"
			node.Value = []interface{}{low, high}
		} else if p.isKeyword("IN") {
			p.advance()
			if _, err := p.expect(tkLParen); err != nil {
				return nil, nil, err
			}

			// Check for subquery: IN (SELECT ...)
			if p.isKeyword("SELECT") {
				subStmt, err := p.parseSelect()
				if err != nil {
					return nil, nil, fmt.Errorf("subquery error: %w", err)
				}
				if _, err := p.expect(tkRParen); err != nil {
					return nil, nil, err
				}
				node.Op = "IN"
				node.Subquery = subStmt
				node.Value = nil // Will be resolved at execution.
			} else {
				var vals []interface{}
				for {
					val, err := p.parseValue()
					if err != nil {
						return nil, nil, err
					}
					vals = append(vals, val)
					if p.peek().Kind != tkComma {
						break
					}
					p.advance()
				}
				if _, err := p.expect(tkRParen); err != nil {
					return nil, nil, err
				}
				node.Op = "IN"
				node.Value = vals
			}
		} else {
			op, err := p.expect(tkOp)
			if err != nil {
				return nil, nil, fmt.Errorf("expected operator in WHERE: %w", err)
			}
			val, err := p.parseValue()
			if err != nil {
				return nil, nil, err
			}
			node.Op = op.Value
			node.Value = val
		}

		simple = append(simple, WhereClause{Column: node.Column, Op: node.Op, Value: node.Value})

		if p.isKeyword("AND") {
			node.Logic = "AND"
			p.advance()
			nodes = append(nodes, node)
			continue
		}
		if p.isKeyword("OR") {
			node.Logic = "OR"
			p.advance()
			nodes = append(nodes, node)
			continue
		}

		node.Logic = ""
		nodes = append(nodes, node)
		break
	}

	return nodes, simple, nil
}

// ── Value parsing ────────────────────────────────────────────────

func (p *parser) parseValue() (interface{}, error) {
	t := p.peek()

	switch t.Kind {
	case tkString:
		p.advance()
		return t.Value, nil
	case tkNumber:
		p.advance()
		if strings.Contains(t.Value, ".") {
			f, err := strconv.ParseFloat(t.Value, 64)
			if err != nil {
				return nil, err
			}
			return f, nil
		}
		n, err := strconv.ParseInt(t.Value, 10, 64)
		if err != nil {
			return nil, err
		}
		return n, nil
	case tkIdent:
		upper := strings.ToUpper(t.Value)
		if upper == "NULL" {
			p.advance()
			return nil, nil
		}
		if upper == "TRUE" {
			p.advance()
			return true, nil
		}
		if upper == "FALSE" {
			p.advance()
			return false, nil
		}
		p.advance()
		return t.Value, nil
	case tkQuestion:
		p.advance()
		return "?", nil // Placeholder for prepared statements.
	default:
		return nil, fmt.Errorf("unexpected token for value: %q (type %d)", t.Value, t.Kind)
	}
}
