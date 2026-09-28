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
	gdprSchema "github.com/Chief-Strategist-J/platform-orchestrator/src/features/gdpr/schema"
	gdprService "github.com/Chief-Strategist-J/platform-orchestrator/src/features/gdpr/services"
	healthSchema "github.com/Chief-Strategist-J/platform-orchestrator/src/features/health/schema"
	healthService "github.com/Chief-Strategist-J/platform-orchestrator/src/features/health/services"
	portsService "github.com/Chief-Strategist-J/platform-orchestrator/src/features/ports/services"
	prereqsService "github.com/Chief-Strategist-J/platform-orchestrator/src/features/prereqs/services"
	scaleSchema "github.com/Chief-Strategist-J/platform-orchestrator/src/features/scale/schema"
	scaleService "github.com/Chief-Strategist-J/platform-orchestrator/src/features/scale/services"
	setupSchema "github.com/Chief-Strategist-J/platform-orchestrator/src/features/setup/schema"
	setupService "github.com/Chief-Strategist-J/platform-orchestrator/src/features/setup/services"
	stackSchema "github.com/Chief-Strategist-J/platform-orchestrator/src/features/stack/schema"
	stackService "github.com/Chief-Strategist-J/platform-orchestrator/src/features/stack/services"
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
