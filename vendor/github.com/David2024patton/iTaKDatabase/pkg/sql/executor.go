package sql

import (
	"fmt"
	"sort"
	"strings"

	"github.com/David2024patton/iTaKDatabase/pkg/table"
)

// Result holds the output of a SQL execution.
type Result struct {
	Rows     []table.Row    `json:"rows,omitempty"`
	Columns  []string       `json:"columns,omitempty"`
	InsertID uint64         `json:"insert_id,omitempty"`
	Affected int            `json:"affected,omitempty"`
	Count    int            `json:"count,omitempty"`
	AggValue float64        `json:"agg_value,omitempty"`
	AggFunc  string         `json:"agg_func,omitempty"`
	Tables   []string       `json:"tables,omitempty"`
	Schema   []table.Column `json:"schema,omitempty"`
	Message  string         `json:"message,omitempty"`
	Updated  bool           `json:"updated,omitempty"` // For UPSERT: true = update, false = insert.
	Groups   []GroupRow     `json:"groups,omitempty"`  // For GROUP BY results.
}

// GroupRow holds grouped aggregation results.
type GroupRow struct {
	Key    map[string]interface{} `json:"key"`
	Count  int                    `json:"count"`
	Sum    float64                `json:"sum,omitempty"`
	Avg    float64                `json:"avg,omitempty"`
	Min    float64                `json:"min,omitempty"`
	Max    float64                `json:"max,omitempty"`
}

// TransactionBuffer stores statements for BEGIN/COMMIT/ROLLBACK.
type TransactionBuffer struct {
	Statements []*Statement
	Active     bool
}

// Executor runs parsed SQL statements against the table engine.
type Executor struct {
	engine *table.Engine
	txBuf  *TransactionBuffer
}

// NewExecutor creates an executor bound to a table engine.
func NewExecutor(engine *table.Engine) *Executor {
	return &Executor{engine: engine, txBuf: &TransactionBuffer{}}
}

// Execute parses and runs a SQL string in one call.
func (ex *Executor) Execute(sql string) (*Result, error) {
	stmt, err := Parse(sql)
	if err != nil {
		return nil, fmt.Errorf("parse error: %w", err)
	}
	return ex.Run(stmt)
}

// ExecutePrepared runs a prepared statement with bound parameters.
func (ex *Executor) ExecutePrepared(sql string, params ...interface{}) (*Result, error) {
	stmt, err := PrepareSQL(sql)
	if err != nil {
		return nil, err
	}

	// Bind ? parameters in WHERE clauses.
	paramIdx := 0
	for i := range stmt.Where {
		if fmt.Sprintf("%v", stmt.Where[i].Value) == "?" && paramIdx < len(params) {
			stmt.Where[i].Value = params[paramIdx]
			paramIdx++
		}
	}
	for i := range stmt.WhereTree {
		if fmt.Sprintf("%v", stmt.WhereTree[i].Value) == "?" && paramIdx < len(params) {
			stmt.WhereTree[i].Value = params[paramIdx]
			paramIdx++
		}
	}
	// Bind ? in INSERT values.
	for i := range stmt.Values {
		if fmt.Sprintf("%v", stmt.Values[i]) == "?" && paramIdx < len(params) {
			stmt.Values[i] = params[paramIdx]
			paramIdx++
		}
	}

	return ex.Run(stmt)
}

// Run executes a parsed Statement.
func (ex *Executor) Run(stmt *Statement) (*Result, error) {
	// Transaction management.
	switch stmt.Type {
	case StmtBegin:
		ex.txBuf.Active = true
		ex.txBuf.Statements = nil
		return &Result{Message: "transaction started"}, nil
	case StmtRollback:
		ex.txBuf.Active = false
		ex.txBuf.Statements = nil
		return &Result{Message: "transaction rolled back"}, nil
	case StmtCommit:
		return ex.commitTransaction()
	}

	// If inside a transaction, buffer the statement.
	if ex.txBuf.Active {
		ex.txBuf.Statements = append(ex.txBuf.Statements, stmt)
		return &Result{Message: fmt.Sprintf("statement buffered (%d in transaction)", len(ex.txBuf.Statements))}, nil
	}

	return ex.executeStatement(stmt)
}

func (ex *Executor) commitTransaction() (*Result, error) {
	if !ex.txBuf.Active {
		return nil, fmt.Errorf("no active transaction")
	}
	ex.txBuf.Active = false

	total := 0
	for _, stmt := range ex.txBuf.Statements {
		_, err := ex.executeStatement(stmt)
		if err != nil {
			// Rollback: we can't undo bbolt ops, but we stop processing.
			ex.txBuf.Statements = nil
			return nil, fmt.Errorf("transaction failed at statement %d: %w", total+1, err)
		}
		total++
	}
	ex.txBuf.Statements = nil
	return &Result{Affected: total, Message: fmt.Sprintf("committed %d statement(s)", total)}, nil
}

func (ex *Executor) executeStatement(stmt *Statement) (*Result, error) {
	switch stmt.Type {
	case StmtSelect:
		return ex.execSelect(stmt)
	case StmtInsert:
		return ex.execInsert(stmt)
	case StmtUpsert:
		return ex.execUpsert(stmt)
	case StmtUpdate:
		return ex.execUpdate(stmt)
	case StmtDelete:
		return ex.execDelete(stmt)
	case StmtCreateTable:
		return ex.execCreateTable(stmt)
	case StmtDropTable:
		return ex.execDropTable(stmt)
	case StmtAlterTable:
		return ex.execAlterTable(stmt)
	case StmtCreateView:
		return ex.execCreateView(stmt)
	case StmtDropView:
		return ex.execDropView(stmt)
	case StmtShowTables:
		return ex.execShowTables()
	case StmtDescribe:
		return ex.execDescribe(stmt)
	default:
		return nil, fmt.Errorf("unsupported statement type: %d", stmt.Type)
	}
}

// ── SELECT ───────────────────────────────────────────────────────

func (ex *Executor) execSelect(stmt *Statement) (*Result, error) {
	// Aggregation mode (SUM, AVG, MIN, MAX) without GROUP BY.
	if stmt.Agg != nil && !stmt.CountMode && len(stmt.GroupBy) == 0 {
		return ex.execAggregate(stmt)
	}

	// JOIN mode.
	if stmt.Join != nil {
		return ex.execJoin(stmt)
	}

	// Resolve subqueries in WHERE.
	if err := ex.resolveSubqueries(stmt); err != nil {
		return nil, err
	}

	conditions := convertConditions(stmt.Where)

	useOR := hasOR(stmt.WhereTree)
	fetchConditions := conditions
	if useOR {
		fetchConditions = nil
	}

	fetchLimit := 0
	if stmt.Limit > 0 && stmt.OrderBy == "" && !useOR && len(stmt.GroupBy) == 0 && !stmt.Distinct {
		fetchLimit = stmt.Limit
	}

	rows, err := ex.engine.Select(stmt.Table, fetchConditions, fetchLimit)
	if err != nil {
		return nil, err
	}

	if useOR {
		rows = filterWithOR(rows, stmt.WhereTree)
	}

	// COUNT(*) mode.
	if stmt.CountMode && len(stmt.GroupBy) == 0 {
		if len(conditions) > 0 || useOR {
			return &Result{Count: len(rows)}, nil
		}
		count, err := ex.engine.Count(stmt.Table)
		if err != nil {
			return nil, err
		}
		return &Result{Count: count}, nil
	}

	// GROUP BY.
	if len(stmt.GroupBy) > 0 {
		return ex.execGroupBy(rows, stmt)
	}

	// ORDER BY (supports multi-column).
	if len(stmt.OrderByList) > 0 {
		sort.SliceStable(rows, func(i, j int) bool {
			for _, ob := range stmt.OrderByList {
				vi := fmt.Sprintf("%v", rows[i].Data[ob.Column])
				vj := fmt.Sprintf("%v", rows[j].Data[ob.Column])
				if vi == vj {
					continue // Tie: fall through to next sort column.
				}
				desc := strings.EqualFold(ob.Dir, "DESC")
				if desc {
					return vi > vj
				}
				return vi < vj
			}
			return false
		})
	} else if stmt.OrderBy != "" {
		col := stmt.OrderBy
		desc := strings.EqualFold(stmt.OrderDir, "DESC")
		sort.Slice(rows, func(i, j int) bool {
			vi := fmt.Sprintf("%v", rows[i].Data[col])
			vj := fmt.Sprintf("%v", rows[j].Data[col])
			if desc {
				return vi > vj
			}
			return vi < vj
		})
	}

	// DISTINCT.
	if stmt.Distinct {
		rows = dedup(rows, stmt.Columns)
	}

	// OFFSET (skip rows).
	if stmt.Offset > 0 && len(rows) > stmt.Offset {
		rows = rows[stmt.Offset:]
	} else if stmt.Offset > 0 {
		rows = nil
	}

	// LIMIT (after ordering + offset).
	if stmt.Limit > 0 && len(rows) > stmt.Limit {
		rows = rows[:stmt.Limit]
	}

	// Column projection.
	if len(stmt.Columns) > 0 {
		for i := range rows {
			filtered := make(map[string]interface{})
			for _, col := range stmt.Columns {
				if v, ok := rows[i].Data[col]; ok {
					filtered[col] = v
				}
			}
			rows[i].Data = filtered
		}
	}

	return &Result{Rows: rows, Columns: stmt.Columns}, nil
}

// ── GROUP BY ─────────────────────────────────────────────────────

func (ex *Executor) execGroupBy(rows []table.Row, stmt *Statement) (*Result, error) {
	groups := make(map[string]*GroupRow)
	var order []string

	aggCol := ""
	aggFunc := ""
	if stmt.Agg != nil {
		aggCol = stmt.Agg.Column
		aggFunc = stmt.Agg.Name
	}

	for _, row := range rows {
		// Build group key from GROUP BY columns.
		var keyParts []string
		keyMap := make(map[string]interface{})
		for _, col := range stmt.GroupBy {
			val := row.Data[col]
			keyParts = append(keyParts, fmt.Sprintf("%v", val))
			keyMap[col] = val
		}
		key := strings.Join(keyParts, "|")

		if _, exists := groups[key]; !exists {
			groups[key] = &GroupRow{Key: keyMap}
			order = append(order, key)
		}

		g := groups[key]
		g.Count++

		if aggCol != "" && aggCol != "*" {
			val, ok := row.Data[aggCol]
			if ok {
				f := toFloat(val)
				switch aggFunc {
				case "SUM", "AVG":
					g.Sum += f
				case "MIN":
					if g.Count == 1 || f < g.Min {
						g.Min = f
					}
				case "MAX":
					if g.Count == 1 || f > g.Max {
						g.Max = f
					}
				}
			}
		}
	}

	// Compute AVG.
	for _, g := range groups {
		if aggFunc == "AVG" && g.Count > 0 {
			g.Avg = g.Sum / float64(g.Count)
		}
	}

	// Apply HAVING filter.
	var result []GroupRow
	for _, key := range order {
		g := groups[key]
		if matchesHaving(g, stmt.Having, aggFunc) {
			result = append(result, *g)
		}
	}

	return &Result{Groups: result, Count: len(result), Message: fmt.Sprintf("%d group(s)", len(result))}, nil
}

// matchesHaving evaluates HAVING conditions against a group row.
func matchesHaving(g *GroupRow, having []WhereClause, aggFunc string) bool {
	if len(having) == 0 {
		return true
	}
	for _, h := range having {
		var groupVal float64
		col := strings.ToUpper(h.Column)
		if strings.HasPrefix(col, "COUNT") {
			groupVal = float64(g.Count)
		} else if strings.HasPrefix(col, "SUM") {
			groupVal = g.Sum
		} else if strings.HasPrefix(col, "AVG") {
			groupVal = g.Avg
		} else if strings.HasPrefix(col, "MIN") {
			groupVal = g.Min
		} else if strings.HasPrefix(col, "MAX") {
			groupVal = g.Max
		} else {
			// Direct column comparison on group key.
			if val, ok := g.Key[h.Column]; ok {
				if !table.MatchesFilter(map[string]interface{}{h.Column: val}, table.FilterExpr{
					Conditions: []table.Condition{{Column: h.Column, Op: h.Op, Value: h.Value}},
				}) {
					return false
				}
				continue
			}
			continue
		}

		target := toFloat(h.Value)
		switch h.Op {
		case ">":
			if !(groupVal > target) { return false }
		case "<":
			if !(groupVal < target) { return false }
		case ">=":
			if !(groupVal >= target) { return false }
		case "<=":
			if !(groupVal <= target) { return false }
		case "=":
			if groupVal != target { return false }
		case "!=":
			if groupVal == target { return false }
		}
	}
	return true
}

func toFloat(v interface{}) float64 {
	switch t := v.(type) {
	case float64:
		return t
	case float32:
		return float64(t)
	case int:
		return float64(t)
	case int64:
		return float64(t)
	case uint64:
		return float64(t)
	default:
		return 0
	}
}

// ── DISTINCT ─────────────────────────────────────────────────────

func dedup(rows []table.Row, cols []string) []table.Row {
	seen := make(map[string]bool)
	var result []table.Row

	for _, row := range rows {
		var keyParts []string
		if len(cols) > 0 {
			for _, col := range cols {
				keyParts = append(keyParts, fmt.Sprintf("%v", row.Data[col]))
			}
		} else {
			for _, v := range row.Data {
				keyParts = append(keyParts, fmt.Sprintf("%v", v))
			}
		}
		key := strings.Join(keyParts, "|")
		if !seen[key] {
			seen[key] = true
			result = append(result, row)
		}
	}
	return result
}

// ── Subquery resolution ──────────────────────────────────────────

func (ex *Executor) resolveSubqueries(stmt *Statement) error {
	for i := range stmt.WhereTree {
		if stmt.WhereTree[i].Subquery != nil {
			subResult, err := ex.execSelect(stmt.WhereTree[i].Subquery)
			if err != nil {
				return fmt.Errorf("subquery failed: %w", err)
			}
			// Collect the first column values from subquery results.
			var vals []interface{}
			for _, row := range subResult.Rows {
				for _, v := range row.Data {
					vals = append(vals, v)
					break // Take only first column.
				}
			}
			stmt.WhereTree[i].Value = vals
			stmt.WhereTree[i].Subquery = nil

			// Update the simple Where list too.
			if i < len(stmt.Where) {
				stmt.Where[i].Value = vals
			}
		}
	}
	return nil
}

// ── Aggregation (SUM, AVG, MIN, MAX) ─────────────────────────────

func (ex *Executor) execAggregate(stmt *Statement) (*Result, error) {
	conditions := convertConditions(stmt.Where)
	value, count, err := ex.engine.Aggregate(stmt.Table, stmt.Agg.Name, stmt.Agg.Column, conditions)
	if err != nil {
		return nil, err
	}
	return &Result{
		AggValue: value,
		AggFunc:  stmt.Agg.Name,
		Count:    count,
		Message:  fmt.Sprintf("%s(%s) = %g over %d row(s)", stmt.Agg.Name, stmt.Agg.Column, value, count),
	}, nil
}

// ── JOIN ─────────────────────────────────────────────────────────

func (ex *Executor) execJoin(stmt *Statement) (*Result, error) {
	conditions := convertConditions(stmt.Where)

	var rows []table.Row
	var err error

	if stmt.Join.JoinType == "LEFT" {
		rows, err = ex.engine.SelectLeftJoin(
			stmt.Table, stmt.Join.Table, stmt.Join.ColA, stmt.Join.ColB,
			conditions, stmt.Limit,
		)
	} else {
		rows, err = ex.engine.SelectJoin(
			stmt.Table, stmt.Join.Table, stmt.Join.ColA, stmt.Join.ColB,
			conditions, stmt.Limit,
		)
	}
	if err != nil {
		return nil, err
	}

	if stmt.OrderBy != "" {
		col := stmt.OrderBy
		desc := strings.EqualFold(stmt.OrderDir, "DESC")
		sort.Slice(rows, func(i, j int) bool {
			vi := fmt.Sprintf("%v", rows[i].Data[col])
			vj := fmt.Sprintf("%v", rows[j].Data[col])
			if desc {
				return vi > vj
			}
			return vi < vj
		})
	}

	if len(stmt.Columns) > 0 {
		for i := range rows {
			filtered := make(map[string]interface{})
			for _, col := range stmt.Columns {
				if v, ok := rows[i].Data[col]; ok {
					filtered[col] = v
				}
			}
			rows[i].Data = filtered
		}
	}

	return &Result{Rows: rows, Columns: stmt.Columns}, nil
}

// ── INSERT ───────────────────────────────────────────────────────

func (ex *Executor) execInsert(stmt *Statement) (*Result, error) {
	data := make(map[string]interface{})
	for i, col := range stmt.Columns {
		data[col] = stmt.Values[i]
	}

	id, err := ex.engine.Insert(stmt.Table, data)
	if err != nil {
		return nil, err
	}
	return &Result{InsertID: id, Affected: 1, Message: fmt.Sprintf("inserted row %d", id)}, nil
}

// ── UPSERT ───────────────────────────────────────────────────────

func (ex *Executor) execUpsert(stmt *Statement) (*Result, error) {
	data := make(map[string]interface{})
	for i, col := range stmt.Columns {
		data[col] = stmt.Values[i]
	}

	// Determine the conflict column: explicit ON CONFLICT or first column.
	matchCol := stmt.ConflictCol
	if matchCol == "" && len(stmt.Columns) > 0 {
		matchCol = stmt.Columns[0]
	}
	matchVal := data[matchCol]

	id, updated, err := ex.engine.Upsert(stmt.Table, matchCol, matchVal, data)
	if err != nil {
		return nil, err
	}

	action := "inserted"
	if updated {
		action = "updated"
	}
	return &Result{InsertID: id, Affected: 1, Updated: updated, Message: fmt.Sprintf("%s row %d", action, id)}, nil
}

// ── UPDATE ───────────────────────────────────────────────────────

func (ex *Executor) execUpdate(stmt *Statement) (*Result, error) {
	conditions := convertConditions(stmt.Where)
	count, err := ex.engine.Update(stmt.Table, conditions, stmt.Set)
	if err != nil {
		return nil, err
	}
	return &Result{Affected: count, Message: fmt.Sprintf("updated %d row(s)", count)}, nil
}

// ── DELETE ───────────────────────────────────────────────────────

func (ex *Executor) execDelete(stmt *Statement) (*Result, error) {
	conditions := convertConditions(stmt.Where)
	count, err := ex.engine.Delete(stmt.Table, conditions)
	if err != nil {
		return nil, err
	}
	return &Result{Affected: count, Message: fmt.Sprintf("deleted %d row(s)", count)}, nil
}

// ── CREATE TABLE ─────────────────────────────────────────────────

func (ex *Executor) execCreateTable(stmt *Statement) (*Result, error) {
	var cols []table.Column
	for _, def := range stmt.ColDefs {
		cols = append(cols, table.Column{
			Name: def.Name, Type: mapColumnType(def.Type), Nullable: def.Nullable,
		})
	}
	if err := ex.engine.CreateTable(stmt.Table, cols); err != nil {
		return nil, err
	}
	return &Result{Message: fmt.Sprintf("table %q created", stmt.Table)}, nil
}

// ── DROP TABLE ───────────────────────────────────────────────────

func (ex *Executor) execDropTable(stmt *Statement) (*Result, error) {
	if err := ex.engine.DropTable(stmt.Table); err != nil {
		return nil, err
	}
	return &Result{Message: fmt.Sprintf("table %q dropped", stmt.Table)}, nil
}

// ── ALTER TABLE ──────────────────────────────────────────────────

func (ex *Executor) execAlterTable(stmt *Statement) (*Result, error) {
	var addCols []table.Column
	for _, def := range stmt.AddCols {
		addCols = append(addCols, table.Column{
			Name: def.Name, Type: mapColumnType(def.Type), Nullable: def.Nullable,
		})
	}

	if err := ex.engine.AlterTable(stmt.Table, addCols, stmt.DropCols); err != nil {
		return nil, err
	}

	parts := []string{}
	if len(stmt.AddCols) > 0 {
		names := []string{}
		for _, c := range stmt.AddCols {
			names = append(names, c.Name)
		}
		parts = append(parts, fmt.Sprintf("added [%s]", strings.Join(names, ", ")))
	}
	if len(stmt.DropCols) > 0 {
		parts = append(parts, fmt.Sprintf("dropped [%s]", strings.Join(stmt.DropCols, ", ")))
	}

	return &Result{Message: fmt.Sprintf("table %q altered: %s", stmt.Table, strings.Join(parts, ", "))}, nil
}

// ── CREATE / DROP VIEW ───────────────────────────────────────────

func (ex *Executor) execCreateView(stmt *Statement) (*Result, error) {
	if err := ex.engine.CreateView(stmt.ViewName, stmt.ViewQuery); err != nil {
		return nil, err
	}
	return &Result{Message: fmt.Sprintf("view %q created", stmt.ViewName)}, nil
}

func (ex *Executor) execDropView(stmt *Statement) (*Result, error) {
	if err := ex.engine.DropView(stmt.ViewName); err != nil {
		return nil, err
	}
	return &Result{Message: fmt.Sprintf("view %q dropped", stmt.ViewName)}, nil
}

// ── SHOW TABLES / DESCRIBE ───────────────────────────────────────

func (ex *Executor) execShowTables() (*Result, error) {
	tables, err := ex.engine.ListTables()
	if err != nil {
		return nil, err
	}
	return &Result{Tables: tables, Message: fmt.Sprintf("%d table(s)", len(tables))}, nil
}

func (ex *Executor) execDescribe(stmt *Statement) (*Result, error) {
	cols, err := ex.engine.GetSchema(stmt.Table)
	if err != nil {
		return nil, err
	}
	return &Result{
		Schema:  cols,
		Message: fmt.Sprintf("%d column(s) in %q", len(cols), stmt.Table),
	}, nil
}

// ── OR-aware filtering ───────────────────────────────────────────

func hasOR(nodes []WhereNode) bool {
	for _, n := range nodes {
		if n.Logic == "OR" {
			return true
		}
	}
	return false
}

func filterWithOR(rows []table.Row, nodes []WhereNode) []table.Row {
	var groups [][]table.Condition
	var current []table.Condition

	for _, node := range nodes {
		current = append(current, table.Condition{
			Column: node.Column, Op: node.Op, Value: node.Value,
		})
		if node.Logic == "OR" || node.Logic == "" {
			groups = append(groups, current)
			current = nil
		}
	}

	var result []table.Row
	for _, row := range rows {
		for _, group := range groups {
			if table.MatchesFilter(row.Data, table.FilterExpr{Conditions: group}) {
				result = append(result, row)
				break
			}
		}
	}
	return result
}

// ── Helpers ──────────────────────────────────────────────────────

func convertConditions(where []WhereClause) []table.Condition {
	var conds []table.Condition
	for _, w := range where {
		conds = append(conds, table.Condition{
			Column: w.Column, Op: w.Op, Value: w.Value,
		})
	}
	return conds
}

func mapColumnType(t string) table.ColumnType {
	switch strings.ToUpper(t) {
	case "INT", "INTEGER":
		return table.TypeInt
	case "FLOAT", "DOUBLE", "REAL":
		return table.TypeFloat
	case "BOOL", "BOOLEAN":
		return table.TypeBool
	case "TIME", "TIMESTAMP", "DATETIME":
		return table.TypeTime
	case "JSON", "JSONB":
		return table.TypeJSON
	default:
		return table.TypeString
	}
}
