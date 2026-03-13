package table

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
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
	Op     string // "=", ">", "<", ">=", "<=", "LIKE", "IN", "!="
	Value  interface{}
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

// ── Condition matching ────────────────────────────────────────────

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
	// Simple LIKE: % = wildcard.
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
