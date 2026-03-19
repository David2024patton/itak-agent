// Package codex provides code knowledge graph indexing for Go projects.
//
// What: Parses Go source code using go/ast and indexes the structure into itakdb.
// Why:  Gives agents deep awareness of codebase architecture (functions, types,
//       imports, call chains) so they can make informed edits without breaking things.
// How:  4-phase pipeline: Structure -> Parse -> Resolution -> Search.
//       Uses pure Go stdlib (go/parser, go/ast) with zero external dependencies.
package codex

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/David2024patton/iTaKDatabase/pkg/itakdb"
)

// IndexStats tracks what was indexed during a run.
type IndexStats struct {
	Directories int `json:"directories"`
	Files       int `json:"files"`
	Functions   int `json:"functions"`
	Types       int `json:"types"`
	Interfaces  int `json:"interfaces"`
	Imports     int `json:"imports"`
	CallEdges   int `json:"call_edges"`
	Duration    string `json:"duration"`
}

// Indexer parses Go source code and stores the structure in itakdb.
type Indexer struct {
	db      *itakdb.DB
	rootDir string
	fset    *token.FileSet

	// Caches for resolution phase.
	funcNodes map[string]uint64 // "pkg.FuncName" -> node ID
	fileNodes map[string]uint64 // filepath -> node ID
	dirNodes  map[string]uint64 // dirpath -> node ID
}

// NewIndexer creates a code indexer connected to the given database.
func NewIndexer(db *itakdb.DB) *Indexer {
	return &Indexer{
		db:        db,
		fset:      token.NewFileSet(),
		funcNodes: make(map[string]uint64),
		fileNodes: make(map[string]uint64),
		dirNodes:  make(map[string]uint64),
	}
}

// Index runs the full 4-phase pipeline on the given directory.
func (idx *Indexer) Index(rootDir string) (*IndexStats, error) {
	start := time.Now()
	idx.rootDir = rootDir

	stats := &IndexStats{}

	// Phase 1: Structure - walk directories and files.
	log.Printf("[codex] Phase 1: Indexing directory structure for %s", rootDir)
	if err := idx.indexStructure(rootDir, stats); err != nil {
		return stats, fmt.Errorf("structure phase: %w", err)
	}

	// Phase 2: Parse - extract functions, types, interfaces from Go AST.
	log.Printf("[codex] Phase 2: Parsing Go source files (%d files)", stats.Files)
	if err := idx.parseGoFiles(rootDir, stats); err != nil {
		return stats, fmt.Errorf("parse phase: %w", err)
	}

	// Phase 3: Resolution - resolve imports and function calls.
	log.Printf("[codex] Phase 3: Resolving imports and call chains")
	if err := idx.resolveRelationships(rootDir, stats); err != nil {
		return stats, fmt.Errorf("resolution phase: %w", err)
	}

	// Phase 4: Search - index symbols into FTS.
	log.Printf("[codex] Phase 4: Indexing symbols for search")
	idx.indexForSearch(stats)

	stats.Duration = time.Since(start).String()
	log.Printf("[codex] Done: %d dirs, %d files, %d funcs, %d types, %d imports, %d call edges in %s",
		stats.Directories, stats.Files, stats.Functions, stats.Types,
		stats.Imports, stats.CallEdges, stats.Duration)

	return stats, nil
}

// ── Phase 1: Structure ──────────────────────────────────────────

func (idx *Indexer) indexStructure(rootDir string, stats *IndexStats) error {
	return filepath.Walk(rootDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // skip unreadable paths
		}

		// Skip hidden dirs, vendor, .git, node_modules.
		name := info.Name()
		if info.IsDir() && (strings.HasPrefix(name, ".") || name == "vendor" || name == "node_modules" || name == "testdata") {
			return filepath.SkipDir
		}

		relPath, _ := filepath.Rel(rootDir, path)
		if relPath == "." {
			relPath = filepath.Base(rootDir)
		}

		if info.IsDir() {
			// Create directory node.
			dirID, _, _ := idx.db.MergeNode("CodeDir", "path", relPath, map[string]interface{}{
				"name":       name,
				"indexed_at": time.Now().Format(time.RFC3339),
			}, nil)
			idx.dirNodes[relPath] = dirID
			stats.Directories++

			// Link to parent directory.
			parentRel, _ := filepath.Rel(rootDir, filepath.Dir(path))
			if parentRel == "." {
				parentRel = filepath.Base(rootDir)
			}
			if parentID, ok := idx.dirNodes[parentRel]; ok && parentID != dirID {
				idx.db.Graph.CreateEdge("CONTAINS", parentID, dirID, nil)
			}
			return nil
		}

		// Only index Go files.
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			return nil
		}

		// Create file node.
		fileID, _, _ := idx.db.MergeNode("CodeFile", "path", relPath, map[string]interface{}{
			"name":       name,
			"language":   "go",
			"size":       info.Size(),
			"indexed_at": time.Now().Format(time.RFC3339),
		}, nil)
		idx.fileNodes[relPath] = fileID
		stats.Files++

		// Link file to parent directory.
		parentRel, _ := filepath.Rel(rootDir, filepath.Dir(path))
		if parentRel == "." {
			parentRel = filepath.Base(rootDir)
		}
		if parentID, ok := idx.dirNodes[parentRel]; ok {
			idx.db.Graph.CreateEdge("CONTAINS", parentID, fileID, nil)
		}

		return nil
	})
}

// ── Phase 2: Parse ──────────────────────────────────────────────

func (idx *Indexer) parseGoFiles(rootDir string, stats *IndexStats) error {
	return filepath.Walk(rootDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}

		name := info.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			return nil
		}

		// Skip hidden/vendored.
		if strings.Contains(path, string(filepath.Separator)+".") ||
			strings.Contains(path, "vendor"+string(filepath.Separator)) {
			return nil
		}

		relPath, _ := filepath.Rel(rootDir, path)
		fileID, ok := idx.fileNodes[relPath]
		if !ok {
			return nil
		}

		// Parse the Go file.
		f, parseErr := parser.ParseFile(idx.fset, path, nil, parser.ParseComments)
		if parseErr != nil {
			// Still try to index what we can.
			log.Printf("[codex] Warning: parse error in %s: %v", relPath, parseErr)
			return nil
		}

		// Store package name on the file node.
		if f.Name != nil {
			idx.db.Graph.UpdateNode(fileID, map[string]interface{}{
				"package": f.Name.Name,
			})
		}

		// Walk declarations.
		for _, decl := range f.Decls {
			switch d := decl.(type) {
			case *ast.FuncDecl:
				idx.indexFunction(d, f, relPath, fileID, stats)
			case *ast.GenDecl:
				idx.indexGenDecl(d, f, relPath, fileID, stats)
			}
		}

		return nil
	})
}

// indexFunction creates a CodeFunc node for a function or method declaration.
func (idx *Indexer) indexFunction(fn *ast.FuncDecl, file *ast.File, relPath string, fileID uint64, stats *IndexStats) {
	funcName := fn.Name.Name
	exported := ast.IsExported(funcName)
	pos := idx.fset.Position(fn.Pos())

	// Build a qualified name: "pkg.FuncName" or "pkg.TypeName.MethodName"
	pkgName := ""
	if file.Name != nil {
		pkgName = file.Name.Name
	}

	qualifiedName := pkgName + "." + funcName
	receiverType := ""

	// Check if it's a method (has a receiver).
	if fn.Recv != nil && len(fn.Recv.List) > 0 {
		recv := fn.Recv.List[0]
		receiverType = exprToString(recv.Type)
		qualifiedName = pkgName + "." + receiverType + "." + funcName
	}

	// Build signature.
	sig := buildSignature(fn)

	// Get doc comment.
	doc := ""
	if fn.Doc != nil {
		doc = fn.Doc.Text()
		if len(doc) > 500 {
			doc = doc[:500]
		}
	}

	props := map[string]interface{}{
		"name":      funcName,
		"package":   pkgName,
		"exported":  exported,
		"line":      pos.Line,
		"signature": sig,
		"file_path": relPath,
	}
	if doc != "" {
		props["doc"] = doc
	}
	if receiverType != "" {
		props["receiver"] = receiverType
		props["kind"] = "method"
	} else {
		props["kind"] = "function"
	}

	funcID, _, _ := idx.db.MergeNode("CodeFunc", "qualified_name", qualifiedName, props, nil)
	idx.funcNodes[qualifiedName] = funcID
	stats.Functions++

	// DEFINED_IN edge to file.
	idx.db.Graph.CreateEdge("DEFINED_IN", funcID, fileID, nil)

	// HAS_METHOD edge if it's a method with a receiver.
	if receiverType != "" {
		typeQualified := pkgName + "." + strings.TrimPrefix(strings.TrimPrefix(receiverType, "*"), "")
		typeQualified = strings.TrimSpace(typeQualified)
		// We'll link this in the resolution phase when types are known.
	}
}

// indexGenDecl handles type, const, and var declarations.
func (idx *Indexer) indexGenDecl(gd *ast.GenDecl, file *ast.File, relPath string, fileID uint64, stats *IndexStats) {
	if gd.Tok != token.TYPE {
		return // only index type declarations for now
	}

	pkgName := ""
	if file.Name != nil {
		pkgName = file.Name.Name
	}

	for _, spec := range gd.Specs {
		ts, ok := spec.(*ast.TypeSpec)
		if !ok {
			continue
		}

		typeName := ts.Name.Name
		exported := ast.IsExported(typeName)
		pos := idx.fset.Position(ts.Pos())
		qualifiedName := pkgName + "." + typeName

		kind := "struct"
		var fields []string

		switch t := ts.Type.(type) {
		case *ast.StructType:
			kind = "struct"
			if t.Fields != nil {
				for _, field := range t.Fields.List {
					for _, name := range field.Names {
						fields = append(fields, name.Name)
					}
				}
			}
		case *ast.InterfaceType:
			kind = "interface"
			stats.Interfaces++
			if t.Methods != nil {
				for _, method := range t.Methods.List {
					for _, name := range method.Names {
						fields = append(fields, name.Name)
					}
				}
			}
		default:
			kind = "alias"
		}

		doc := ""
		if gd.Doc != nil {
			doc = gd.Doc.Text()
			if len(doc) > 500 {
				doc = doc[:500]
			}
		}

		props := map[string]interface{}{
			"name":     typeName,
			"package":  pkgName,
			"kind":     kind,
			"exported": exported,
			"line":     pos.Line,
			"file_path": relPath,
		}
		if doc != "" {
			props["doc"] = doc
		}
		if len(fields) > 0 {
			props["fields"] = strings.Join(fields, ", ")
		}

		typeID, _, _ := idx.db.MergeNode("CodeType", "qualified_name", qualifiedName, props, nil)
		stats.Types++

		// DEFINED_IN edge.
		idx.db.Graph.CreateEdge("DEFINED_IN", typeID, fileID, nil)
	}
}

// ── Phase 3: Resolution ─────────────────────────────────────────

func (idx *Indexer) resolveRelationships(rootDir string, stats *IndexStats) error {
	return filepath.Walk(rootDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}

		name := info.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			return nil
		}
		if strings.Contains(path, string(filepath.Separator)+".") ||
			strings.Contains(path, "vendor"+string(filepath.Separator)) {
			return nil
		}

		relPath, _ := filepath.Rel(rootDir, path)
		fileID, ok := idx.fileNodes[relPath]
		if !ok {
			return nil
		}

		f, parseErr := parser.ParseFile(idx.fset, path, nil, 0)
		if parseErr != nil {
			return nil
		}

		// Index imports.
		for _, imp := range f.Imports {
			importPath := strings.Trim(imp.Path.Value, `"`)

			alias := ""
			if imp.Name != nil {
				alias = imp.Name.Name
			}

			importID, _, _ := idx.db.MergeNode("CodeImport", "import_path", importPath, map[string]interface{}{
				"path":  importPath,
				"alias": alias,
			}, nil)

			idx.db.Graph.CreateEdge("IMPORTS", fileID, importID, nil)
			stats.Imports++
		}

		// Resolve function calls within the file.
		pkgName := ""
		if f.Name != nil {
			pkgName = f.Name.Name
		}

		for _, decl := range f.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}

			callerName := pkgName + "." + fn.Name.Name
			if fn.Recv != nil && len(fn.Recv.List) > 0 {
				recv := fn.Recv.List[0]
				callerName = pkgName + "." + exprToString(recv.Type) + "." + fn.Name.Name
			}

			callerID, callerExists := idx.funcNodes[callerName]
			if !callerExists {
				continue
			}

			// Walk the function body to find call expressions.
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}

				calleeName := callExprToName(call, pkgName)
				if calleeName == "" {
					return true
				}

				if calleeID, found := idx.funcNodes[calleeName]; found && calleeID != callerID {
					idx.db.Graph.CreateEdge("CALLS", callerID, calleeID, nil)
					stats.CallEdges++
				}

				return true
			})
		}

		// Link methods to their types.
		for _, decl := range f.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Recv == nil || len(fn.Recv.List) == 0 {
				continue
			}

			recv := fn.Recv.List[0]
			recvType := exprToString(recv.Type)
			recvType = strings.TrimPrefix(recvType, "*")

			typeQualified := pkgName + "." + recvType
			methodQualified := pkgName + "." + recvType + "." + fn.Name.Name

			// Find the type and method nodes and link them.
			typeNodes, _ := idx.db.Graph.FindByProperty("qualified_name", typeQualified)
			if len(typeNodes) > 0 {
				if methodID, found := idx.funcNodes[methodQualified]; found {
					idx.db.Graph.CreateEdge("HAS_METHOD", typeNodes[0].ID, methodID, nil)
				}
			}
		}

		return nil
	})
}

// ── Phase 4: Search ─────────────────────────────────────────────

func (idx *Indexer) indexForSearch(stats *IndexStats) {
	// Index functions into FTS.
	funcNodes, _ := idx.db.Graph.FindByLabel("CodeFunc")
	for _, node := range funcNodes {
		name, _ := node.Properties["name"].(string)
		pkg, _ := node.Properties["package"].(string)
		doc, _ := node.Properties["doc"].(string)
		sig, _ := node.Properties["signature"].(string)

		content := fmt.Sprintf("%s.%s %s %s", pkg, name, sig, doc)
		idx.db.Search.IndexDocument(node.ID, content)
	}

	// Index types into FTS.
	typeNodes, _ := idx.db.Graph.FindByLabel("CodeType")
	for _, node := range typeNodes {
		name, _ := node.Properties["name"].(string)
		pkg, _ := node.Properties["package"].(string)
		kind, _ := node.Properties["kind"].(string)
		fields, _ := node.Properties["fields"].(string)

		content := fmt.Sprintf("%s.%s %s %s", pkg, name, kind, fields)
		idx.db.Search.IndexDocument(node.ID, content)
	}
}

// ── Helpers ─────────────────────────────────────────────────────

// exprToString converts an AST expression to a string (for receiver types).
func exprToString(expr ast.Expr) string {
	switch e := expr.(type) {
	case *ast.Ident:
		return e.Name
	case *ast.StarExpr:
		return "*" + exprToString(e.X)
	case *ast.SelectorExpr:
		return exprToString(e.X) + "." + e.Sel.Name
	case *ast.IndexExpr:
		return exprToString(e.X) // generic type, strip type param
	default:
		return ""
	}
}

// buildSignature creates a readable function signature.
func buildSignature(fn *ast.FuncDecl) string {
	var parts []string

	if fn.Type.Params != nil {
		for _, p := range fn.Type.Params.List {
			typeStr := exprToString(p.Type)
			for _, name := range p.Names {
				parts = append(parts, name.Name+" "+typeStr)
			}
			if len(p.Names) == 0 {
				parts = append(parts, typeStr)
			}
		}
	}

	sig := "(" + strings.Join(parts, ", ") + ")"

	if fn.Type.Results != nil && len(fn.Type.Results.List) > 0 {
		var rets []string
		for _, r := range fn.Type.Results.List {
			rets = append(rets, exprToString(r.Type))
		}
		if len(rets) == 1 {
			sig += " " + rets[0]
		} else {
			sig += " (" + strings.Join(rets, ", ") + ")"
		}
	}

	return sig
}

// callExprToName extracts the function name from a call expression.
func callExprToName(call *ast.CallExpr, currentPkg string) string {
	switch fn := call.Fun.(type) {
	case *ast.Ident:
		// Simple call: funcName()
		return currentPkg + "." + fn.Name
	case *ast.SelectorExpr:
		// Method or package call: obj.Method() or pkg.Func()
		if ident, ok := fn.X.(*ast.Ident); ok {
			return currentPkg + "." + ident.Name + "." + fn.Sel.Name
		}
	}
	return ""
}

// QueryResult holds code query results.
type QueryResult struct {
	Type       string                 `json:"type"`
	Name       string                 `json:"name"`
	Properties map[string]interface{} `json:"properties"`
	Related    []string               `json:"related,omitempty"`
}

// Query searches the code graph by keyword or symbol name.
func (idx *Indexer) Query(query string) ([]QueryResult, error) {
	var results []QueryResult
	seen := make(map[uint64]bool)

	// Search FTS first. Results come back as search.Result{DocID, Score}.
	// The DocID is the graph node ID we stored during indexing.
	ftsResults, _ := idx.db.Search.Search(query, 20)
	for _, result := range ftsResults {
		nodeID := result.DocID
		if seen[nodeID] {
			continue
		}
		seen[nodeID] = true

		node, _ := idx.db.Graph.GetNode(nodeID)
		if node == nil {
			continue
		}

		// Determine type from labels.
		nodeType := "unknown"
		for _, l := range node.Labels {
			switch l {
			case "CodeFunc":
				nodeType = "function"
			case "CodeType":
				nodeType = "type"
			case "CodeFile":
				nodeType = "file"
			case "CodeDir":
				nodeType = "directory"
			case "CodeImport":
				nodeType = "import"
			default:
				nodeType = l
			}
		}

		results = append(results, QueryResult{
			Type:       nodeType,
			Name:       propStr(node.Properties, "qualified_name"),
			Properties: node.Properties,
		})
	}

	// Also search by label + property if no FTS results.
	if len(results) == 0 {
		nodes, _ := idx.db.Graph.FindByProperty("name", query)
		for _, node := range nodes {
			if seen[node.ID] {
				continue
			}
			seen[node.ID] = true

			label := "unknown"
			if len(node.Labels) > 0 {
				label = node.Labels[0]
			}
			results = append(results, QueryResult{
				Type:       label,
				Name:       propStr(node.Properties, "name"),
				Properties: node.Properties,
			})
		}
	}

	return results, nil
}

func propStr(props map[string]interface{}, key string) string {
	v, _ := props[key].(string)
	return v
}
