package search

import (
	"math"
	"strings"
	"sync"
	"unicode"

	bolt "go.etcd.io/bbolt"
)

// Engine provides full-text search with BM25 ranking.
//
// What: Inverted index with BM25 scoring for keyword search.
// Why:  AI agents need fast keyword lookup across all stored text
//       (conversations, facts, tool results, page content).
// How:  Tokenizes text into terms, builds an inverted index in bbolt,
//       and scores matches using BM25 (k1=1.2, b=0.75).
type Engine struct {
	db        *bolt.DB
	mu        sync.RWMutex
	docCount  int
	avgDocLen float64
}

// Result represents a single search hit.
type Result struct {
	DocID uint64
	Score float64
}

// NewEngine creates a full-text search engine backed by bbolt.
func NewEngine(db *bolt.DB) *Engine {
	e := &Engine{db: db}
	e.loadStats()
	return e
}

// IndexDocument tokenizes and indexes a document's text.
func (e *Engine) IndexDocument(docID uint64, text string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	tokens := tokenize(text)
	if len(tokens) == 0 {
		return nil
	}

	// Count term frequency for this document.
	termFreqs := make(map[string]int)
	for _, token := range tokens {
		termFreqs[token]++
	}

	return e.db.Update(func(tx *bolt.Tx) error {
		// Inverted index bucket: term -> list of (docID, tf) pairs.
		idx, err := tx.CreateBucketIfNotExists([]byte("_fts_index"))
		if err != nil {
			return err
		}

		// Document lengths bucket.
		lens, err := tx.CreateBucketIfNotExists([]byte("_fts_lengths"))
		if err != nil {
			return err
		}

		// Stats bucket.
		stats, err := tx.CreateBucketIfNotExists([]byte("_fts_stats"))
		if err != nil {
			return err
		}

		// Store document length.
		docKey := uint64ToBytes(docID)
		lens.Put(docKey, uint64ToBytes(uint64(len(tokens))))

		// For each unique term, append this document to the posting list.
		for term, freq := range termFreqs {
			termKey := []byte(term)

			// Get existing posting list.
			var postings []posting
			if existing := idx.Get(termKey); existing != nil {
				postings = decodePostings(existing)
			}

			// Remove existing entry for this doc (re-index).
			filtered := make([]posting, 0, len(postings))
			for _, p := range postings {
				if p.DocID != docID {
					filtered = append(filtered, p)
				}
			}

			// Append new entry.
			filtered = append(filtered, posting{DocID: docID, TermFreq: freq})
			idx.Put(termKey, encodePostings(filtered))
		}

		// Update global stats.
		e.docCount++
		totalLen := float64(e.docCount-1)*e.avgDocLen + float64(len(tokens))
		e.avgDocLen = totalLen / float64(e.docCount)

		stats.Put([]byte("doc_count"), uint64ToBytes(uint64(e.docCount)))
		stats.Put([]byte("avg_doc_len"), float64ToBytes(e.avgDocLen))

		return nil
	})
}

// Search performs a BM25-ranked keyword search.
func (e *Engine) Search(query string, limit int) ([]Result, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	queryTokens := tokenize(query)
	if len(queryTokens) == 0 {
		return nil, nil
	}

	// Collect BM25 scores per document.
	scores := make(map[uint64]float64)

	err := e.db.View(func(tx *bolt.Tx) error {
		idx := tx.Bucket([]byte("_fts_index"))
		if idx == nil {
			return nil
		}

		lens := tx.Bucket([]byte("_fts_lengths"))

		for _, term := range queryTokens {
			postingData := idx.Get([]byte(term))
			if postingData == nil {
				continue
			}

			postings := decodePostings(postingData)
			df := len(postings) // document frequency

			for _, p := range postings {
				docLen := 1.0
				if lens != nil {
					if dl := lens.Get(uint64ToBytes(p.DocID)); dl != nil {
						docLen = float64(bytesToUint64(dl))
					}
				}

				score := bm25(float64(p.TermFreq), float64(df), float64(e.docCount), docLen, e.avgDocLen)
				scores[p.DocID] += score
			}
		}
		return nil
	})

	if err != nil {
		return nil, err
	}

	// Sort by score descending.
	results := make([]Result, 0, len(scores))
	for docID, score := range scores {
		results = append(results, Result{DocID: docID, Score: score})
	}

	// Simple selection sort (fine for typical result sets).
	for i := 0; i < len(results)-1; i++ {
		maxIdx := i
		for j := i + 1; j < len(results); j++ {
			if results[j].Score > results[maxIdx].Score {
				maxIdx = j
			}
		}
		results[i], results[maxIdx] = results[maxIdx], results[i]
	}

	if limit > 0 && limit < len(results) {
		results = results[:limit]
	}

	return results, nil
}

// RemoveDocument removes a document from the full-text index.
func (e *Engine) RemoveDocument(docID uint64) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	return e.db.Update(func(tx *bolt.Tx) error {
		idx := tx.Bucket([]byte("_fts_index"))
		if idx == nil {
			return nil
		}

		// Scan all terms and remove this docID from posting lists.
		c := idx.Cursor()
		for k, v := c.First(); k != nil; k, v = c.Next() {
			postings := decodePostings(v)
			filtered := make([]posting, 0, len(postings))
			for _, p := range postings {
				if p.DocID != docID {
					filtered = append(filtered, p)
				}
			}
			if len(filtered) < len(postings) {
				if len(filtered) == 0 {
					idx.Delete(k)
				} else {
					idx.Put(k, encodePostings(filtered))
				}
			}
		}

		// Remove doc length.
		lens := tx.Bucket([]byte("_fts_lengths"))
		if lens != nil {
			lens.Delete(uint64ToBytes(docID))
		}

		return nil
	})
}

// DocumentCount returns the number of indexed documents.
func (e *Engine) DocumentCount() int {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.docCount
}

// ── BM25 scoring ──────────────────────────────────────────────────

const (
	bm25K1 = 1.2
	bm25B  = 0.75
)

func bm25(tf, df, n, docLen, avgDocLen float64) float64 {
	idf := math.Log((n - df + 0.5) / (df + 0.5))
	if idf < 0 {
		idf = 0
	}
	tfNorm := (tf * (bm25K1 + 1)) / (tf + bm25K1*(1-bm25B+bm25B*(docLen/avgDocLen)))
	return idf * tfNorm
}

// ── Tokenization ──────────────────────────────────────────────────

func tokenize(text string) []string {
	text = strings.ToLower(text)
	var tokens []string
	var current strings.Builder

	for _, r := range text {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			current.WriteRune(r)
		} else {
			if current.Len() > 0 {
				token := current.String()
				if len(token) >= 2 && !isStopWord(token) {
					tokens = append(tokens, token)
				}
				current.Reset()
			}
		}
	}
	if current.Len() > 0 {
		token := current.String()
		if len(token) >= 2 && !isStopWord(token) {
			tokens = append(tokens, token)
		}
	}

	return tokens
}

var stopWords = map[string]bool{
	"the": true, "is": true, "at": true, "of": true,
	"on": true, "in": true, "to": true, "for": true,
	"and": true, "or": true, "an": true, "a": true,
	"it": true, "be": true, "as": true, "by": true,
	"this": true, "that": true, "was": true, "are": true,
	"with": true, "from": true, "has": true, "had": true,
	"have": true, "not": true, "but": true, "its": true,
}

func isStopWord(w string) bool {
	return stopWords[w]
}

// ── Posting list encoding ─────────────────────────────────────────

type posting struct {
	DocID    uint64
	TermFreq int
}

func encodePostings(postings []posting) []byte {
	// Simple encoding: 8 bytes docID + 4 bytes tf per entry.
	data := make([]byte, len(postings)*12)
	for i, p := range postings {
		offset := i * 12
		putUint64(data[offset:], p.DocID)
		putUint32(data[offset+8:], uint32(p.TermFreq))
	}
	return data
}

func decodePostings(data []byte) []posting {
	n := len(data) / 12
	postings := make([]posting, n)
	for i := 0; i < n; i++ {
		offset := i * 12
		postings[i] = posting{
			DocID:    getUint64(data[offset:]),
			TermFreq: int(getUint32(data[offset+8:])),
		}
	}
	return postings
}

// ── Binary helpers ────────────────────────────────────────────────

func uint64ToBytes(v uint64) []byte {
	b := make([]byte, 8)
	putUint64(b, v)
	return b
}

func bytesToUint64(b []byte) uint64 {
	if len(b) < 8 {
		return 0
	}
	return getUint64(b)
}

func float64ToBytes(v float64) []byte {
	bits := math.Float64bits(v)
	return uint64ToBytes(bits)
}

func putUint64(b []byte, v uint64) {
	b[0] = byte(v >> 56)
	b[1] = byte(v >> 48)
	b[2] = byte(v >> 40)
	b[3] = byte(v >> 32)
	b[4] = byte(v >> 24)
	b[5] = byte(v >> 16)
	b[6] = byte(v >> 8)
	b[7] = byte(v)
}

func getUint64(b []byte) uint64 {
	return uint64(b[0])<<56 | uint64(b[1])<<48 | uint64(b[2])<<40 | uint64(b[3])<<32 |
		uint64(b[4])<<24 | uint64(b[5])<<16 | uint64(b[6])<<8 | uint64(b[7])
}

func putUint32(b []byte, v uint32) {
	b[0] = byte(v >> 24)
	b[1] = byte(v >> 16)
	b[2] = byte(v >> 8)
	b[3] = byte(v)
}

func getUint32(b []byte) uint32 {
	return uint32(b[0])<<24 | uint32(b[1])<<16 | uint32(b[2])<<8 | uint32(b[3])
}

// ── Stats persistence ─────────────────────────────────────────────

func (e *Engine) loadStats() {
	e.db.View(func(tx *bolt.Tx) error {
		stats := tx.Bucket([]byte("_fts_stats"))
		if stats == nil {
			return nil
		}
		if dc := stats.Get([]byte("doc_count")); dc != nil {
			e.docCount = int(bytesToUint64(dc))
		}
		if adl := stats.Get([]byte("avg_doc_len")); adl != nil {
			bits := bytesToUint64(adl)
			e.avgDocLen = math.Float64frombits(bits)
		}
		return nil
	})
}
