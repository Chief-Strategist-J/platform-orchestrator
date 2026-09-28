/*
Package services — deep health check runner extension.

ALGORITHM BLUEPRINT:
1. RunDeepHealthChecks: Extends HealthService with deep functional probe execution.
   Reuses the sync.WaitGroup + sync.Mutex fan-out pattern from RunHealthChecks
   (health.service.go) — single source of truth for concurrent probe fan-out.
2. ProbeRegistry: Maps each service name to its ProbeFunc. This is the Open/Closed
   extension point — adding a new service requires only a new entry here, not
   touching any existing RunHealthChecks or probeSingle logic.
3. Config resolution: Merges the caller-supplied DeepProbeConfig list with
   DefaultDeepProbeConfigs; caller values take precedence (override by service name).
4. Report assembly: Produces DeepHealthReport (schema.DeepHealthReport) reusing
   schema.SingleProbeResult — no new result type (DRY rule 14).
5. Span naming: "llmobs.health.deep_check_all" per OTel naming formula
   {domain}.{feature}.{operation} from api.structure.working.rule.md §2.6.
6. Invariants:
   - Every ProbeFunc receives an independent copy of its DeepProbeConfig.
   - Goroutine panics are recovered and converted to failProbe results.
   - Results slice is protected by sync.Mutex; no lock held during I/O.
   - overallHealthy is only false when a Required service fails.
*/
package services

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/Chief-Strategist-J/platform-orchestrator/src/features/health/probes"
	"github.com/Chief-Strategist-J/platform-orchestrator/src/features/health/schema"
	"github.com/Chief-Strategist-J/platform-orchestrator/src/shared/ports"
)

type HealthService struct {
	tracer ports.TracerPort
}

func NewHealthService(tracer ports.TracerPort) *HealthService {
	return &HealthService{
		tracer: tracer,
	}
}

var deepProbeRegistry = map[string]probes.ProbeFunc{
	"alloydb":          probes.ProbeAlloyDB,
	"redis":            probes.ProbeRedis,
	"kafka":            probes.ProbeKafka,
	"clickhouse":       probes.ProbeClickHouse,
	"grafana":          probes.ProbeGrafana,
	"tempo":            probes.ProbeGrafanaTempo,
	"temporal":         probes.ProbeTemporalWorkflow,
	"otel-collector":   probes.ProbeOtelCollector,
	"traefik":          probes.ProbeTraefik,
	"service-registry": probes.ProbeServiceRegistry,
}

func (s *HealthService) RunDeepHealthChecks(ctx context.Context, targetConfigs []schema.DeepProbeConfig) schema.DeepHealthReport {
	_, endSpan := s.tracer.StartSpan(ctx, "llmobs.health.deep_check_all")
	defer endSpan()

	configs := targetConfigs
	if len(configs) == 0 {
		configs = schema.DefaultDeepProbeConfigs("")
	}

	var wg sync.WaitGroup
	var mu sync.Mutex
	var results []schema.SingleProbeResult
	overallHealthy := true

	for _, cfg := range configs {
		probeFn, exists := deepProbeRegistry[cfg.Service]
		if !exists {
			continue
		}
		wg.Add(1)
		go func(fn probes.ProbeFunc, c schema.DeepProbeConfig) {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					mu.Lock()
					results = append(results, schema.SingleProbeResult{
						Service:   c.Service,
						Target:    c.Service,
						Status:    "DOWN",
						LatencyMs: 0,
						Error:     fmt.Sprintf("probe panic: %v", r),
						IsHealthy: false,
					})
					mu.Unlock()
				}
			}()
			res := fn(c)
			mu.Lock()
			results = append(results, res)
			mu.Unlock()
		}(probeFn, cfg)
	}

	wg.Wait()

	healthyCount := 0
	for _, r := range results {
		if r.IsHealthy {
			healthyCount++
		} else {
			overallHealthy = false
		}
	}

	return schema.DeepHealthReport{
		Healthy:      overallHealthy,
		CheckedCount: len(results),
		HealthyCount: healthyCount,
		Results:      results,
		ReportedAt:   time.Now().UTC().Format(time.RFC3339),
	}
}
