/*
Package rest implements the HTTP presentation layer and REST controller handlers.

ALGORITHM BLUEPRINT:
1. Envelope Wrapping: All successful responses are formatted via types.NewSuccessResponse(data, "v1").
2. Error Handling: All failures map to HTTP status codes with structured types.NewErrorResponse(code, msg, target, "v1").
3. Input Decoding: Request JSON bodies are decoded into feature commands without business branching.
4. Route Coverage:
   - HandleStackUp, HandleStackDown, HandleStackStatus
   - HandleScaleService, HandleScaleNode, HandleTerminateNode
   - HandleDeepHealth: deep functional probe per service (POST /api/v1/health/deep)
   - HandleBackupExecute: disaster recovery backup and volume purge
   - HandleSetupBootstrap: automated 7-step bootstrapping
   - HandlePrereqsAudit: host and kernel prerequisite auditing
   - HandleCloudflareToken, HandleCloudflareStart, HandleCloudflareStop, HandleCloudflareStatus
   - HandleGdprErasure: GDPR/CCPA right-to-erasure across databases
   - HandlePortsStatus, HandlePortsFree: host port contention diagnosis and freeing
5. Invariants:
   - Handlers must not contain domain business logic or direct persistence calls.
   - Trace context is derived and injected into response envelopes.
   - Zero inline comments inside function bodies.
*/
package rest

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	backupSchema "github.com/Chief-Strategist-J/platform-orchestrator/src/features/backup/schema"
	backupService "github.com/Chief-Strategist-J/platform-orchestrator/src/features/backup/services"
	certsSchema "github.com/Chief-Strategist-J/platform-orchestrator/src/features/certs/schema"
	certsService "github.com/Chief-Strategist-J/platform-orchestrator/src/features/certs/services"
	cloudflareSchema "github.com/Chief-Strategist-J/platform-orchestrator/src/features/cloudflare/schema"
	cloudflareService "github.com/Chief-Strategist-J/platform-orchestrator/src/features/cloudflare/services"
	configSchema "github.com/Chief-Strategist-J/platform-orchestrator/src/features/config/schema"
	configService "github.com/Chief-Strategist-J/platform-orchestrator/src/features/config/services"
	dnsService "github.com/Chief-Strategist-J/platform-orchestrator/src/features/dns/services"
	dnsTypes "github.com/Chief-Strategist-J/platform-orchestrator/src/features/dns/types"
	gdprSchema "github.com/Chief-Strategist-J/platform-orchestrator/src/features/gdpr/schema"
	gdprService "github.com/Chief-Strategist-J/platform-orchestrator/src/features/gdpr/services"
	grafanaSchema "github.com/Chief-Strategist-J/platform-orchestrator/src/features/grafana/schema"
	grafanaService "github.com/Chief-Strategist-J/platform-orchestrator/src/features/grafana/services"
	grafanaTypes "github.com/Chief-Strategist-J/platform-orchestrator/src/features/grafana/types"
	healthSchema "github.com/Chief-Strategist-J/platform-orchestrator/src/features/health/schema"
	healthService "github.com/Chief-Strategist-J/platform-orchestrator/src/features/health/services"
	portsService "github.com/Chief-Strategist-J/platform-orchestrator/src/features/ports/services"
	prereqsService "github.com/Chief-Strategist-J/platform-orchestrator/src/features/prereqs/services"
	scaleSchema "github.com/Chief-Strategist-J/platform-orchestrator/src/features/scale/schema"
	scaleService "github.com/Chief-Strategist-J/platform-orchestrator/src/features/scale/services"
	servicesSchema "github.com/Chief-Strategist-J/platform-orchestrator/src/features/services/schema"
	servicesService "github.com/Chief-Strategist-J/platform-orchestrator/src/features/services/services"
	setupSchema "github.com/Chief-Strategist-J/platform-orchestrator/src/features/setup/schema"
	setupService "github.com/Chief-Strategist-J/platform-orchestrator/src/features/setup/services"
	stackSchema "github.com/Chief-Strategist-J/platform-orchestrator/src/features/stack/schema"
	stackService "github.com/Chief-Strategist-J/platform-orchestrator/src/features/stack/services"
	traefikSchema "github.com/Chief-Strategist-J/platform-orchestrator/src/features/traefik/schema"
	traefikService "github.com/Chief-Strategist-J/platform-orchestrator/src/features/traefik/services"
	traefikTypes "github.com/Chief-Strategist-J/platform-orchestrator/src/features/traefik/types"
	"github.com/Chief-Strategist-J/platform-orchestrator/src/shared/types"
)

type OrchestratorHandler struct {
	stackService      *stackService.StackService
	scaleService      *scaleService.ScaleService
	healthService     *healthService.HealthService
	certsService      *certsService.CertService
	backupService     *backupService.BackupService
	cloudflareService *cloudflareService.CloudflareService
	gdprService       *gdprService.GDPRService
	prereqService     *prereqsService.PrereqService
	setupService      *setupService.SetupService
	portService       *portsService.PortService
	configService     *configService.ConfigService
	grafanaService    *grafanaService.GrafanaService
	servicesService   *servicesService.ServicesService
	traefikService    *traefikService.TraefikService
	dnsService        *dnsService.DNSService
	baseDir           string
}

func NewOrchestratorHandler(
	stackSvc *stackService.StackService,
	scaleSvc *scaleService.ScaleService,
	healthSvc *healthService.HealthService,
	certsSvc *certsService.CertService,
	backupSvc *backupService.BackupService,
	cfSvc *cloudflareService.CloudflareService,
	gdprSvc *gdprService.GDPRService,
	prereqSvc *prereqsService.PrereqService,
	setupSvc *setupService.SetupService,
	portSvc *portsService.PortService,
	configSvc *configService.ConfigService,
	grafanaSvc *grafanaService.GrafanaService,
	servicesSvc *servicesService.ServicesService,
	traefikSvc *traefikService.TraefikService,
	dnsSvc *dnsService.DNSService,
	baseDir string,
) *OrchestratorHandler {
	return &OrchestratorHandler{
		stackService:      stackSvc,
		scaleService:      scaleSvc,
		healthService:     healthSvc,
		certsService:      certsSvc,
		backupService:     backupSvc,
		cloudflareService: cfSvc,
		gdprService:       gdprSvc,
		prereqService:     prereqSvc,
		setupService:      setupSvc,
		portService:       portSvc,
		configService:     configSvc,
		grafanaService:    grafanaSvc,
		servicesService:   servicesSvc,
		traefikService:    traefikSvc,
		dnsService:        dnsSvc,
		baseDir:           baseDir,
	}
}

func (h *OrchestratorHandler) writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func (h *OrchestratorHandler) HandleStackUp(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Profiles       []string `json:"profiles"`
		Detach         bool     `json:"detach"`
		NetworkName    string   `json:"networkName,omitempty"`
		NetworkSubnet  string   `json:"networkSubnet,omitempty"`
		NetworkGateway string   `json:"networkGateway,omitempty"`
	}
	if r.Body != nil && r.ContentLength != 0 {
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			h.writeJSON(w, http.StatusBadRequest, types.NewErrorResponse[any]("ERR_INVALID_BODY", err.Error(), "body", "v1"))
			return
		}
	}

	outcome, err := h.stackService.StartStack(r.Context(), stackSchema.StackUpCommand{
		Profiles:       body.Profiles,
		Detach:         body.Detach,
		NetworkName:    body.NetworkName,
		NetworkSubnet:  body.NetworkSubnet,
		NetworkGateway: body.NetworkGateway,
	})
	if err != nil {
		h.writeJSON(w, http.StatusInternalServerError, types.NewErrorResponse[any]("ERR_STACK_UP", err.Error(), "stack", "v1"))
		return
	}

	h.writeJSON(w, http.StatusOK, types.NewSuccessResponse(outcome, "v1"))
}

func (h *OrchestratorHandler) HandleStackDown(w http.ResponseWriter, r *http.Request) {
	outcome, err := h.stackService.StopStack(r.Context())
	if err != nil {
		h.writeJSON(w, http.StatusInternalServerError, types.NewErrorResponse[any]("ERR_STACK_DOWN", err.Error(), "stack", "v1"))
		return
	}
	h.writeJSON(w, http.StatusOK, types.NewSuccessResponse(outcome, "v1"))
}

func (h *OrchestratorHandler) HandleStackStatus(w http.ResponseWriter, r *http.Request) {
	statuses, err := h.stackService.GetStatus(r.Context())
	if err != nil {
		h.writeJSON(w, http.StatusInternalServerError, types.NewErrorResponse[any]("ERR_STACK_STATUS", err.Error(), "stack", "v1"))
		return
	}
	h.writeJSON(w, http.StatusOK, types.NewSuccessResponse(map[string]any{"services": statuses}, "v1"))
}

func (h *OrchestratorHandler) HandleScaleService(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Service  string `json:"service"`
		Replicas int    `json:"replicas"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Service == "" || body.Replicas < 1 {
		h.writeJSON(w, http.StatusBadRequest, types.NewErrorResponse[any]("ERR_INVALID_SCALE_REQUEST", "invalid service or replica count", "scale", "v1"))
		return
	}

	if err := h.scaleService.ScaleService(r.Context(), scaleSchema.ScaleServiceCommand{
		Service:  body.Service,
		Replicas: body.Replicas,
	}); err != nil {
		h.writeJSON(w, http.StatusInternalServerError, types.NewErrorResponse[any]("ERR_SCALE_FAILED", err.Error(), "scale", "v1"))
		return
	}

	h.writeJSON(w, http.StatusOK, types.NewSuccessResponse(map[string]any{
		"service":  body.Service,
		"replicas": body.Replicas,
		"message":  "Scale command applied",
	}, "v1"))
}

func (h *OrchestratorHandler) HandleScaleNode(w http.ResponseWriter, r *http.Request) {
	var body struct {
		NodeID           int    `json:"nodeId"`
		PrimaryDataHost  string `json:"primaryDataHost"`
		EnableCloudflare bool   `json:"enableCloudflare"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.NodeID < 2 {
		h.writeJSON(w, http.StatusBadRequest, types.NewErrorResponse[any]("ERR_INVALID_NODE_REQUEST", "nodeId must be >= 2", "nodeId", "v1"))
		return
	}

	if body.PrimaryDataHost == "" {
		body.PrimaryDataHost = "host.docker.internal"
	}

	meta, err := h.scaleService.LaunchComputeNode(r.Context(), scaleSchema.LaunchNodeCommand{
		NodeID:           body.NodeID,
		PrimaryDataHost:  body.PrimaryDataHost,
		EnableCloudflare: body.EnableCloudflare,
	})
	if err != nil {
		h.writeJSON(w, http.StatusInternalServerError, types.NewErrorResponse[any]("ERR_LAUNCH_NODE", err.Error(), "node", "v1"))
		return
	}

	h.writeJSON(w, http.StatusOK, types.NewSuccessResponse(meta, "v1"))
}

func (h *OrchestratorHandler) HandleTerminateNode(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) < 4 {
		h.writeJSON(w, http.StatusBadRequest, types.NewErrorResponse[any]("ERR_INVALID_URL", "missing node ID", "nodeId", "v1"))
		return
	}

	nodeID, err := strconv.Atoi(parts[3])
	if err != nil {
		h.writeJSON(w, http.StatusBadRequest, types.NewErrorResponse[any]("ERR_INVALID_NODE_ID", "nodeId must be integer", "nodeId", "v1"))
		return
	}

	if err := h.scaleService.TerminateComputeNode(r.Context(), nodeID); err != nil {
		h.writeJSON(w, http.StatusInternalServerError, types.NewErrorResponse[any]("ERR_TERMINATE_NODE", err.Error(), "nodeId", "v1"))
		return
	}

	h.writeJSON(w, http.StatusOK, types.NewSuccessResponse(map[string]any{
		"message": "Compute node terminated",
		"nodeId":  nodeID,
	}, "v1"))
}

func (h *OrchestratorHandler) HandleDeepHealth(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Services    []string `json:"services,omitempty"`
		PrimaryHost string   `json:"primaryHost,omitempty"`
		TimeoutMs   int      `json:"timeoutMs,omitempty"`
		Overrides   []struct {
			Service       string `json:"service"`
			Host          string `json:"host,omitempty"`
			Port          int    `json:"port,omitempty"`
			Username      string `json:"username,omitempty"`
			Password      string `json:"password,omitempty"`
			Database      string `json:"database,omitempty"`
			Container     string `json:"container,omitempty"`
			KafkaTopic    string `json:"kafkaTopic,omitempty"`
			GrafanaURL    string `json:"grafanaUrl,omitempty"`
			GrafanaUser   string `json:"grafanaUser,omitempty"`
			GrafanaPass   string `json:"grafanaPass,omitempty"`
			TemporalNS    string `json:"temporalNs,omitempty"`
			OtelGRPCPort  int    `json:"otelGrpcPort,omitempty"`
			ClickHouseDB  string `json:"clickhouseDb,omitempty"`
		} `json:"overrides,omitempty"`
	}
	if r.Body != nil && r.ContentLength != 0 {
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			h.writeJSON(w, http.StatusBadRequest, types.NewErrorResponse[any]("ERR_INVALID_BODY", err.Error(), "body", "v1"))
			return
		}
	}

	defaults := healthSchema.DefaultDeepProbeConfigs(body.PrimaryHost)
	filterSet := make(map[string]struct{}, len(body.Services))
	for _, s := range body.Services {
		filterSet[s] = struct{}{}
	}

	var overrides []healthSchema.DeepProbeConfig
	for _, d := range defaults {
		if len(filterSet) > 0 {
			if _, ok := filterSet[d.Service]; !ok {
				continue
			}
		}
		overrides = append(overrides, d)
	}
	for _, o := range body.Overrides {
		timeout := healthSchema.DeepProbeConfig{}.Timeout
		if body.TimeoutMs > 0 {
			timeout = time.Duration(body.TimeoutMs) * time.Millisecond
		}
		overrides = append(overrides, healthSchema.DeepProbeConfig{
			Service:      o.Service,
			Host:         o.Host,
			Port:         o.Port,
			Timeout:      timeout,
			Username:     o.Username,
			Password:     o.Password,
			Database:     o.Database,
			Container:    o.Container,
			KafkaTopic:   o.KafkaTopic,
			GrafanaURL:   o.GrafanaURL,
			GrafanaUser:  o.GrafanaUser,
			GrafanaPass:  o.GrafanaPass,
			TemporalNS:   o.TemporalNS,
			OtelGRPCPort: o.OtelGRPCPort,
			ClickHouseDB: o.ClickHouseDB,
		})
	}

	report := h.healthService.RunDeepHealthChecks(r.Context(), overrides)
	status := http.StatusOK
	if !report.Healthy {
		status = http.StatusServiceUnavailable
	}
	h.writeJSON(w, status, types.NewSuccessResponse(report, "v1"))
}

func (h *OrchestratorHandler) HandleGenerateCerts(w http.ResponseWriter, r *http.Request) {
	spec := certsSchema.DefaultCertSpec(h.baseDir)
	res, err := h.certsService.EnsureCertificates(r.Context(), spec)
	if err != nil {
		h.writeJSON(w, http.StatusInternalServerError, types.NewErrorResponse[any]("ERR_GENERATE_CERTS", err.Error(), "certs", "v1"))
		return
	}
	h.writeJSON(w, http.StatusOK, types.NewSuccessResponse(res, "v1"))
}

func (h *OrchestratorHandler) HandleBackupExecute(w http.ResponseWriter, r *http.Request) {
	var opts backupSchema.BackupOptions
	if r.Body != nil && r.ContentLength != 0 {
		if err := json.NewDecoder(r.Body).Decode(&opts); err != nil {
			h.writeJSON(w, http.StatusBadRequest, types.NewErrorResponse[any]("ERR_INVALID_BODY", err.Error(), "body", "v1"))
			return
		}
	}

	report, err := h.backupService.ExecuteBackupAndPurge(r.Context(), opts)
	if err != nil {
		h.writeJSON(w, http.StatusInternalServerError, types.NewErrorResponse[any]("ERR_BACKUP_EXECUTE", err.Error(), "backup", "v1"))
		return
	}
	h.writeJSON(w, http.StatusOK, types.NewSuccessResponse(report, "v1"))
}

func (h *OrchestratorHandler) HandleSetupBootstrap(w http.ResponseWriter, r *http.Request) {
	var cmd setupSchema.SetupCommand
	if r.Body != nil && r.ContentLength != 0 {
		if err := json.NewDecoder(r.Body).Decode(&cmd); err != nil {
			h.writeJSON(w, http.StatusBadRequest, types.NewErrorResponse[any]("ERR_INVALID_BODY", err.Error(), "body", "v1"))
			return
		}
	}

	report, err := h.setupService.RunSetupPipeline(r.Context(), cmd)
	if err != nil {
		h.writeJSON(w, http.StatusInternalServerError, types.NewErrorResponse[any]("ERR_SETUP_FAILED", err.Error(), "setup", "v1"))
		return
	}
	h.writeJSON(w, http.StatusOK, types.NewSuccessResponse(report, "v1"))
}

func (h *OrchestratorHandler) HandlePrereqsAudit(w http.ResponseWriter, r *http.Request) {
	report := h.prereqService.VerifySystemPrerequisites(r.Context())
	h.writeJSON(w, http.StatusOK, types.NewSuccessResponse(report, "v1"))
}

func (h *OrchestratorHandler) HandleCloudflareToken(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Token == "" {
		h.writeJSON(w, http.StatusBadRequest, types.NewErrorResponse[any]("ERR_INVALID_TOKEN", "token cannot be empty", "token", "v1"))
		return
	}

	if err := h.cloudflareService.SaveTunnelToken(body.Token); err != nil {
		h.writeJSON(w, http.StatusInternalServerError, types.NewErrorResponse[any]("ERR_SAVE_TOKEN", err.Error(), "token", "v1"))
		return
	}

	h.writeJSON(w, http.StatusOK, types.NewSuccessResponse(cloudflareSchema.CloudflareTunnelReport{
		Status:  "configured",
		Active:  false,
		Message: "Cloudflare tunnel token saved to environment",
	}, "v1"))
}

func (h *OrchestratorHandler) HandleCloudflareStart(w http.ResponseWriter, r *http.Request) {
	report, err := h.cloudflareService.StartTunnel(r.Context())
	if err != nil {
		h.writeJSON(w, http.StatusInternalServerError, types.NewErrorResponse[any]("ERR_CF_START", err.Error(), "cloudflare", "v1"))
		return
	}
	h.writeJSON(w, http.StatusOK, types.NewSuccessResponse(report, "v1"))
}

func (h *OrchestratorHandler) HandleCloudflareStop(w http.ResponseWriter, r *http.Request) {
	report, err := h.cloudflareService.StopTunnel(r.Context())
	if err != nil {
		h.writeJSON(w, http.StatusInternalServerError, types.NewErrorResponse[any]("ERR_CF_STOP", err.Error(), "cloudflare", "v1"))
		return
	}
	h.writeJSON(w, http.StatusOK, types.NewSuccessResponse(report, "v1"))
}

func (h *OrchestratorHandler) HandleCloudflareStatus(w http.ResponseWriter, r *http.Request) {
	st, err := h.cloudflareService.GetStatus(r.Context())
	if err != nil {
		h.writeJSON(w, http.StatusInternalServerError, types.NewErrorResponse[any]("ERR_CF_STATUS", err.Error(), "cloudflare", "v1"))
		return
	}
	h.writeJSON(w, http.StatusOK, types.NewSuccessResponse(map[string]string{"status": st}, "v1"))
}

func (h *OrchestratorHandler) HandleGdprErasure(w http.ResponseWriter, r *http.Request) {
	var req gdprSchema.ErasureRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || (req.UserID == "" && req.CustomerID == "") {
		h.writeJSON(w, http.StatusBadRequest, types.NewErrorResponse[any]("ERR_INVALID_ERASURE_REQ", "must provide userId or customerId", "targetId", "v1"))
		return
	}

	report, err := h.gdprService.ExecuteErasure(r.Context(), req)
	if err != nil {
		h.writeJSON(w, http.StatusInternalServerError, types.NewErrorResponse[any]("ERR_GDPR_ERASURE", err.Error(), "gdpr", "v1"))
		return
	}
	h.writeJSON(w, http.StatusOK, types.NewSuccessResponse(report, "v1"))
}

func (h *OrchestratorHandler) HandlePortsStatus(w http.ResponseWriter, r *http.Request) {
	results := h.portService.CheckPorts(r.Context(), nil)
	h.writeJSON(w, http.StatusOK, types.NewSuccessResponse(map[string]any{"ports": results}, "v1"))
}

func (h *OrchestratorHandler) HandlePortsFree(w http.ResponseWriter, r *http.Request) {
	if err := h.portService.FreePorts(r.Context(), nil); err != nil {
		h.writeJSON(w, http.StatusInternalServerError, types.NewErrorResponse[any]("ERR_PORTS_FREE", err.Error(), "ports", "v1"))
		return
	}
	h.writeJSON(w, http.StatusOK, types.NewSuccessResponse(map[string]any{
		"freed":   true,
		"message": "Platform ports verified and conflicting sockets freed",
	}, "v1"))
}

func (h *OrchestratorHandler) HandleGetConfig(w http.ResponseWriter, r *http.Request) {
	report, err := h.configService.GetPlatformConfig(r.Context())
	if err != nil {
		h.writeJSON(w, http.StatusInternalServerError, types.NewErrorResponse[any]("ERR_CONFIG_FETCH", err.Error(), "config", "v1"))
		return
	}
	h.writeJSON(w, http.StatusOK, types.NewSuccessResponse(report, "v1"))
}

func (h *OrchestratorHandler) HandleUpdateConfig(w http.ResponseWriter, r *http.Request) {
	var cmd configSchema.UpdateConfigCommand
	if err := json.NewDecoder(r.Body).Decode(&cmd); err != nil {
		h.writeJSON(w, http.StatusBadRequest, types.NewErrorResponse[any]("ERR_INVALID_BODY", err.Error(), "body", "v1"))
		return
	}
	report, err := h.configService.UpdatePlatformConfig(r.Context(), cmd)
	if err != nil {
		h.writeJSON(w, http.StatusBadRequest, types.NewErrorResponse[any]("ERR_CONFIG_UPDATE", err.Error(), "config", "v1"))
		return
	}
	h.writeJSON(w, http.StatusOK, types.NewSuccessResponse(report, "v1"))
}

func (h *OrchestratorHandler) extractClientOptions(r *http.Request) grafanaTypes.ClientOptions {
	q := r.URL.Query()
	timeoutSec := 10
	if tStr := q.Get("timeout"); tStr != "" {
		if t, err := strconv.Atoi(tStr); err == nil && t > 0 {
			timeoutSec = t
		}
	}
	return grafanaTypes.ClientOptions{
		GrafanaURL: q.Get("grafanaUrl"),
		Username:   q.Get("username"),
		Password:   q.Get("password"),
		Timeout:    time.Duration(timeoutSec) * time.Second,
	}
}

func (h *OrchestratorHandler) HandleListDatasources(w http.ResponseWriter, r *http.Request) {
	opts := h.extractClientOptions(r)
	list, err := h.grafanaService.ListDatasources(r.Context(), opts)
	if err != nil {
		h.writeJSON(w, http.StatusInternalServerError, types.NewErrorResponse[any]("ERR_GRAFANA_LIST", err.Error(), "datasources", "v1"))
		return
	}
	h.writeJSON(w, http.StatusOK, types.NewSuccessResponse(list, "v1"))
}

func (h *OrchestratorHandler) HandleGetDatasource(w http.ResponseWriter, r *http.Request) {
	idOrUid := strings.TrimPrefix(r.URL.Path, "/api/v1/grafana/datasources/")
	idOrUid = strings.TrimSuffix(idOrUid, "/health")
	if idOrUid == "" {
		h.writeJSON(w, http.StatusBadRequest, types.NewErrorResponse[any]("ERR_INVALID_PARAM", "missing datasource id or uid", "idOrUid", "v1"))
		return
	}
	opts := h.extractClientOptions(r)
	ds, err := h.grafanaService.GetDatasource(r.Context(), opts, idOrUid)
	if err != nil {
		h.writeJSON(w, http.StatusNotFound, types.NewErrorResponse[any]("ERR_GRAFANA_NOT_FOUND", err.Error(), "idOrUid", "v1"))
		return
	}
	h.writeJSON(w, http.StatusOK, types.NewSuccessResponse(ds, "v1"))
}

func (h *OrchestratorHandler) HandleCreateDatasource(w http.ResponseWriter, r *http.Request) {
	var payload grafanaSchema.DatasourcePayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		h.writeJSON(w, http.StatusBadRequest, types.NewErrorResponse[any]("ERR_INVALID_BODY", err.Error(), "body", "v1"))
		return
	}
	opts := h.extractClientOptions(r)
	res, err := h.grafanaService.CreateDatasource(r.Context(), opts, payload)
	if err != nil {
		h.writeJSON(w, http.StatusBadRequest, types.NewErrorResponse[any]("ERR_GRAFANA_CREATE", err.Error(), "datasource", "v1"))
		return
	}
	h.writeJSON(w, http.StatusCreated, types.NewSuccessResponse(res, "v1"))
}

func (h *OrchestratorHandler) HandleUpdateDatasource(w http.ResponseWriter, r *http.Request) {
	idOrUid := strings.TrimPrefix(r.URL.Path, "/api/v1/grafana/datasources/")
	if idOrUid == "" {
		h.writeJSON(w, http.StatusBadRequest, types.NewErrorResponse[any]("ERR_INVALID_PARAM", "missing datasource id or uid", "idOrUid", "v1"))
		return
	}
	var payload grafanaSchema.DatasourcePayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		h.writeJSON(w, http.StatusBadRequest, types.NewErrorResponse[any]("ERR_INVALID_BODY", err.Error(), "body", "v1"))
		return
	}
	opts := h.extractClientOptions(r)
	res, err := h.grafanaService.UpdateDatasource(r.Context(), opts, idOrUid, payload)
	if err != nil {
		h.writeJSON(w, http.StatusBadRequest, types.NewErrorResponse[any]("ERR_GRAFANA_UPDATE", err.Error(), "datasource", "v1"))
		return
	}
	h.writeJSON(w, http.StatusOK, types.NewSuccessResponse(res, "v1"))
}

func (h *OrchestratorHandler) HandleDeleteDatasource(w http.ResponseWriter, r *http.Request) {
	idOrUid := strings.TrimPrefix(r.URL.Path, "/api/v1/grafana/datasources/")
	if idOrUid == "" {
		h.writeJSON(w, http.StatusBadRequest, types.NewErrorResponse[any]("ERR_INVALID_PARAM", "missing datasource id or uid", "idOrUid", "v1"))
		return
	}
	opts := h.extractClientOptions(r)
	res, err := h.grafanaService.DeleteDatasource(r.Context(), opts, idOrUid)
	if err != nil {
		h.writeJSON(w, http.StatusBadRequest, types.NewErrorResponse[any]("ERR_GRAFANA_DELETE", err.Error(), "datasource", "v1"))
		return
	}
	h.writeJSON(w, http.StatusOK, types.NewSuccessResponse(res, "v1"))
}

func (h *OrchestratorHandler) HandleTestDatasourceHealth(w http.ResponseWriter, r *http.Request) {
	idOrUid := strings.TrimPrefix(r.URL.Path, "/api/v1/grafana/datasources/")
	idOrUid = strings.TrimSuffix(idOrUid, "/health")
	if idOrUid == "" {
		h.writeJSON(w, http.StatusBadRequest, types.NewErrorResponse[any]("ERR_INVALID_PARAM", "missing datasource id or uid", "idOrUid", "v1"))
		return
	}
	opts := h.extractClientOptions(r)
	res, err := h.grafanaService.TestDatasourceHealth(r.Context(), opts, idOrUid)
	if err != nil {
		h.writeJSON(w, http.StatusBadRequest, types.NewErrorResponse[any]("ERR_GRAFANA_HEALTH", err.Error(), "datasource", "v1"))
		return
	}
	h.writeJSON(w, http.StatusOK, types.NewSuccessResponse(res, "v1"))
}

func (h *OrchestratorHandler) HandleSyncDatasources(w http.ResponseWriter, r *http.Request) {
	var opts grafanaSchema.DatasourceSyncOptions
	if r.Body != nil && r.ContentLength > 0 {
		_ = json.NewDecoder(r.Body).Decode(&opts)
	}
	if opts.GrafanaURL == "" {
		opts.GrafanaURL = r.URL.Query().Get("grafanaUrl")
	}
	if opts.GrafanaUser == "" {
		opts.GrafanaUser = r.URL.Query().Get("username")
	}
	if opts.GrafanaPass == "" {
		opts.GrafanaPass = r.URL.Query().Get("password")
	}
	opts.TestConnection = r.URL.Query().Get("test") != "false"

	report, err := h.grafanaService.SyncDatasources(r.Context(), opts)
	if err != nil {
		h.writeJSON(w, http.StatusInternalServerError, types.NewErrorResponse[any]("ERR_GRAFANA_SYNC", err.Error(), "sync", "v1"))
		return
	}
	h.writeJSON(w, http.StatusOK, types.NewSuccessResponse(report, "v1"))
}

func (h *OrchestratorHandler) HandleSearchDashboards(w http.ResponseWriter, r *http.Request) {
	opts := h.extractClientOptions(r)
	q := r.URL.Query()
	query := q.Get("query")
	folderUID := q.Get("folder")
	tag := q.Get("tag")

	results, err := h.grafanaService.SearchDashboards(r.Context(), opts, query, folderUID, tag)
	if err != nil {
		h.writeJSON(w, http.StatusInternalServerError, types.NewErrorResponse[any]("ERR_DASHBOARD_SEARCH", err.Error(), "dashboards", "v1"))
		return
	}
	h.writeJSON(w, http.StatusOK, types.NewSuccessResponse(results, "v1"))
}

func (h *OrchestratorHandler) HandleGetDashboard(w http.ResponseWriter, r *http.Request) {
	uid := strings.TrimPrefix(r.URL.Path, "/api/v1/grafana/dashboards/")
	if uid == "" {
		h.writeJSON(w, http.StatusBadRequest, types.NewErrorResponse[any]("ERR_INVALID_PARAM", "missing dashboard uid", "uid", "v1"))
		return
	}
	opts := h.extractClientOptions(r)
	detail, err := h.grafanaService.GetDashboard(r.Context(), opts, uid)
	if err != nil {
		h.writeJSON(w, http.StatusNotFound, types.NewErrorResponse[any]("ERR_DASHBOARD_NOT_FOUND", err.Error(), "uid", "v1"))
		return
	}
	h.writeJSON(w, http.StatusOK, types.NewSuccessResponse(detail, "v1"))
}

func (h *OrchestratorHandler) HandleSaveDashboard(w http.ResponseWriter, r *http.Request) {
	var payload grafanaSchema.DashboardPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		h.writeJSON(w, http.StatusBadRequest, types.NewErrorResponse[any]("ERR_INVALID_BODY", err.Error(), "body", "v1"))
		return
	}
	opts := h.extractClientOptions(r)
	res, err := h.grafanaService.CreateOrUpdateDashboard(r.Context(), opts, payload)
	if err != nil {
		h.writeJSON(w, http.StatusBadRequest, types.NewErrorResponse[any]("ERR_DASHBOARD_SAVE", err.Error(), "dashboard", "v1"))
		return
	}
	h.writeJSON(w, http.StatusOK, types.NewSuccessResponse(res, "v1"))
}

func (h *OrchestratorHandler) HandleImportDashboard(w http.ResponseWriter, r *http.Request) {
	var importOpts grafanaSchema.DashboardImportOptions
	if err := json.NewDecoder(r.Body).Decode(&importOpts); err != nil {
		h.writeJSON(w, http.StatusBadRequest, types.NewErrorResponse[any]("ERR_INVALID_BODY", err.Error(), "body", "v1"))
		return
	}
	opts := h.extractClientOptions(r)
	res, err := h.grafanaService.ImportDashboard(r.Context(), opts, importOpts)
	if err != nil {
		h.writeJSON(w, http.StatusBadRequest, types.NewErrorResponse[any]("ERR_DASHBOARD_IMPORT", err.Error(), "dashboard", "v1"))
		return
	}
	h.writeJSON(w, http.StatusOK, types.NewSuccessResponse(res, "v1"))
}

func (h *OrchestratorHandler) HandleDeleteDashboard(w http.ResponseWriter, r *http.Request) {
	uid := strings.TrimPrefix(r.URL.Path, "/api/v1/grafana/dashboards/")
	if uid == "" {
		h.writeJSON(w, http.StatusBadRequest, types.NewErrorResponse[any]("ERR_INVALID_PARAM", "missing dashboard uid", "uid", "v1"))
		return
	}
	opts := h.extractClientOptions(r)
	res, err := h.grafanaService.DeleteDashboard(r.Context(), opts, uid)
	if err != nil {
		h.writeJSON(w, http.StatusBadRequest, types.NewErrorResponse[any]("ERR_DASHBOARD_DELETE", err.Error(), "uid", "v1"))
		return
	}
	h.writeJSON(w, http.StatusOK, types.NewSuccessResponse(res, "v1"))
}

func (h *OrchestratorHandler) HandleListAlertRules(w http.ResponseWriter, r *http.Request) {
	opts := h.extractClientOptions(r)
	list, err := h.grafanaService.ListAlertRules(r.Context(), opts)
	if err != nil {
		h.writeJSON(w, http.StatusInternalServerError, types.NewErrorResponse[any]("ERR_ALERTS_LIST", err.Error(), "alerts", "v1"))
		return
	}
	h.writeJSON(w, http.StatusOK, types.NewSuccessResponse(list, "v1"))
}

func (h *OrchestratorHandler) HandleGetAlertRule(w http.ResponseWriter, r *http.Request) {
	uid := strings.TrimPrefix(r.URL.Path, "/api/v1/grafana/alerts/")
	if uid == "" {
		h.writeJSON(w, http.StatusBadRequest, types.NewErrorResponse[any]("ERR_INVALID_PARAM", "missing alert rule uid", "uid", "v1"))
		return
	}
	opts := h.extractClientOptions(r)
	rule, err := h.grafanaService.GetAlertRule(r.Context(), opts, uid)
	if err != nil {
		h.writeJSON(w, http.StatusNotFound, types.NewErrorResponse[any]("ERR_ALERT_NOT_FOUND", err.Error(), "uid", "v1"))
		return
	}
	h.writeJSON(w, http.StatusOK, types.NewSuccessResponse(rule, "v1"))
}

func (h *OrchestratorHandler) HandleSaveAlertRule(w http.ResponseWriter, r *http.Request) {
	var rule grafanaSchema.AlertRulePayload
	if err := json.NewDecoder(r.Body).Decode(&rule); err != nil {
		h.writeJSON(w, http.StatusBadRequest, types.NewErrorResponse[any]("ERR_INVALID_BODY", err.Error(), "body", "v1"))
		return
	}
	opts := h.extractClientOptions(r)
	res, err := h.grafanaService.CreateOrUpdateAlertRule(r.Context(), opts, rule)
	if err != nil {
		h.writeJSON(w, http.StatusBadRequest, types.NewErrorResponse[any]("ERR_ALERT_SAVE", err.Error(), "alert", "v1"))
		return
	}
	h.writeJSON(w, http.StatusOK, types.NewSuccessResponse(res, "v1"))
}

func (h *OrchestratorHandler) HandleDeleteAlertRule(w http.ResponseWriter, r *http.Request) {
	uid := strings.TrimPrefix(r.URL.Path, "/api/v1/grafana/alerts/")
	if uid == "" {
		h.writeJSON(w, http.StatusBadRequest, types.NewErrorResponse[any]("ERR_INVALID_PARAM", "missing alert rule uid", "uid", "v1"))
		return
	}
	opts := h.extractClientOptions(r)
	res, err := h.grafanaService.DeleteAlertRule(r.Context(), opts, uid)
	if err != nil {
		h.writeJSON(w, http.StatusBadRequest, types.NewErrorResponse[any]("ERR_ALERT_DELETE", err.Error(), "uid", "v1"))
		return
	}
	h.writeJSON(w, http.StatusOK, types.NewSuccessResponse(res, "v1"))
}

func (h *OrchestratorHandler) HandleListContactPoints(w http.ResponseWriter, r *http.Request) {
	opts := h.extractClientOptions(r)
	list, err := h.grafanaService.ListContactPoints(r.Context(), opts)
	if err != nil {
		h.writeJSON(w, http.StatusInternalServerError, types.NewErrorResponse[any]("ERR_CONTACT_POINTS_LIST", err.Error(), "contactPoints", "v1"))
		return
	}
	h.writeJSON(w, http.StatusOK, types.NewSuccessResponse(list, "v1"))
}

func (h *OrchestratorHandler) HandleSaveContactPoint(w http.ResponseWriter, r *http.Request) {
	var cp grafanaSchema.ContactPointPayload
	if err := json.NewDecoder(r.Body).Decode(&cp); err != nil {
		h.writeJSON(w, http.StatusBadRequest, types.NewErrorResponse[any]("ERR_INVALID_BODY", err.Error(), "body", "v1"))
		return
	}
	opts := h.extractClientOptions(r)
	res, err := h.grafanaService.CreateOrUpdateContactPoint(r.Context(), opts, cp)
	if err != nil {
		h.writeJSON(w, http.StatusBadRequest, types.NewErrorResponse[any]("ERR_CONTACT_POINT_SAVE", err.Error(), "contactPoint", "v1"))
		return
	}
	h.writeJSON(w, http.StatusOK, types.NewSuccessResponse(res, "v1"))
}

func (h *OrchestratorHandler) HandleDeleteContactPoint(w http.ResponseWriter, r *http.Request) {
	uid := strings.TrimPrefix(r.URL.Path, "/api/v1/grafana/contact-points/")
	if uid == "" {
		h.writeJSON(w, http.StatusBadRequest, types.NewErrorResponse[any]("ERR_INVALID_PARAM", "missing contact point uid", "uid", "v1"))
		return
	}
	opts := h.extractClientOptions(r)
	res, err := h.grafanaService.DeleteContactPoint(r.Context(), opts, uid)
	if err != nil {
		h.writeJSON(w, http.StatusBadRequest, types.NewErrorResponse[any]("ERR_CONTACT_POINT_DELETE", err.Error(), "uid", "v1"))
		return
	}
	h.writeJSON(w, http.StatusOK, types.NewSuccessResponse(res, "v1"))
}

func (h *OrchestratorHandler) HandleTestContactPoint(w http.ResponseWriter, r *http.Request) {
	var cp grafanaSchema.ContactPointPayload
	if err := json.NewDecoder(r.Body).Decode(&cp); err != nil {
		h.writeJSON(w, http.StatusBadRequest, types.NewErrorResponse[any]("ERR_INVALID_BODY", err.Error(), "body", "v1"))
		return
	}
	opts := h.extractClientOptions(r)
	res, err := h.grafanaService.TestContactPoint(r.Context(), opts, cp)
	if err != nil {
		h.writeJSON(w, http.StatusBadRequest, types.NewErrorResponse[any]("ERR_CONTACT_POINT_TEST", err.Error(), "contactPoint", "v1"))
		return
	}
	h.writeJSON(w, http.StatusOK, types.NewSuccessResponse(res, "v1"))
}

func (h *OrchestratorHandler) HandleListServices(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	filter := servicesSchema.ServiceFilterOptions{
		Category: q.Get("category"),
		Type:     q.Get("type"),
		Query:    q.Get("query"),
		Tag:      q.Get("tag"),
	}
	list, err := h.servicesService.ListServices(r.Context(), filter)
	if err != nil {
		h.writeJSON(w, http.StatusInternalServerError, types.NewErrorResponse[any]("ERR_SERVICES_LIST", err.Error(), "services", "v1"))
		return
	}
	h.writeJSON(w, http.StatusOK, types.NewSuccessResponse(list, "v1"))
}

func (h *OrchestratorHandler) HandleGetService(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/v1/services/")
	id = strings.TrimSuffix(id, "/health")
	id = strings.TrimSuffix(id, "/sync-to-grafana")
	if id == "" {
		h.writeJSON(w, http.StatusBadRequest, types.NewErrorResponse[any]("ERR_INVALID_PARAM", "missing service id", "id", "v1"))
		return
	}
	svc, err := h.servicesService.GetService(r.Context(), id)
	if err != nil {
		h.writeJSON(w, http.StatusNotFound, types.NewErrorResponse[any]("ERR_SERVICE_NOT_FOUND", err.Error(), "id", "v1"))
		return
	}
	h.writeJSON(w, http.StatusOK, types.NewSuccessResponse(svc, "v1"))
}

func (h *OrchestratorHandler) HandleRegisterService(w http.ResponseWriter, r *http.Request) {
	var svc servicesSchema.ServiceDefinition
	if err := json.NewDecoder(r.Body).Decode(&svc); err != nil {
		h.writeJSON(w, http.StatusBadRequest, types.NewErrorResponse[any]("ERR_INVALID_BODY", err.Error(), "body", "v1"))
		return
	}
	created, err := h.servicesService.RegisterService(r.Context(), svc)
	if err != nil {
		h.writeJSON(w, http.StatusBadRequest, types.NewErrorResponse[any]("ERR_SERVICE_REGISTER", err.Error(), "service", "v1"))
		return
	}
	h.writeJSON(w, http.StatusCreated, types.NewSuccessResponse(created, "v1"))
}

func (h *OrchestratorHandler) HandleUpdateService(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/v1/services/")
	if id == "" {
		h.writeJSON(w, http.StatusBadRequest, types.NewErrorResponse[any]("ERR_INVALID_PARAM", "missing service id", "id", "v1"))
		return
	}
	var patch servicesSchema.ServiceDefinition
	if err := json.NewDecoder(r.Body).Decode(&patch); err != nil {
		h.writeJSON(w, http.StatusBadRequest, types.NewErrorResponse[any]("ERR_INVALID_BODY", err.Error(), "body", "v1"))
		return
	}
	updated, err := h.servicesService.UpdateService(r.Context(), id, patch)
	if err != nil {
		h.writeJSON(w, http.StatusBadRequest, types.NewErrorResponse[any]("ERR_SERVICE_UPDATE", err.Error(), "service", "v1"))
		return
	}
	h.writeJSON(w, http.StatusOK, types.NewSuccessResponse(updated, "v1"))
}

func (h *OrchestratorHandler) HandleDeleteService(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/v1/services/")
	if id == "" {
		h.writeJSON(w, http.StatusBadRequest, types.NewErrorResponse[any]("ERR_INVALID_PARAM", "missing service id", "id", "v1"))
		return
	}
	res, err := h.servicesService.DeleteService(r.Context(), id)
	if err != nil {
		h.writeJSON(w, http.StatusBadRequest, types.NewErrorResponse[any]("ERR_SERVICE_DELETE", err.Error(), "id", "v1"))
		return
	}
	h.writeJSON(w, http.StatusOK, types.NewSuccessResponse(res, "v1"))
}

func (h *OrchestratorHandler) HandleTestServiceHealth(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/v1/services/")
	id = strings.TrimSuffix(id, "/health")
	if id == "" {
		h.writeJSON(w, http.StatusBadRequest, types.NewErrorResponse[any]("ERR_INVALID_PARAM", "missing service id", "id", "v1"))
		return
	}
	res, err := h.servicesService.TestServiceHealth(r.Context(), id)
	if err != nil {
		h.writeJSON(w, http.StatusBadRequest, types.NewErrorResponse[any]("ERR_SERVICE_HEALTH", err.Error(), "id", "v1"))
		return
	}
	h.writeJSON(w, http.StatusOK, types.NewSuccessResponse(res, "v1"))
}

func (h *OrchestratorHandler) HandleSyncServiceToGrafana(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/v1/services/")
	id = strings.TrimSuffix(id, "/sync-to-grafana")
	if id == "" {
		h.writeJSON(w, http.StatusBadRequest, types.NewErrorResponse[any]("ERR_INVALID_PARAM", "missing service id", "id", "v1"))
		return
	}
	res, err := h.servicesService.SyncToGrafana(r.Context(), id, h.grafanaService)
	if err != nil {
		h.writeJSON(w, http.StatusBadRequest, types.NewErrorResponse[any]("ERR_SERVICE_GRAFANA_SYNC", err.Error(), "service", "v1"))
		return
	}
	h.writeJSON(w, http.StatusOK, types.NewSuccessResponse(res, "v1"))
}

func (h *OrchestratorHandler) HandleTraefikPing(w http.ResponseWriter, r *http.Request) {
	opts := traefikTypes.ClientOptions{Timeout: 10 * time.Second}
	res, err := h.traefikService.Ping(r.Context(), opts)
	if err != nil {
		h.writeJSON(w, http.StatusServiceUnavailable, types.NewErrorResponse[any]("ERR_TRAEFIK_UNREACHABLE", err.Error(), "traefik", "v1"))
		return
	}
	h.writeJSON(w, http.StatusOK, types.NewSuccessResponse(res, "v1"))
}

func (h *OrchestratorHandler) HandleTraefikOverview(w http.ResponseWriter, r *http.Request) {
	opts := traefikTypes.ClientOptions{Timeout: 10 * time.Second}
	res, err := h.traefikService.GetOverview(r.Context(), opts)
	if err != nil {
		h.writeJSON(w, http.StatusInternalServerError, types.NewErrorResponse[any]("ERR_TRAEFIK_OVERVIEW", err.Error(), "traefik", "v1"))
		return
	}
	h.writeJSON(w, http.StatusOK, types.NewSuccessResponse(res, "v1"))
}

func (h *OrchestratorHandler) HandleTraefikEntryPoints(w http.ResponseWriter, r *http.Request) {
	opts := traefikTypes.ClientOptions{Timeout: 10 * time.Second}
	res, err := h.traefikService.ListEntryPoints(r.Context(), opts)
	if err != nil {
		h.writeJSON(w, http.StatusInternalServerError, types.NewErrorResponse[any]("ERR_TRAEFIK_ENTRYPOINTS", err.Error(), "traefik", "v1"))
		return
	}
	h.writeJSON(w, http.StatusOK, types.NewSuccessResponse(res, "v1"))
}

func (h *OrchestratorHandler) HandleListTraefikHTTPRouters(w http.ResponseWriter, r *http.Request) {
	opts := traefikTypes.ClientOptions{Timeout: 10 * time.Second}
	res, err := h.traefikService.ListHTTPRouters(r.Context(), opts)
	if err != nil {
		h.writeJSON(w, http.StatusInternalServerError, types.NewErrorResponse[any]("ERR_TRAEFIK_ROUTERS", err.Error(), "traefik", "v1"))
		return
	}
	h.writeJSON(w, http.StatusOK, types.NewSuccessResponse(res, "v1"))
}

func (h *OrchestratorHandler) HandleGetTraefikHTTPRouter(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(r.URL.Path, "/api/v1/traefik/routers/")
	if name == "" {
		h.writeJSON(w, http.StatusBadRequest, types.NewErrorResponse[any]("ERR_INVALID_PARAM", "missing router name", "name", "v1"))
		return
	}
	opts := traefikTypes.ClientOptions{Timeout: 10 * time.Second}
	res, err := h.traefikService.GetHTTPRouter(r.Context(), opts, name)
	if err != nil {
		h.writeJSON(w, http.StatusNotFound, types.NewErrorResponse[any]("ERR_ROUTER_NOT_FOUND", err.Error(), "name", "v1"))
		return
	}
	h.writeJSON(w, http.StatusOK, types.NewSuccessResponse(res, "v1"))
}

func (h *OrchestratorHandler) HandleSaveTraefikHTTPRouter(w http.ResponseWriter, r *http.Request) {
	var body traefikSchema.HTTPRouterDefinition
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		h.writeJSON(w, http.StatusBadRequest, types.NewErrorResponse[any]("ERR_INVALID_BODY", err.Error(), "body", "v1"))
		return
	}
	res, err := h.traefikService.SaveHTTPRouter(r.Context(), body)
	if err != nil {
		h.writeJSON(w, http.StatusBadRequest, types.NewErrorResponse[any]("ERR_ROUTER_SAVE", err.Error(), "router", "v1"))
		return
	}
	h.writeJSON(w, http.StatusOK, types.NewSuccessResponse(res, "v1"))
}

func (h *OrchestratorHandler) HandleDeleteTraefikHTTPRouter(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(r.URL.Path, "/api/v1/traefik/routers/")
	if name == "" {
		h.writeJSON(w, http.StatusBadRequest, types.NewErrorResponse[any]("ERR_INVALID_PARAM", "missing router name", "name", "v1"))
		return
	}
	res, err := h.traefikService.DeleteHTTPRouter(r.Context(), name)
	if err != nil {
		h.writeJSON(w, http.StatusBadRequest, types.NewErrorResponse[any]("ERR_ROUTER_DELETE", err.Error(), "name", "v1"))
		return
	}
	h.writeJSON(w, http.StatusOK, types.NewSuccessResponse(res, "v1"))
}

func (h *OrchestratorHandler) HandleListTraefikHTTPServices(w http.ResponseWriter, r *http.Request) {
	opts := traefikTypes.ClientOptions{Timeout: 10 * time.Second}
	res, err := h.traefikService.ListHTTPServices(r.Context(), opts)
	if err != nil {
		h.writeJSON(w, http.StatusInternalServerError, types.NewErrorResponse[any]("ERR_TRAEFIK_SERVICES", err.Error(), "traefik", "v1"))
		return
	}
	h.writeJSON(w, http.StatusOK, types.NewSuccessResponse(res, "v1"))
}

func (h *OrchestratorHandler) HandleListTraefikMiddlewares(w http.ResponseWriter, r *http.Request) {
	opts := traefikTypes.ClientOptions{Timeout: 10 * time.Second}
	res, err := h.traefikService.ListMiddlewares(r.Context(), opts)
	if err != nil {
		h.writeJSON(w, http.StatusInternalServerError, types.NewErrorResponse[any]("ERR_TRAEFIK_MIDDLEWARES", err.Error(), "traefik", "v1"))
		return
	}
	h.writeJSON(w, http.StatusOK, types.NewSuccessResponse(res, "v1"))
}

func (h *OrchestratorHandler) HandleListTraefikTCPRouters(w http.ResponseWriter, r *http.Request) {
	opts := traefikTypes.ClientOptions{Timeout: 10 * time.Second}
	res, err := h.traefikService.ListTCPRouters(r.Context(), opts)
	if err != nil {
		h.writeJSON(w, http.StatusInternalServerError, types.NewErrorResponse[any]("ERR_TRAEFIK_TCP_ROUTERS", err.Error(), "traefik", "v1"))
		return
	}
	h.writeJSON(w, http.StatusOK, types.NewSuccessResponse(res, "v1"))
}

func (h *OrchestratorHandler) HandleSaveTraefikTCPRouter(w http.ResponseWriter, r *http.Request) {
	var body traefikSchema.TCPRouterDefinition
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		h.writeJSON(w, http.StatusBadRequest, types.NewErrorResponse[any]("ERR_INVALID_BODY", err.Error(), "body", "v1"))
		return
	}
	res, err := h.traefikService.SaveTCPRouter(r.Context(), body)
	if err != nil {
		h.writeJSON(w, http.StatusBadRequest, types.NewErrorResponse[any]("ERR_TCP_ROUTER_SAVE", err.Error(), "router", "v1"))
		return
	}
	h.writeJSON(w, http.StatusOK, types.NewSuccessResponse(res, "v1"))
}

func (h *OrchestratorHandler) HandleDeleteTraefikTCPRouter(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(r.URL.Path, "/api/v1/traefik/tcp/routers/")
	if name == "" {
		h.writeJSON(w, http.StatusBadRequest, types.NewErrorResponse[any]("ERR_INVALID_PARAM", "missing tcp router name", "name", "v1"))
		return
	}
	res, err := h.traefikService.DeleteTCPRouter(r.Context(), name)
	if err != nil {
		h.writeJSON(w, http.StatusBadRequest, types.NewErrorResponse[any]("ERR_TCP_ROUTER_DELETE", err.Error(), "name", "v1"))
		return
	}
	h.writeJSON(w, http.StatusOK, types.NewSuccessResponse(res, "v1"))
}

func (h *OrchestratorHandler) HandleListDnsRecords(w http.ResponseWriter, r *http.Request) {
	hostsPath := r.URL.Query().Get("hostsFile")
	records, err := h.dnsService.ListRecords(r.Context(), hostsPath)
	if err != nil {
		h.writeJSON(w, http.StatusInternalServerError, types.NewErrorResponse[any]("ERR_DNS_RECORDS_LIST", err.Error(), "dns", "v1"))
		return
	}
	h.writeJSON(w, http.StatusOK, types.NewSuccessResponse(records, "v1"))
}

func (h *OrchestratorHandler) HandleSyncDnsHosts(w http.ResponseWriter, r *http.Request) {
	var body dnsTypes.SyncOptions
	if r.ContentLength > 0 {
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			h.writeJSON(w, http.StatusBadRequest, types.NewErrorResponse[any]("ERR_INVALID_BODY", err.Error(), "body", "v1"))
			return
		}
	}
	res, err := h.dnsService.SyncHosts(r.Context(), body)
	if err != nil {
		h.writeJSON(w, http.StatusBadRequest, types.NewErrorResponse[any]("ERR_DNS_SYNC", err.Error(), "dns", "v1"))
		return
	}
	h.writeJSON(w, http.StatusOK, types.NewSuccessResponse(res, "v1"))
}

func (h *OrchestratorHandler) HandleCheckDnsResolution(w http.ResponseWriter, r *http.Request) {
	var domains []string
	if r.Method == http.MethodPost && r.ContentLength > 0 {
		var body struct {
			Domains []string `json:"domains"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err == nil {
			domains = body.Domains
		}
	}
	if len(domains) == 0 {
		domainsParam := r.URL.Query().Get("domains")
		if domainsParam != "" {
			domains = strings.Split(domainsParam, ",")
		}
	}

	hostsPath := r.URL.Query().Get("hostsFile")
	results := h.dnsService.CheckResolution(r.Context(), domains, hostsPath)
	h.writeJSON(w, http.StatusOK, types.NewSuccessResponse(results, "v1"))
}


