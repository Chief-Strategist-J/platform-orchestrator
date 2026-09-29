/*
Package services provides declarative dashboard source loaders for files and remote URLs with distributed tracing integration.

ALGORITHM BLUEPRINT (DashboardLoaderRegistry):
1. Loader Interface: Defines Matches predicate and Load extraction contract.
2. HTTP Loader: Fetches dashboard JSON payloads from remote endpoints, injecting trace context.
3. File Loader: Reads local JSON files from workspace relative or absolute paths.
4. Extensibility: New source loaders can be registered without modifying calling components.
5. Invariants:
   - Zero inline comments inside function bodies.
   - Non-200 HTTP responses and missing files return descriptive errors.
*/
package services

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Chief-Strategist-J/platform-orchestrator/src/infra/observability"
	"github.com/Chief-Strategist-J/platform-orchestrator/src/shared/ports"
)

type DashboardSourceLoader interface {
	Matches(source string) bool
	Load(ctx context.Context, source string, baseDir string) ([]byte, error)
}

type HTTPDashboardLoader struct {
	client *http.Client
	tracer ports.TracerPort
}

func NewHTTPDashboardLoader(tracer ports.TracerPort) *HTTPDashboardLoader {
	return &HTTPDashboardLoader{
		client: &http.Client{Timeout: 30 * time.Second},
		tracer: tracer,
	}
}

func (l *HTTPDashboardLoader) Matches(source string) bool {
	return strings.HasPrefix(source, "http://") || strings.HasPrefix(source, "https://")
}

func (l *HTTPDashboardLoader) Load(ctx context.Context, source string, _ string) ([]byte, error) {
	ctx, span := l.tracer.StartSpanWithAttributes(ctx, "grafana.dashboard.loader.http", map[string]interface{}{
		"loader.source_url": source,
	})
	defer span.End()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, source, nil)
	if err != nil {
		observability.RecordError(ctx, err)
		return nil, fmt.Errorf("failed creating download request: %w", err)
	}

	observability.InjectHTTPHeaders(ctx, req.Header)
	resp, err := l.client.Do(req)
	if err != nil {
		observability.RecordError(ctx, err)
		return nil, fmt.Errorf("failed downloading dashboard from %s: %w", source, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		statusErr := fmt.Errorf("download from %s failed with status %d", source, resp.StatusCode)
		observability.RecordError(ctx, statusErr)
		return nil, statusErr
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		observability.RecordError(ctx, err)
		return nil, err
	}

	observability.SetAttribute(ctx, "loader.bytes_read", len(data))
	return data, nil
}

type LocalFileDashboardLoader struct {
	tracer ports.TracerPort
}

func NewLocalFileDashboardLoader(tracer ports.TracerPort) *LocalFileDashboardLoader {
	return &LocalFileDashboardLoader{tracer: tracer}
}

func (l *LocalFileDashboardLoader) Matches(_ string) bool {
	return true
}

func (l *LocalFileDashboardLoader) Load(ctx context.Context, source string, baseDir string) ([]byte, error) {
	filePath := source
	if !filepath.IsAbs(filePath) && baseDir != "" {
		filePath = filepath.Join(baseDir, filePath)
	}

	ctx, span := l.tracer.StartSpanWithAttributes(ctx, "grafana.dashboard.loader.file", map[string]interface{}{
		"loader.file_path": filePath,
	})
	defer span.End()

	data, err := os.ReadFile(filePath)
	if err != nil {
		fileErr := fmt.Errorf("failed reading local dashboard file %s: %w", filePath, err)
		observability.RecordError(ctx, fileErr)
		return nil, fileErr
	}

	observability.SetAttribute(ctx, "loader.bytes_read", len(data))
	return data, nil
}

type DashboardLoaderRegistry struct {
	loaders []DashboardSourceLoader
}

func NewDashboardLoaderRegistry(tracer ports.TracerPort) *DashboardLoaderRegistry {
	return &DashboardLoaderRegistry{
		loaders: []DashboardSourceLoader{
			NewHTTPDashboardLoader(tracer),
			NewLocalFileDashboardLoader(tracer),
		},
	}
}

func (r *DashboardLoaderRegistry) Load(ctx context.Context, source string, baseDir string) ([]byte, error) {
	for _, l := range r.loaders {
		if l.Matches(source) {
			return l.Load(ctx, source, baseDir)
		}
	}
	return nil, fmt.Errorf("no matching dashboard loader found for source: %s", source)
}
