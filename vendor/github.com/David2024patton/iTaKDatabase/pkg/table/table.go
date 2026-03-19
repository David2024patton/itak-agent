package table

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	bolt "go.etcd.io/bbolt"
)

// ── Column types ──────────────────────────────────────────────────

type ColumnType int

const (
	TypeString ColumnType = iota
	TypeInt
	TypeFloat
	TypeBool
	TypeTime
	TypeJSON
)

// Column defines a single table column.
type Column struct {
	Name     string
	Type     ColumnType
	Nullable bool
}

// Row represents a single table row with an auto-incrementing primary key.
type Row struct {
	ID   uint64
	Data map[string]interface{}
}

// Condition represents a WHERE clause condition.
type Condition struct {
	Column string
	Op     string // "=", ">", "<", ">=", "<=", "LIKE", "IN", "!=", "BETWEEN"
	Value  interface{}
}

// ForeignKey defines a foreign key constraint.
type ForeignKey struct {
	Column    string
	RefTable  string
	RefColumn string
}

// ViewDef stores a view definition.
type ViewDef struct {
	Name  string `json:"name"`
	Query string `json:"query"`
}

// ── Table Engine ──────────────────────────────────────────────────

// Engine provides SQL-like table operations on top of bbolt.
//
// What: Structured row storage with typed columns and filtered queries.
// Why:  AI agents need structured data for facts, sessions, config,
//       and any tabular data alongside graph and vector stores.
// How:  Each table gets its own bbolt bucket. Rows are JSON-encoded
//       with auto-incrementing uint64 keys. Schema is stored in a
//       special _meta bucket.
type Engine struct {
	db *bolt.DB
	mu sync.RWMutex
}

// NewEngine wraps an existing bolt.DB for table operations.
func NewEngine(db *bolt.DB) *Engine {
	return &Engine{db: db}
}

// CreateTable defines a new table schema. Idempotent (no-op if exists).
func (e *Engine) CreateTable(name string, columns []Column) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	return e.db.Update(func(tx *bolt.Tx) error {
		// Create the data bucket.
		_, err := tx.CreateBucketIfNotExists([]byte("tbl_" + name))
		if err != nil {
			return fmt.Errorf("create table %s: %w", name, err)
		}

		// Store schema in _meta.
		meta, err := tx.CreateBucketIfNotExists([]byte("_table_meta"))
		if err != nil {
			return err
		}

		schema, _ := json.Marshal(columns)
		return meta.Put([]byte(name), schema)
	})
}

// Insert adds a row to the table and returns the auto-generated ID.
func (e *Engine) Insert(table string, data map[string]interface{}) (uint64, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	var id uint64
	err := e.db.Update(func(tx *bolt.Tx) error {
		bkt := tx.Bucket([]byte("tbl_" + table))
		if bkt == nil {
			return fmt.Errorf("table %q does not exist", table)
		}

		// Auto-increment ID.
		seq, err := bkt.NextSequence()
		if err != nil {
			return err
		}
		id = seq

		// Add timestamp if not present.
		if _, ok := data["_created_at"]; !ok {
			data["_created_at"] = time.Now().Format(time.RFC3339)
		}

		rowBytes, err := json.Marshal(data)
		if err != nil {
			return err
		}

		key := make([]byte, 8)
		binary.BigEndian.PutUint64(key, id)
		return bkt.Put(key, rowBytes)
	})

	return id, err
}

// Select queries rows from a table with optional WHERE conditions, ordering, and limit.
func (e *Engine) Select(table string, where []Condition, limit int) ([]Row, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	var results []Row

	err := e.db.View(func(tx *bolt.Tx) error {
		bkt := tx.Bucket([]byte("tbl_" + table))
		if bkt == nil {
			return fmt.Errorf("table %q does not exist", table)
		}

		c := bkt.Cursor()
		for k, v := c.First(); k != nil; k, v = c.Next() {
			if limit > 0 && len(results) >= limit {
				break
			}

			var data map[string]interface{}
			if err := json.Unmarshal(v, &data); err != nil {
				continue
			}

			if matchesConditions(data, where) {
				id := binary.BigEndian.Uint64(k)
				results = append(results, Row{ID: id, Data: data})
			}
		}
		return nil
	})

	return results, err
}

// Update modifies rows matching the WHERE conditions.
func (e *Engine) Update(table string, where []Condition, set map[string]interface{}) (int, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	count := 0
	err := e.db.Update(func(tx *bolt.Tx) error {
		bkt := tx.Bucket([]byte("tbl_" + table))
		if bkt == nil {
			return fmt.Errorf("table %q does not exist", table)
		}

		c := bkt.Cursor()
		for k, v := c.First(); k != nil; k, v = c.Next() {
			var data map[string]interface{}
			if err := json.Unmarshal(v, &data); err != nil {
				continue
			}

			if matchesConditions(data, where) {
				for key, val := range set {
					data[key] = val
				}
				data["_updated_at"] = time.Now().Format(time.RFC3339)

				updated, err := json.Marshal(data)
				if err != nil {
					continue
				}
				if err := bkt.Put(k, updated); err != nil {
					return err
				}
				count++
			}
		}
		return nil
	})

	return count, err
}

// Delete removes rows matching the WHERE conditions.
func (e *Engine) Delete(table string, where []Condition) (int, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	count := 0
	err := e.db.Update(func(tx *bolt.Tx) error {
		bkt := tx.Bucket([]byte("tbl_" + table))
		if bkt == nil {
			return fmt.Errorf("table %q does not exist", table)
		}

		// Collect keys to delete (can't delete during iteration).
		var toDelete [][]byte
		c := bkt.Cursor()
		for k, v := c.First(); k != nil; k, v = c.Next() {
			var data map[string]interface{}
			if err := json.Unmarshal(v, &data); err != nil {
				continue
			}
			if matchesConditions(data, where) {
				keyCopy := make([]byte, len(k))
				copy(keyCopy, k)
				toDelete = append(toDelete, keyCopy)
			}
		}

		for _, k := range toDelete {
			if err := bkt.Delete(k); err != nil {
				return err
			}
			count++
		}
		return nil
	})

	return count, err
}

// Count returns the number of rows in a table.
func (e *Engine) Count(table string) (int, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	count := 0
	err := e.db.View(func(tx *bolt.Tx) error {
		bkt := tx.Bucket([]byte("tbl_" + table))
		if bkt == nil {
			return fmt.Errorf("table %q does not exist", table)
		}
		count = bkt.Stats().KeyN
		return nil
	})
	return count, err
}

// ListTables returns the names of all defined tables.
func (e *Engine) ListTables() ([]string, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	var tables []string
	err := e.db.View(func(tx *bolt.Tx) error {
		meta := tx.Bucket([]byte("_table_meta"))
		if meta == nil {
			return nil
		}
		return meta.ForEach(func(k, _ []byte) error {
			tables = append(tables, string(k))
			return nil
		})
	})
	return tables, err
}

// DropTable removes a table and its schema. Returns error if table doesn't exist.
func (e *Engine) DropTable(name string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	return e.db.Update(func(tx *bolt.Tx) error {
		bkt := tx.Bucket([]byte("tbl_" + name))
		if bkt == nil {
			return fmt.Errorf("table %q does not exist", name)
		}
		if err := tx.DeleteBucket([]byte("tbl_" + name)); err != nil {
			return fmt.Errorf("drop table %s: %w", name, err)
		}

		// Remove schema from _meta.
		meta := tx.Bucket([]byte("_table_meta"))
		if meta != nil {
			meta.Delete([]byte(name))
		}
		return nil
	})
}

// GetSchema returns the column definitions for a table.
func (e *Engine) GetSchema(name string) ([]Column, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	var cols []Column
	err := e.db.View(func(tx *bolt.Tx) error {
		meta := tx.Bucket([]byte("_table_meta"))
		if meta == nil {
			return fmt.Errorf("no tables exist")
		}
		schema := meta.Get([]byte(name))
		if schema == nil {
			return fmt.Errorf("table %q does not exist", name)
		}
		return json.Unmarshal(schema, &cols)
	})
	return cols, err
}

// AlterTable modifies a table schema: add or drop columns.
//
// What: Schema migration for existing tables.
// Why:  Agents evolve their data models over time. Columns need to be added
//       or removed without recreating entire tables.
// How:  Updates the schema in _meta and optionally migrates existing rows.
func (e *Engine) AlterTable(name string, addCols []Column, dropCols []string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	return e.db.Update(func(tx *bolt.Tx) error {
		meta := tx.Bucket([]byte("_table_meta"))
		if meta == nil {
			return fmt.Errorf("no tables exist")
		}
		schemaBytes := meta.Get([]byte(name))
		if schemaBytes == nil {
			return fmt.Errorf("table %q does not exist", name)
		}

		var cols []Column
		if err := json.Unmarshal(schemaBytes, &cols); err != nil {
			return err
		}

		// Drop columns from schema.
		dropSet := make(map[string]bool)
		for _, d := range dropCols {
			dropSet[d] = true
		}
		var remaining []Column
		for _, c := range cols {
			if !dropSet[c.Name] {
				remaining = append(remaining, c)
			}
		}

		// Add new columns.
		remaining = append(remaining, addCols...)

		// Persist updated schema.
		newSchema, _ := json.Marshal(remaining)
		if err := meta.Put([]byte(name), newSchema); err != nil {
			return err
		}

		// Migrate existing rows: remove dropped columns from row data.
		if len(dropCols) > 0 {
			bkt := tx.Bucket([]byte("tbl_" + name))
			if bkt == nil {
				return nil
			}
			c := bkt.Cursor()
			for k, v := c.First(); k != nil; k, v = c.Next() {
				var data map[string]interface{}
				if err := json.Unmarshal(v, &data); err != nil {
					continue
				}
				changed := false
				for _, d := range dropCols {
					if _, ok := data[d]; ok {
						delete(data, d)
						changed = true
					}
				}
				if changed {
					updated, err := json.Marshal(data)
					if err != nil {
						continue
					}
					bkt.Put(k, updated)
				}
			}
		}
		return nil
	})
}

// Backup creates a safe copy of the database file at the given path.
//
// What: Point-in-time snapshot of the entire database.
// Why:  Data safety for embedded databases that can't use replication.
// How:  Uses bbolt's read transaction to get a consistent snapshot,
//       writing it to the target file without blocking writes.
func (e *Engine) Backup(destPath string) error {
	e.mu.RLock()
	defer e.mu.RUnlock()

	return e.db.View(func(tx *bolt.Tx) error {
		return tx.CopyFile(destPath, 0600)
	})
}

// BoltDB returns the underlying bolt database handle.
// This is used for backup operations at the itakdb level.
func (e *Engine) BoltDB() *bolt.DB {
	return e.db
}

// Aggregate computes an aggregation (SUM, AVG, MIN, MAX) over a column.
func (e *Engine) Aggregate(table string, op string, column string, where []Condition) (float64, int, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	var result float64
	count := 0
	initialized := false

	err := e.db.View(func(tx *bolt.Tx) error {
		bkt := tx.Bucket([]byte("tbl_" + table))
		if bkt == nil {
			return fmt.Errorf("table %q does not exist", table)
		}

		c := bkt.Cursor()
		for k, v := c.First(); k != nil; k, v = c.Next() {
			var data map[string]interface{}
			if err := json.Unmarshal(v, &data); err != nil {
				continue
			}

			if !matchesConditions(data, where) {
				continue
			}

			val, ok := data[column]
			if !ok {
				continue
			}

			f := toFloat(val)
			count++

			switch strings.ToUpper(op) {
			case "SUM":
				result += f
			case "AVG":
				result += f
			case "MIN":
				if !initialized || f < result {
					result = f
					initialized = true
				}
			case "MAX":
				if !initialized || f > result {
					result = f
					initialized = true
				}
			}
		}
		return nil
	})

	if err != nil {
		return 0, 0, err
	}

	if strings.ToUpper(op) == "AVG" && count > 0 {
		result = result / float64(count)
	}

	return result, count, nil
}

// SelectJoin performs an INNER JOIN between two tables on matching columns.
//
// What: Cross-table query combining rows from two tables.
// Why:  Relational queries need joins to correlate related data.
// How:  Nested loop join -- for each row in table A, scan table B
//       for matching values on the join columns.
func (e *Engine) SelectJoin(tableA, tableB, colA, colB string, where []Condition, limit int) ([]Row, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	var results []Row

	err := e.db.View(func(tx *bolt.Tx) error {
		bktA := tx.Bucket([]byte("tbl_" + tableA))
		bktB := tx.Bucket([]byte("tbl_" + tableB))
		if bktA == nil {
			return fmt.Errorf("table %q does not exist", tableA)
		}
		if bktB == nil {
			return fmt.Errorf("table %q does not exist", tableB)
		}

		// Build index on table B for the join column.
		bIndex := make(map[string][]map[string]interface{})
		cb := bktB.Cursor()
		for k, v := cb.First(); k != nil; k, v = cb.Next() {
			var data map[string]interface{}
			if err := json.Unmarshal(v, &data); err != nil {
				continue
			}
			key := fmt.Sprintf("%v", data[colB])
			// Prefix all B columns with "b." to avoid collisions.
			prefixed := make(map[string]interface{})
			for dk, dv := range data {
				prefixed[tableB+"."+dk] = dv
			}
			bIndex[key] = append(bIndex[key], prefixed)
		}

		// Scan table A and join.
		ca := bktA.Cursor()
		for k, v := ca.First(); k != nil; k, v = ca.Next() {
			if limit > 0 && len(results) >= limit {
				break
			}

			var dataA map[string]interface{}
			if err := json.Unmarshal(v, &dataA); err != nil {
				continue
			}

			joinKey := fmt.Sprintf("%v", dataA[colA])
			matchingBRows, found := bIndex[joinKey]
			if !found {
				continue
			}

			for _, bRow := range matchingBRows {
				if limit > 0 && len(results) >= limit {
					break
				}

				// Merge A and B data.
				merged := make(map[string]interface{})
				for dk, dv := range dataA {
					merged[tableA+"."+dk] = dv
				}
				for dk, dv := range bRow {
					merged[dk] = dv
				}

				if matchesConditions(merged, where) {
					id := binary.BigEndian.Uint64(k)
					results = append(results, Row{ID: id, Data: merged})
				}
			}
		}
		return nil
	})

	return results, err
}

// ── Condition matching ────────────────────────────────────────────

// WhereGroup holds conditions connected by AND or OR.
type WhereGroup struct {
	Conditions []Condition
	Logic      string // "AND" or "OR"
}

// MatchesFilter evaluates a FilterExpr (supports AND/OR groups).
func MatchesFilter(data map[string]interface{}, filter FilterExpr) bool {
	if len(filter.Groups) == 0 {
		// Simple AND-only mode for backward compatibility.
		return matchesConditions(data, filter.Conditions)
	}

	for _, group := range filter.Groups {
		groupResult := evaluateGroup(data, group)
		if filter.Logic == "OR" {
			if groupResult {
				return true
			}
		} else {
			// AND (default)
			if !groupResult {
				return false
			}
		}
	}

	if filter.Logic == "OR" {
		return false
	}
	return true
}

// FilterExpr represents a compound WHERE with AND/OR support.
type FilterExpr struct {
	Conditions []Condition  // Simple AND-only conditions (backward compat)
	Groups     []WhereGroup // Grouped conditions with AND/OR
	Logic      string       // Top-level logic: "AND" or "OR"
}

func evaluateGroup(data map[string]interface{}, group WhereGroup) bool {
	for i, cond := range group.Conditions {
		val, ok := data[cond.Column]
		if !ok {
			if cond.Op == "=" && cond.Value == nil {
				if group.Logic == "OR" && i < len(group.Conditions)-1 {
					continue
				}
				continue
			}
			if group.Logic == "OR" {
				continue
			}
			return false
		}
		match := matchCondition(val, cond)
		if group.Logic == "OR" {
			if match {
				return true
			}
		} else {
			if !match {
				return false
			}
		}
	}

	if group.Logic == "OR" {
		return false
	}
	return true
}

func matchesConditions(data map[string]interface{}, conditions []Condition) bool {
	for _, cond := range conditions {
		val, ok := data[cond.Column]
		if !ok {
			if cond.Op == "=" && cond.Value == nil {
				continue // NULL check matches missing
			}
			return false
		}

		if !matchCondition(val, cond) {
			return false
		}
	}
	return true
}

func matchCondition(val interface{}, cond Condition) bool {
	switch cond.Op {
	case "=", "==":
		return fmt.Sprintf("%v", val) == fmt.Sprintf("%v", cond.Value)
	case "!=":
		return fmt.Sprintf("%v", val) != fmt.Sprintf("%v", cond.Value)
	case ">":
		return toFloat(val) > toFloat(cond.Value)
	case "<":
		return toFloat(val) < toFloat(cond.Value)
	case ">=":
		return toFloat(val) >= toFloat(cond.Value)
	case "<=":
		return toFloat(val) <= toFloat(cond.Value)
	case "LIKE", "like":
		return matchLike(fmt.Sprintf("%v", val), fmt.Sprintf("%v", cond.Value))
	case "IN", "in":
		return matchIn(val, cond.Value)
	case "BETWEEN", "between":
		return matchBetween(val, cond.Value)
	default:
		return false
	}
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
	case json.Number:
		f, _ := t.Float64()
		return f
	default:
		return 0
	}
}

func matchLike(val, pattern string) bool {
	pattern = strings.ToLower(pattern)
	val = strings.ToLower(val)

	if strings.HasPrefix(pattern, "%") && strings.HasSuffix(pattern, "%") {
		return strings.Contains(val, strings.Trim(pattern, "%"))
	}
	if strings.HasPrefix(pattern, "%") {
		return strings.HasSuffix(val, strings.TrimPrefix(pattern, "%"))
	}
	if strings.HasSuffix(pattern, "%") {
		return strings.HasPrefix(val, strings.TrimSuffix(pattern, "%"))
	}
	return val == pattern
}

func matchIn(val, list interface{}) bool {
	items, ok := list.([]interface{})
	if !ok {
		return false
	}
	valStr := fmt.Sprintf("%v", val)
	for _, item := range items {
		if fmt.Sprintf("%v", item) == valStr {
			return true
		}
	}
	return false
}

func matchBetween(val, rangeVal interface{}) bool {
	pair, ok := rangeVal.([]interface{})
	if !ok || len(pair) != 2 {
		return false
	}
	v := toFloat(val)
	low := toFloat(pair[0])
	high := toFloat(pair[1])
	if low == 0 && high == 0 {
		// Try string comparison for dates/strings.
		vs := fmt.Sprintf("%v", val)
		ls := fmt.Sprintf("%v", pair[0])
		hs := fmt.Sprintf("%v", pair[1])
		return vs >= ls && vs <= hs
	}
	return v >= low && v <= high
}

// ── Upsert ─────────────────────────────────────────────────────────

// Upsert inserts a row or updates it if a matching row exists.
//
// What: INSERT OR UPDATE semantics.
// Why:  Agents often want to set a value regardless of whether the row exists.
// How:  Checks for a matching row using matchCol/matchVal. If found, updates.
//       If not, inserts.
func (e *Engine) Upsert(tableName string, matchCol string, matchVal interface{}, data map[string]interface{}) (uint64, bool, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	var id uint64
	updated := false

	err := e.db.Update(func(tx *bolt.Tx) error {
		bkt := tx.Bucket([]byte("tbl_" + tableName))
		if bkt == nil {
			return fmt.Errorf("table %q does not exist", tableName)
		}

		// Scan for existing row.
		c := bkt.Cursor()
		for k, v := c.First(); k != nil; k, v = c.Next() {
			var existing map[string]interface{}
			if err := json.Unmarshal(v, &existing); err != nil {
				continue
			}
			if fmt.Sprintf("%v", existing[matchCol]) == fmt.Sprintf("%v", matchVal) {
				// Update existing row.
				for dk, dv := range data {
					existing[dk] = dv
				}
				existing["_updated_at"] = time.Now().Format(time.RFC3339)
				rowBytes, err := json.Marshal(existing)
				if err != nil {
					return err
				}
				id = binary.BigEndian.Uint64(k)
				updated = true
				return bkt.Put(k, rowBytes)
			}
		}

		// Insert new row.
		seq, err := bkt.NextSequence()
		if err != nil {
			return err
		}
		id = seq
		data["_created_at"] = time.Now().Format(time.RFC3339)
		rowBytes, err := json.Marshal(data)
		if err != nil {
			return err
		}
		key := make([]byte, 8)
		binary.BigEndian.PutUint64(key, id)
		return bkt.Put(key, rowBytes)
	})

	return id, updated, err
}

// ── LEFT JOIN ──────────────────────────────────────────────────────

// SelectLeftJoin performs a LEFT JOIN. All rows from table A are returned,
// with NULL-equivalent merged data when no match exists in table B.
func (e *Engine) SelectLeftJoin(tableA, tableB, colA, colB string, where []Condition, limit int) ([]Row, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	var results []Row

	err := e.db.View(func(tx *bolt.Tx) error {
		bktA := tx.Bucket([]byte("tbl_" + tableA))
		bktB := tx.Bucket([]byte("tbl_" + tableB))
		if bktA == nil {
			return fmt.Errorf("table %q does not exist", tableA)
		}
		if bktB == nil {
			return fmt.Errorf("table %q does not exist", tableB)
		}

		// Build index on table B.
		bIndex := make(map[string][]map[string]interface{})
		cb := bktB.Cursor()
		for k, v := cb.First(); k != nil; k, v = cb.Next() {
			var data map[string]interface{}
			if err := json.Unmarshal(v, &data); err != nil {
				continue
			}
			key := fmt.Sprintf("%v", data[colB])
			prefixed := make(map[string]interface{})
			for dk, dv := range data {
				prefixed[tableB+"."+dk] = dv
			}
			bIndex[key] = append(bIndex[key], prefixed)
		}

		// Scan table A.
		ca := bktA.Cursor()
		for k, v := ca.First(); k != nil; k, v = ca.Next() {
			if limit > 0 && len(results) >= limit {
				break
			}

			var dataA map[string]interface{}
			if err := json.Unmarshal(v, &dataA); err != nil {
				continue
			}

			joinKey := fmt.Sprintf("%v", dataA[colA])
			matches, found := bIndex[joinKey]

			if found {
				for _, bRow := range matches {
					if limit > 0 && len(results) >= limit {
						break
					}
					merged := make(map[string]interface{})
					for dk, dv := range dataA {
						merged[tableA+"."+dk] = dv
					}
					for dk, dv := range bRow {
						merged[dk] = dv
					}
					if matchesConditions(merged, where) {
						id := binary.BigEndian.Uint64(k)
						results = append(results, Row{ID: id, Data: merged})
					}
				}
			} else {
				// LEFT JOIN: include row from A with no B data.
				merged := make(map[string]interface{})
				for dk, dv := range dataA {
					merged[tableA+"."+dk] = dv
				}
				if matchesConditions(merged, where) {
					id := binary.BigEndian.Uint64(k)
					results = append(results, Row{ID: id, Data: merged})
				}
			}
		}
		return nil
	})

	return results, err
}

// ── Views ──────────────────────────────────────────────────────────

// CreateView stores a named view (saved query).
func (e *Engine) CreateView(name, query string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	return e.db.Update(func(tx *bolt.Tx) error {
		meta, err := tx.CreateBucketIfNotExists([]byte("_views"))
		if err != nil {
			return err
		}
		view := ViewDef{Name: name, Query: query}
		data, _ := json.Marshal(view)
		return meta.Put([]byte(name), data)
	})
}

// GetView retrieves a stored view's query.
func (e *Engine) GetView(name string) (string, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	var query string
	err := e.db.View(func(tx *bolt.Tx) error {
		meta := tx.Bucket([]byte("_views"))
		if meta == nil {
			return fmt.Errorf("no views exist")
		}
		data := meta.Get([]byte(name))
		if data == nil {
			return fmt.Errorf("view %q does not exist", name)
		}
		var view ViewDef
		if err := json.Unmarshal(data, &view); err != nil {
			return err
		}
		query = view.Query
		return nil
	})
	return query, err
}

// DropView removes a stored view.
func (e *Engine) DropView(name string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	return e.db.Update(func(tx *bolt.Tx) error {
		meta := tx.Bucket([]byte("_views"))
		if meta == nil {
			return fmt.Errorf("no views exist")
		}
		if meta.Get([]byte(name)) == nil {
			return fmt.Errorf("view %q does not exist", name)
		}
		return meta.Delete([]byte(name))
	})
}

// ListViews returns the names of all stored views.
func (e *Engine) ListViews() ([]string, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	var views []string
	err := e.db.View(func(tx *bolt.Tx) error {
		meta := tx.Bucket([]byte("_views"))
		if meta == nil {
			return nil
		}
		return meta.ForEach(func(k, _ []byte) error {
			views = append(views, string(k))
			return nil
		})
	})
	return views, err
}

// ── Foreign Keys ───────────────────────────────────────────────────

// AddForeignKey stores a foreign key constraint for a table.
func (e *Engine) AddForeignKey(tableName string, fk ForeignKey) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	return e.db.Update(func(tx *bolt.Tx) error {
		meta, err := tx.CreateBucketIfNotExists([]byte("_foreign_keys"))
		if err != nil {
			return err
		}

		// Load existing FKs for this table.
		var fks []ForeignKey
		existing := meta.Get([]byte(tableName))
		if existing != nil {
			json.Unmarshal(existing, &fks)
		}

		fks = append(fks, fk)
		data, _ := json.Marshal(fks)
		return meta.Put([]byte(tableName), data)
	})
}

// CheckForeignKey validates that a value exists in the referenced table.
func (e *Engine) CheckForeignKey(tableName string, data map[string]interface{}) error {
	e.mu.RLock()
	defer e.mu.RUnlock()

	return e.db.View(func(tx *bolt.Tx) error {
		meta := tx.Bucket([]byte("_foreign_keys"))
		if meta == nil {
			return nil // No FKs defined.
		}

		fkData := meta.Get([]byte(tableName))
		if fkData == nil {
			return nil
		}

		var fks []ForeignKey
		if err := json.Unmarshal(fkData, &fks); err != nil {
			return err
		}

		for _, fk := range fks {
			val, ok := data[fk.Column]
			if !ok {
				continue // Column not in data, skip.
			}

			refBkt := tx.Bucket([]byte("tbl_" + fk.RefTable))
			if refBkt == nil {
				return fmt.Errorf("foreign key: referenced table %q does not exist", fk.RefTable)
			}

			found := false
			c := refBkt.Cursor()
			for k, v := c.First(); k != nil; k, v = c.Next() {
				var refData map[string]interface{}
				if err := json.Unmarshal(v, &refData); err != nil {
					continue
				}
				if fmt.Sprintf("%v", refData[fk.RefColumn]) == fmt.Sprintf("%v", val) {
					found = true
					break
				}
			}
			if !found {
				return fmt.Errorf("foreign key violation: %s.%s = %v not found in %s.%s",
					tableName, fk.Column, val, fk.RefTable, fk.RefColumn)
			}
		}
		return nil
	})
}

// ── Export / Import ────────────────────────────────────────────────

// ExportJSON writes all rows from a table as JSON to the writer.
func (e *Engine) ExportJSON(tableName string, w io.Writer) (int, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	count := 0
	enc := json.NewEncoder(w)

	err := e.db.View(func(tx *bolt.Tx) error {
		bkt := tx.Bucket([]byte("tbl_" + tableName))
		if bkt == nil {
			return fmt.Errorf("table %q does not exist", tableName)
		}

		c := bkt.Cursor()
		for k, v := c.First(); k != nil; k, v = c.Next() {
			var data map[string]interface{}
			if err := json.Unmarshal(v, &data); err != nil {
				continue
			}
			id := binary.BigEndian.Uint64(k)
			row := map[string]interface{}{"id": id, "data": data}
			if err := enc.Encode(row); err != nil {
				return err
			}
			count++
		}
		return nil
	})

	return count, err
}

// ImportJSON reads JSON rows from the reader and inserts them into a table.
func (e *Engine) ImportJSON(tableName string, r io.Reader) (int, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	count := 0
	dec := json.NewDecoder(r)

	err := e.db.Update(func(tx *bolt.Tx) error {
		bkt := tx.Bucket([]byte("tbl_" + tableName))
		if bkt == nil {
			return fmt.Errorf("table %q does not exist", tableName)
		}

		for dec.More() {
			var row map[string]interface{}
			if err := dec.Decode(&row); err != nil {
				break
			}

			// Extract data field or use the whole row.
			data, ok := row["data"].(map[string]interface{})
			if !ok {
				data = row
			}

			seq, err := bkt.NextSequence()
			if err != nil {
				return err
			}

			rowBytes, err := json.Marshal(data)
			if err != nil {
				continue
			}

			key := make([]byte, 8)
			binary.BigEndian.PutUint64(key, seq)
			if err := bkt.Put(key, rowBytes); err != nil {
				return err
			}
			count++
		}
		return nil
	})

	return count, err
}

