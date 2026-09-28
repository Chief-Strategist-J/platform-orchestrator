/*
Package rest implements the HTTP router configuration matching OpenAPI 3.1 contract paths.

ALGORITHM BLUEPRINT:
1. RouterRegistration: Binds HTTP method and path patterns to handler methods.
2. Path Dispatching:
   - Core Stack: /api/v1/stack/up, /api/v1/stack/down, /api/v1/stack/status
   - Scaling: /api/v1/scale/service, /api/v1/scale/node, /api/v1/scale/node/{nodeId}
   - Diagnostics: /api/v1/health/deep, /api/v1/certs/generate, /api/v1/prereqs/audit, /api/v1/ports/*
   - Cloudflare & GDPR: /api/v1/cloudflare/*, /api/v1/gdpr/erasure
   - Configuration & Bootstrap: /api/v1/setup/bootstrap, /api/v1/config
   - Grafana Datasources: /api/v1/grafana/datasources/*
   - Grafana Dashboards: /api/v1/grafana/dashboards/*
   - Grafana Alerting: /api/v1/grafana/alerts/*, /api/v1/grafana/contact-points/*
   - External Services Catalog: /api/v1/services/*
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

	mux.HandleFunc("/api/v1/health/deep", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			handler.HandleDeepHealth(w, r)
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
		switch r.Method {
		case http.MethodGet:
			handler.HandleGetConfig(w, r)
		case http.MethodPatch, http.MethodPost:
			handler.HandleUpdateConfig(w, r)
		default:
			http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/api/v1/grafana/datasources", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			handler.HandleListDatasources(w, r)
			return
		}
		if r.Method == http.MethodPost {
			handler.HandleCreateDatasource(w, r)
			return
		}
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
	})

	mux.HandleFunc("/api/v1/grafana/datasources/sync", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			handler.HandleSyncDatasources(w, r)
			return
		}
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
	})

	mux.HandleFunc("/api/v1/grafana/datasources/", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/health") {
			if r.Method == http.MethodGet {
				handler.HandleTestDatasourceHealth(w, r)
				return
			}
			http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
			return
		}
		switch r.Method {
		case http.MethodGet:
			handler.HandleGetDatasource(w, r)
		case http.MethodPut:
			handler.HandleUpdateDatasource(w, r)
		case http.MethodDelete:
			handler.HandleDeleteDatasource(w, r)
		default:
			http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/api/v1/grafana/dashboards", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			handler.HandleSearchDashboards(w, r)
			return
		}
		if r.Method == http.MethodPost {
			handler.HandleSaveDashboard(w, r)
			return
		}
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
	})

	mux.HandleFunc("/api/v1/grafana/dashboards/import", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			handler.HandleImportDashboard(w, r)
			return
		}
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
	})

	mux.HandleFunc("/api/v1/grafana/dashboards/", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			handler.HandleGetDashboard(w, r)
		case http.MethodDelete:
			handler.HandleDeleteDashboard(w, r)
		default:
			http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/api/v1/grafana/alerts", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			handler.HandleListAlertRules(w, r)
			return
		}
		if r.Method == http.MethodPost {
			handler.HandleSaveAlertRule(w, r)
			return
		}
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
	})

	mux.HandleFunc("/api/v1/grafana/alerts/", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			handler.HandleGetAlertRule(w, r)
		case http.MethodDelete:
			handler.HandleDeleteAlertRule(w, r)
		default:
			http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/api/v1/grafana/contact-points", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			handler.HandleListContactPoints(w, r)
			return
		}
		if r.Method == http.MethodPost {
			handler.HandleSaveContactPoint(w, r)
			return
		}
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
	})

	mux.HandleFunc("/api/v1/grafana/contact-points/test", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			handler.HandleTestContactPoint(w, r)
			return
		}
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
	})

	mux.HandleFunc("/api/v1/grafana/contact-points/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			handler.HandleDeleteContactPoint(w, r)
			return
		}
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
	})

	mux.HandleFunc("/api/v1/services", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			handler.HandleListServices(w, r)
			return
		}
		if r.Method == http.MethodPost {
			handler.HandleRegisterService(w, r)
			return
		}
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
	})

	mux.HandleFunc("/api/v1/services/", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/health") {
			if r.Method == http.MethodGet || r.Method == http.MethodPost {
				handler.HandleTestServiceHealth(w, r)
				return
			}
			http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
			return
		}
		if strings.HasSuffix(r.URL.Path, "/sync-to-grafana") {
			if r.Method == http.MethodPost {
				handler.HandleSyncServiceToGrafana(w, r)
				return
			}
			http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
			return
		}
		switch r.Method {
		case http.MethodGet:
			handler.HandleGetService(w, r)
		case http.MethodPut:
			handler.HandleUpdateService(w, r)
		case http.MethodDelete:
			handler.HandleDeleteService(w, r)
		default:
			http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		}
	})

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, PATCH, OPTIONS")
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
