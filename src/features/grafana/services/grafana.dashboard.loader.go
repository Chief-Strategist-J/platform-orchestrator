/*
Package services provides declarative dashboard source loaders for files and remote URLs.

ALGORITHM BLUEPRINT (DashboardLoaderRegistry):
1. Loader Interface: Defines Matches predicate and Load extraction contract.
2. HTTP Loader: Fetches dashboard JSON payloads from remote endpoints (e.g. Grafana.com community downloads).
3. File Loader: Reads local JSON files from workspace relative or absolute paths.
4. Extensibility: New source loaders (e.g. S3, Git, Vault) can be added by registering a new loader struct.
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
)

type DashboardSourceLoader interface {
	Matches(source string) bool
	Load(ctx context.Context, source string, baseDir string) ([]byte, error)
}

type HTTPDashboardLoader struct {
	client *http.Client
}

func NewHTTPDashboardLoader() *HTTPDashboardLoader {
	return &HTTPDashboardLoader{
		client: &http.Client{Timeout: 30 * time.Second},
	}
}

func (l *HTTPDashboardLoader) Matches(source string) bool {
	return strings.HasPrefix(source, "http://") || strings.HasPrefix(source, "https://")
}

func (l *HTTPDashboardLoader) Load(ctx context.Context, source string, _ string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, source, nil)
	if err != nil {
		return nil, fmt.Errorf("failed creating download request: %w", err)
	}

	resp, err := l.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed downloading dashboard from %s: %w", source, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download from %s failed with status %d", source, resp.StatusCode)
	}

	return io.ReadAll(resp.Body)
}

type LocalFileDashboardLoader struct{}

func NewLocalFileDashboardLoader() *LocalFileDashboardLoader {
	return &LocalFileDashboardLoader{}
}

func (l *LocalFileDashboardLoader) Matches(_ string) bool {
	return true
}

func (l *LocalFileDashboardLoader) Load(_ context.Context, source string, baseDir string) ([]byte, error) {
	filePath := source
	if !filepath.IsAbs(filePath) && baseDir != "" {
		filePath = filepath.Join(baseDir, filePath)
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed reading local dashboard file %s: %w", filePath, err)
	}
	return data, nil
}

type DashboardLoaderRegistry struct {
	loaders []DashboardSourceLoader
}

func NewDashboardLoaderRegistry() *DashboardLoaderRegistry {
	return &DashboardLoaderRegistry{
		loaders: []DashboardSourceLoader{
			NewHTTPDashboardLoader(),
			NewLocalFileDashboardLoader(),
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
