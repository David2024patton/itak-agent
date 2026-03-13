package api

import (
	"archive/zip"
	"context"
	"crypto/md5"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/David2024patton/iTaKAgent/pkg/embed"

	"github.com/David2024patton/iTaKAgent/pkg/debug"
	"github.com/David2024patton/iTaKAgent/pkg/memory"
	"github.com/David2024patton/iTaKDatabase/pkg/itakdb"
	"github.com/David2024patton/iTaKDatabase/pkg/table"
)

// IngestAPI handles file and archive uploads into ALL database engines.
//
// What: Multi-database ZIP ingestion pipeline. Every file is written to
//       all four engines simultaneously for full redundancy.
// Why:  Lets you upload website templates, code projects, or document bundles
//       and instantly get searchable knowledge across Graph, Table, Vector,
//       and Full-Text Search.
// How:  Extracts the zip and for each file:
//       1. Graph:  Creates typed node + edges (relationships, CONTAINS)
//       2. Table:  Inserts structured metadata row in 'ingested_files'
//       3. FTS:    Indexes text content for BM25 keyword search
//       4. Vector: Stores content fingerprint for similarity search
type IngestAPI struct {
	backend memory.GraphBackend
}

// RegisterIngestRoutes adds the ingest endpoints to the server mux.
func RegisterIngestRoutes(mux *http.ServeMux, backend memory.GraphBackend) {
	if backend == nil {
		debug.Warn("api", "Graph backend is nil, ingest API disabled")
		return
	}

	ig := &IngestAPI{backend: backend}
	mux.HandleFunc("/v1/graph/ingest", ig.handleIngest)

	debug.Info("api", "Ingest API endpoint registered (POST /v1/graph/ingest)")
}

// labelForExtension returns a graph label based on file extension.
func labelForExtension(ext string) string {
	switch strings.ToLower(ext) {
	case ".html", ".htm":
		return "Page"
	case ".css", ".scss", ".sass", ".less":
		return "Stylesheet"
	case ".js", ".jsx", ".ts", ".tsx", ".mjs":
		return "Script"
	case ".json":
		return "Config"
	case ".md", ".txt", ".rst":
		return "Document"
	case ".png", ".jpg", ".jpeg", ".gif", ".svg", ".webp", ".ico":
		return "Image"
	case ".mp4", ".webm", ".mov", ".avi":
		return "Video"
	case ".mp3", ".wav", ".ogg", ".flac":
		return "Audio"
	case ".woff", ".woff2", ".ttf", ".otf", ".eot":
		return "Font"
	case ".go", ".py", ".rb", ".java", ".c", ".cpp", ".rs":
		return "SourceCode"
	case ".yaml", ".yml", ".toml", ".xml", ".env":
		return "Config"
	default:
		return "File"
	}
}

// isTextFile checks if a file is likely text-based (safe to store content inline).
func isTextFile(ext string) bool {
	textExts := map[string]bool{
		".html": true, ".htm": true, ".css": true, ".scss": true, ".sass": true, ".less": true,
		".js": true, ".jsx": true, ".ts": true, ".tsx": true, ".mjs": true,
		".json": true, ".md": true, ".txt": true, ".rst": true,
		".go": true, ".py": true, ".rb": true, ".java": true, ".c": true, ".cpp": true, ".rs": true,
		".yaml": true, ".yml": true, ".toml": true, ".xml": true, ".env": true,
		".svg": true,
	}
	return textExts[strings.ToLower(ext)]
}

// maxInlineSize is the max file size to store content inline in graph properties (64KB).
const maxInlineSize = 64 * 1024

// referencePatterns detects common cross-file references in web files.
var referencePatterns = []*regexp.Regexp{
	// HTML: <link href="...">  <script src="...">  <img src="...">  <a href="...">
	regexp.MustCompile(`(?i)(?:href|src|action)=["']([^"']+)["']`),
	// CSS: url(...)  @import "..."
	regexp.MustCompile(`(?i)url\(["']?([^"')]+)["']?\)`),
	regexp.MustCompile(`(?i)@import\s+["']([^"']+)["']`),
	// JS: import ... from "..."  require("...")
	regexp.MustCompile(`(?:from|require\()["']([^"']+)["']`),
}

// edgeTypeForReference maps file relationships to edge types.
func edgeTypeForReference(fromExt, toExt string) string {
	fromExt = strings.ToLower(fromExt)
	toExt = strings.ToLower(toExt)

	switch {
	case fromExt == ".html" && (toExt == ".css" || toExt == ".scss"):
		return "IMPORTS"
	case fromExt == ".html" && (toExt == ".js" || toExt == ".ts"):
		return "INCLUDES"
	case fromExt == ".html" && (toExt == ".png" || toExt == ".jpg" || toExt == ".svg" || toExt == ".webp" || toExt == ".gif"):
		return "REFERENCES"
	case fromExt == ".css" && (toExt == ".png" || toExt == ".jpg" || toExt == ".svg" || toExt == ".woff" || toExt == ".woff2"):
		return "REFERENCES"
	case fromExt == ".js" || fromExt == ".ts":
		return "IMPORTS"
	default:
		return "REFERENCES"
	}
}

// POST /v1/graph/ingest  (multipart/form-data with "file" field)
func (ig *IngestAPI) handleIngest(w http.ResponseWriter, r *http.Request) {
	corsHeaders(w)

	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(map[string]string{"error": "POST only"})
		return
	}

	// Parse multipart (max 100MB zip)
	if err := r.ParseMultipartForm(100 << 20); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "parse form: " + err.Error()})
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "missing 'file' field"})
		return
	}
	defer file.Close()

	templateName := r.FormValue("name")
	if templateName == "" {
		templateName = strings.TrimSuffix(header.Filename, filepath.Ext(header.Filename))
	}

	// Save zip to temp
	tmpFile, err := os.CreateTemp("", "ingest-*.zip")
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "temp file: " + err.Error()})
		return
	}
	defer os.Remove(tmpFile.Name())

	if _, err := io.Copy(tmpFile, file); err != nil {
		tmpFile.Close()
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "copy: " + err.Error()})
		return
	}
	tmpFile.Close()

	debug.Info("ingest", "Processing ZIP: %s (%d bytes)", header.Filename, header.Size)

	// Get raw DB
	itakBackend, ok := ig.backend.(*memory.ITakDBBackend)
	if !ok {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "backend does not support direct DB access"})
		return
	}
	db := itakBackend.DB()

	// ── ENGINE 2: Table -- ensure ingested_files table exists ──
	db.Table.CreateTable("ingested_files", []table.Column{
		{Name: "template", Type: table.TypeString},
		{Name: "path", Type: table.TypeString},
		{Name: "filename", Type: table.TypeString},
		{Name: "ext", Type: table.TypeString},
		{Name: "label", Type: table.TypeString},
		{Name: "size", Type: table.TypeInt},
		{Name: "graph_node_id", Type: table.TypeInt},
		{Name: "has_content", Type: table.TypeBool},
		{Name: "content_hash", Type: table.TypeString},
		{Name: "ingested_at", Type: table.TypeTime},
	})

	// ── ENGINE 1: Graph -- create Template root node ──
	templateID, err := db.CreateNode([]string{"Template"}, map[string]interface{}{
		"name":       templateName,
		"filename":   header.Filename,
		"size_bytes": header.Size,
		"uploaded":   time.Now().Format(time.RFC3339),
	}, nil)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "create template node: " + err.Error()})
		return
	}

	// Open zip and process files
	zr, err := zip.OpenReader(tmpFile.Name())
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "open zip: " + err.Error()})
		return
	}
	defer zr.Close()

	// Track created file nodes: relative path -> node ID
	fileNodes := map[string]uint64{}
	// Track file contents for relationship detection
	fileContents := map[string]string{}

	var filesProcessed, edgesCreated, tableRows, ftsIndexed, vectorStored int

	for _, zf := range zr.File {
		// Skip directories, hidden files, __MACOSX
		if zf.FileInfo().IsDir() {
			continue
		}
		name := zf.Name
		if strings.HasPrefix(name, "__MACOSX") || strings.HasPrefix(filepath.Base(name), ".") {
			continue
		}

		ext := filepath.Ext(name)
		label := labelForExtension(ext)

		// Read file content
		rc, err := zf.Open()
		if err != nil {
			debug.Warn("ingest", "Skip %s: %v", name, err)
			continue
		}

		var content string
		var contentStored bool
		var contentHash string
		isText := isTextFile(ext)

		if isText && zf.UncompressedSize64 <= maxInlineSize {
			data, err := io.ReadAll(rc)
			rc.Close()
			if err != nil {
				debug.Warn("ingest", "Skip %s: read: %v", name, err)
				continue
			}
			content = string(data)
			contentStored = true
			contentHash = fmt.Sprintf("%x", md5.Sum(data))
		} else {
			rc.Close()
		}

		// Build properties
		props := map[string]interface{}{
			"path":     name,
			"filename": filepath.Base(name),
			"ext":      ext,
			"size":     zf.UncompressedSize64,
			"template": templateName,
		}
		if contentStored {
			props["content"] = content
			props["content_hash"] = contentHash
		}

		// ── ENGINE 1: Graph -- create file node ──
		nodeID, err := db.CreateNode([]string{label}, props, nil)
		if err != nil {
			debug.Warn("ingest", "Skip %s: create node: %v", name, err)
			continue
		}

		fileNodes[name] = nodeID
		if contentStored {
			fileContents[name] = content
		}

		// Graph: link file to template
		db.Graph.CreateEdge("CONTAINS", templateID, nodeID, nil)

		// ── ENGINE 2: Table -- insert structured metadata row ──
		_, tblErr := db.Table.Insert("ingested_files", map[string]interface{}{
			"template":      templateName,
			"path":          name,
			"filename":      filepath.Base(name),
			"ext":           ext,
			"label":         label,
			"size":          zf.UncompressedSize64,
			"graph_node_id": nodeID,
			"has_content":   contentStored,
			"content_hash":  contentHash,
			"ingested_at":   time.Now().Format(time.RFC3339),
		})
		if tblErr == nil {
			tableRows++
		}

		// ── ENGINE 3: FTS/BM25 -- index text content ──
		if contentStored && content != "" {
			db.Search.IndexDocument(nodeID, content)
			ftsIndexed++
		}

		// ── ENGINE 4: Vector -- generate real embeddings or fingerprint ──
		if contentStored && content != "" {
			vec := embedContent(content)
			if vec == nil && contentHash != "" {
				vec = contentFingerprint(contentHash)
			}
			if vec != nil {
				db.Vector.Insert(nodeID, vec)
				vectorStored++
			}
		}

		filesProcessed++
	}

	// Detect cross-file references and create relationship edges
	for fromPath, content := range fileContents {
		fromID := fileNodes[fromPath]
		fromExt := filepath.Ext(fromPath)
		fromDir := filepath.Dir(fromPath)

		for _, pattern := range referencePatterns {
			matches := pattern.FindAllStringSubmatch(content, -1)
			for _, match := range matches {
				ref := match[1]
				// Skip external URLs, data URIs, anchors
				if strings.HasPrefix(ref, "http") || strings.HasPrefix(ref, "data:") ||
					strings.HasPrefix(ref, "#") || strings.HasPrefix(ref, "//") ||
					strings.HasPrefix(ref, "mailto:") {
					continue
				}
				// Resolve relative path
				resolved := filepath.ToSlash(filepath.Join(fromDir, ref))
				// Clean any query params or fragments
				if idx := strings.IndexAny(resolved, "?#"); idx != -1 {
					resolved = resolved[:idx]
				}
				// Normalize
				resolved = strings.TrimPrefix(resolved, "./")

				// Find matching file node
				if toID, ok := fileNodes[resolved]; ok && toID != fromID {
					toExt := filepath.Ext(resolved)
					edgeType := edgeTypeForReference(fromExt, toExt)
					db.Graph.CreateEdge(edgeType, fromID, toID, map[string]interface{}{
						"detected": "auto",
					})
					edgesCreated++
				}
			}
		}
	}

	debug.Info("ingest", "Ingested %q: %d files, %d edges, %d table rows, %d FTS docs, %d vectors",
		templateName, filesProcessed, edgesCreated, tableRows, ftsIndexed, vectorStored)

	result := map[string]interface{}{
		"status":      "ingested",
		"template_id": templateID,
		"template":    templateName,
		"files":       filesProcessed,
		"engines": map[string]interface{}{
			"graph": map[string]interface{}{
				"nodes":   filesProcessed + 1, // +1 for Template root
				"edges":   edgesCreated + filesProcessed, // CONTAINS + detected
			},
			"table": map[string]interface{}{
				"rows":  tableRows,
				"table": "ingested_files",
			},
			"fts": map[string]interface{}{
				"indexed": ftsIndexed,
			},
			"vector": map[string]interface{}{
				"stored": vectorStored,
			},
		},
		"file_nodes": fileNodes,
	}

	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(result)
}

// contentFingerprint generates a 32-dimensional float32 vector from an MD5 hash.
// This provides basic content similarity grouping without an embedding model.
// When a real embedding model (e.g., Gemini Embedding) is available, replace this.
func contentFingerprint(hash string) []float32 {
	vec := make([]float32, 32)
	for i := 0; i < 32 && i < len(hash); i++ {
		// Map hex chars to [-1, 1] range
		val := float64(hash[i])
		vec[i] = float32(math.Sin(val * float64(i+1)))
	}
	// Normalize
	var norm float32
	for _, v := range vec {
		norm += v * v
	}
	if norm > 0 {
		norm = float32(math.Sqrt(float64(norm)))
		for i := range vec {
			vec[i] /= norm
		}
	}
	return vec
}

// embedContent generates a real embedding vector using the active Embedder.
// Returns nil if no embedder is configured or embedding fails (caller falls
// back to contentFingerprint).
func embedContent(content string) []float32 {
	e := embed.Get()
	if e.Dimensions() == 0 {
		return nil // noop embedder, no real provider configured
	}

	// Truncate very long files to fit model context window.
	// Most embedding models support 8192 tokens (roughly 4000 chars for code).
	if len(content) > 4000 {
		content = content[:4000]
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	vec, err := e.Embed(ctx, content)
	if err != nil {
		// Silent fallback - contentFingerprint will be used instead.
		return nil
	}
	return vec
}

// ingestZipFile runs the full 4-engine pipeline on a local zip file.
// This is the shared core used by both handleIngest (HTTP upload) and
// the knowledge API's repo ingest handler (download from GitHub etc).
//
// Returns a processResult with template_id, file count, and per-engine stats.
func (ig *IngestAPI) ingestZipFile(db *itakdb.DB, zipPath, templateName string) *processResult {
	// Ensure table exists
	db.Table.CreateTable("ingested_files", []table.Column{
		{Name: "template", Type: table.TypeString},
		{Name: "path", Type: table.TypeString},
		{Name: "filename", Type: table.TypeString},
		{Name: "ext", Type: table.TypeString},
		{Name: "label", Type: table.TypeString},
		{Name: "size", Type: table.TypeInt},
		{Name: "graph_node_id", Type: table.TypeInt},
		{Name: "has_content", Type: table.TypeBool},
		{Name: "content_hash", Type: table.TypeString},
		{Name: "ingested_at", Type: table.TypeTime},
	})

	// Create Template root node
	templateID, err := db.CreateNode([]string{"Template"}, map[string]interface{}{
		"name":     templateName,
		"uploaded": time.Now().Format(time.RFC3339),
	}, nil)
	if err != nil {
		return &processResult{Status: "error", Template: templateName}
	}

	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		return &processResult{Status: "error", Template: templateName, TemplateID: templateID}
	}
	defer zr.Close()

	fileNodes := map[string]uint64{}
	fileContents := map[string]string{}
	var filesProcessed, edgesCreated, tableRows, ftsIndexed, vectorStored int

	for _, zf := range zr.File {
		if zf.FileInfo().IsDir() {
			continue
		}
		name := zf.Name
		if strings.HasPrefix(name, "__MACOSX") || strings.HasPrefix(filepath.Base(name), ".") {
			continue
		}

		// Strip top-level directory (GitHub zips have repo-branch/ prefix)
		if idx := strings.Index(name, "/"); idx > 0 {
			name = name[idx+1:]
			if name == "" {
				continue
			}
		}

		ext := filepath.Ext(name)
		label := labelForExtension(ext)

		rc, err := zf.Open()
		if err != nil {
			continue
		}

		var content string
		var contentStored bool
		var contentHash string
		isText := isTextFile(ext)

		if isText && zf.UncompressedSize64 <= maxInlineSize {
			data, err := io.ReadAll(rc)
			rc.Close()
			if err != nil {
				continue
			}
			content = string(data)
			contentStored = true
			contentHash = fmt.Sprintf("%x", md5.Sum(data))
		} else {
			rc.Close()
		}

		props := map[string]interface{}{
			"path":     name,
			"filename": filepath.Base(name),
			"ext":      ext,
			"size":     zf.UncompressedSize64,
			"template": templateName,
		}
		if contentStored {
			props["content"] = content
			props["content_hash"] = contentHash
		}

		// ENGINE 1: Graph
		nodeID, err := db.CreateNode([]string{label}, props, nil)
		if err != nil {
			continue
		}
		fileNodes[name] = nodeID
		if contentStored {
			fileContents[name] = content
		}
		db.Graph.CreateEdge("CONTAINS", templateID, nodeID, nil)

		// ENGINE 2: Table
		_, tblErr := db.Table.Insert("ingested_files", map[string]interface{}{
			"template":      templateName,
			"path":          name,
			"filename":      filepath.Base(name),
			"ext":           ext,
			"label":         label,
			"size":          zf.UncompressedSize64,
			"graph_node_id": nodeID,
			"has_content":   contentStored,
			"content_hash":  contentHash,
			"ingested_at":   time.Now().Format(time.RFC3339),
		})
		if tblErr == nil {
			tableRows++
		}

		// ENGINE 3: FTS
		if contentStored && content != "" {
			db.Search.IndexDocument(nodeID, content)
			ftsIndexed++
		}

		// ENGINE 4: Vector
		if contentStored && content != "" {
			vec := embedContent(content)
			if vec == nil && contentHash != "" {
				vec = contentFingerprint(contentHash)
			}
			if vec != nil {
				db.Vector.Insert(nodeID, vec)
				vectorStored++
			}
		}

		filesProcessed++
	}

	// Detect cross-file references
	for fromPath, content := range fileContents {
		fromID := fileNodes[fromPath]
		fromExt := filepath.Ext(fromPath)
		edgesCreated += detectAndCreateEdges(db, fromID, fromExt, content, fileNodes)
	}

	return &processResult{
		Status:     "ingested",
		TemplateID: templateID,
		Template:   templateName,
		Files:      filesProcessed,
		Engines: map[string]interface{}{
			"graph": map[string]interface{}{
				"nodes": filesProcessed + 1,
				"edges": edgesCreated + filesProcessed,
			},
			"table": map[string]interface{}{
				"rows":  tableRows,
				"table": "ingested_files",
			},
			"fts": map[string]interface{}{
				"indexed": ftsIndexed,
			},
			"vector": map[string]interface{}{
				"stored": vectorStored,
			},
		},
		FileNodes: fileNodes,
	}
}

// detectAndCreateEdges finds cross-file references in content and creates graph edges.
func detectAndCreateEdges(db *itakdb.DB, fromID uint64, fromExt, content string, fileNodes map[string]uint64) int {
	edgesCreated := 0
	fromDir := filepath.Dir(".")

	for _, pat := range referencePatterns {
		for _, m := range pat.FindAllStringSubmatch(content, -1) {
			ref := m[1]
			if strings.HasPrefix(ref, "http") || strings.HasPrefix(ref, "data:") || strings.HasPrefix(ref, "#") || strings.HasPrefix(ref, "//") {
				continue
			}
			resolved := filepath.ToSlash(filepath.Join(fromDir, ref))
			if idx := strings.IndexAny(resolved, "?#"); idx != -1 {
				resolved = resolved[:idx]
			}
			resolved = strings.TrimPrefix(resolved, "./")

			if toID, ok := fileNodes[resolved]; ok && toID != fromID {
				toExt := filepath.Ext(resolved)
				edgeType := edgeTypeForReference(fromExt, toExt)
				db.Graph.CreateEdge(edgeType, fromID, toID, map[string]interface{}{
					"detected": "auto",
				})
				edgesCreated++
			}
		}
	}
	return edgesCreated
}

// IngestResult is returned from the ingest endpoint for display.
type IngestResult struct {
	Status     string            `json:"status"`
	TemplateID uint64            `json:"template_id"`
	Template   string            `json:"template"`
	Files      int               `json:"files"`
	Engines    map[string]interface{} `json:"engines"`
	FileNodes  map[string]uint64 `json:"file_nodes"`
}

// IngestSummary builds a human-readable summary for the chat response.
func IngestSummary(r IngestResult) string {
	return fmt.Sprintf("Ingested **%s**: %d files written to all 4 databases. Template node #%d. View in Graph Explorer.",
		r.Template, r.Files, r.TemplateID)
}
