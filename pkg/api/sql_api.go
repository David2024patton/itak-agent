package api

import (
	"encoding/json"
	"io"
	"net/http"

	isql "github.com/David2024patton/iTaKDatabase/pkg/sql"

	"github.com/David2024patton/iTaKAgent/pkg/debug"
	"github.com/David2024patton/iTaKAgent/pkg/memory"
)

// RegisterSQLRoutes adds the SQL execution endpoint.
//
// What: REST API for running SQL queries against iTaKDB tables.
// Why:  Agents can query structured data with familiar SQL syntax
//       instead of building Go structs.
// How:  Creates a SQL executor bound to the shared table engine,
//       accepts POST requests with a query string, returns JSON.
func RegisterSQLRoutes(mux *http.ServeMux, backend memory.GraphBackend) {
	if backend == nil {
		return
	}

	itakBackend, ok := backend.(*memory.ITakDBBackend)
	if !ok {
		debug.Warn("api", "SQL API requires iTaK Database backend")
		return
	}

	// Build the SQL executor directly from the table engine.
	// The table engine is the established field on itakdb.DB.
	executor := isql.NewExecutor(itakBackend.DB().Table)

	mux.HandleFunc("/v1/sql", func(w http.ResponseWriter, r *http.Request) {
		corsHeaders(w)
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}
		if r.Method != http.MethodPost {
			http.Error(w, "POST only", http.StatusMethodNotAllowed)
			return
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "failed to read body"})
			return
		}

		var req struct {
			Query string `json:"query"`
		}
		if err := json.Unmarshal(body, &req); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "invalid JSON, expected {\"query\": \"SQL string\"}"})
			return
		}

		if req.Query == "" {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "query is required"})
			return
		}

		result, err := executor.Execute(req.Query)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"error": err.Error(),
				"query": req.Query,
			})
			return
		}

		json.NewEncoder(w).Encode(map[string]interface{}{
			"ok":     true,
			"query":  req.Query,
			"result": result,
		})
	})

	debug.Info("api", "SQL API registered (POST /v1/sql)")
}
