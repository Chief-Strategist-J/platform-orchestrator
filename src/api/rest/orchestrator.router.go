/*
Package rest implements the HTTP router configuration matching OpenAPI 3.1 contract paths.

ALGORITHM BLUEPRINT:
1. RouterRegistration: Binds HTTP method and path patterns to handler methods.
2. Path Dispatching:
   - POST /api/v1/stack/up -> HandleStackUp
   - POST /api/v1/stack/down -> HandleStackDown
   - GET  /api/v1/stack/status -> HandleStackStatus
   - POST /api/v1/scale/service -> HandleScaleService
   - POST /api/v1/scale/node -> HandleScaleNode
   - DELETE /api/v1/scale/node/{nodeId} -> HandleTerminateNode
   - GET  /api/v1/health -> HandleHealth
   - POST /api/v1/certs/generate -> HandleGenerateCerts
   - POST /api/v1/backup/execute -> HandleBackupExecute
   - POST /api/v1/setup/bootstrap -> HandleSetupBootstrap
   - GET  /api/v1/prereqs/audit -> HandlePrereqsAudit
   - POST /api/v1/cloudflare/token -> HandleCloudflareToken
   - POST /api/v1/cloudflare/start -> HandleCloudflareStart
   - POST /api/v1/cloudflare/stop -> HandleCloudflareStop
   - GET  /api/v1/cloudflare/status -> HandleCloudflareStatus
   - POST /api/v1/gdpr/erasure -> HandleGdprErasure
   - GET  /api/v1/ports/status -> HandlePortsStatus
   - POST /api/v1/ports/free -> HandlePortsFree
3. Invariants:
   - All routes are scoped under /api/v1 prefix.
   - Non-matching methods return 405 Method Not Allowed.
   - Zero inline comments inside function bodies.
*/
package rest

import (
	"net/http"
	"strings"
)

func NewRouter(handler *OrchestratorHandler) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("/api/v1/stack/up", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			handler.HandleStackUp(w, r)
			return
		}
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
	})

	mux.HandleFunc("/api/v1/stack/down", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			handler.HandleStackDown(w, r)
			return
		}
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
	})

	mux.HandleFunc("/api/v1/stack/status", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			handler.HandleStackStatus(w, r)
			return
		}
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
	})

	mux.HandleFunc("/api/v1/scale/service", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			handler.HandleScaleService(w, r)
			return
		}
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
	})

	mux.HandleFunc("/api/v1/scale/node", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			handler.HandleScaleNode(w, r)
			return
		}
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
	})

	mux.HandleFunc("/api/v1/scale/node/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			handler.HandleTerminateNode(w, r)
			return
		}
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
	})

	mux.HandleFunc("/api/v1/health", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			handler.HandleHealth(w, r)
			return
		}
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
	})

	mux.HandleFunc("/api/v1/certs/generate", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			handler.HandleGenerateCerts(w, r)
			return
		}
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
	})

	mux.HandleFunc("/api/v1/backup/execute", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			handler.HandleBackupExecute(w, r)
			return
		}
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
	})

	mux.HandleFunc("/api/v1/setup/bootstrap", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			handler.HandleSetupBootstrap(w, r)
			return
		}
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
	})

	mux.HandleFunc("/api/v1/prereqs/audit", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			handler.HandlePrereqsAudit(w, r)
			return
		}
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
	})

	mux.HandleFunc("/api/v1/cloudflare/token", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			handler.HandleCloudflareToken(w, r)
			return
		}
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
	})

	mux.HandleFunc("/api/v1/cloudflare/start", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			handler.HandleCloudflareStart(w, r)
			return
		}
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
	})

	mux.HandleFunc("/api/v1/cloudflare/stop", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			handler.HandleCloudflareStop(w, r)
			return
		}
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
	})

	mux.HandleFunc("/api/v1/cloudflare/status", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			handler.HandleCloudflareStatus(w, r)
			return
		}
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
	})

	mux.HandleFunc("/api/v1/gdpr/erasure", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			handler.HandleGdprErasure(w, r)
			return
		}
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
	})

	mux.HandleFunc("/api/v1/ports/status", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			handler.HandlePortsStatus(w, r)
			return
		}
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
	})

	mux.HandleFunc("/api/v1/ports/free", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			handler.HandlePortsFree(w, r)
			return
		}
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
	})

	mux.HandleFunc("/api/v1/config", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			handler.HandleGetConfig(w, r)
			return
		}
		if r.Method == http.MethodPost {
			handler.HandleUpdateConfig(w, r)
			return
		}
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
	})

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, traceparent")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}
		if !strings.HasPrefix(r.URL.Path, "/api/v1") {
			http.NotFound(w, r)
			return
		}
		mux.ServeHTTP(w, r)
	})
}
