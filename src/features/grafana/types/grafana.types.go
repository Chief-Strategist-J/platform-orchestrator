/*
Package types defines domain and transport models for Grafana datasource, dashboard, and alert operations.

ALGORITHM BLUEPRINT:
1. ClientOptions: Transport authentication, endpoint resolution, and HTTP timeout controls.
2. DatasourceHealthResult: Standardized health probe result from Grafana /api/datasources/uid/:uid/health.
3. DashboardDomainModels: Types for search, details, export, and mutations.
4. AlertDomainModels: Unified alerting rules and notification contact points.
5. Invariants:
   - Zero inline comments inside function bodies.
   - PII and credentials must never leak into log strings or public error targets.
   - All models support deterministic JSON serialization.
*/
package types

import "time"

type ClientOptions struct {
	GrafanaURL string        `json:"grafanaUrl"`
	Username   string        `json:"username"`
	Password   string        `json:"password"`
	Timeout    time.Duration `json:"timeout"`
}

type DatasourceHealthResult struct {
	UID       string  `json:"uid"`
	Name      string  `json:"name,omitempty"`
	Status    string  `json:"status"`
	Message   string  `json:"message"`
	LatencyMs float64 `json:"latencyMs"`
	IsHealthy bool    `json:"isHealthy"`
}

type DeleteDatasourceResult struct {
	UID     string `json:"uid"`
	Message string `json:"message"`
	Success bool   `json:"success"`
}

type DashboardSearchResult struct {
	ID          int64    `json:"id"`
	UID         string   `json:"uid"`
	Title       string   `json:"title"`
	URI         string   `json:"uri"`
	URL         string   `json:"url"`
	Slug        string   `json:"slug"`
	Type        string   `json:"type"`
	Tags        []string `json:"tags"`
	IsStarred   bool     `json:"isStarred"`
	FolderID    int64    `json:"folderId,omitempty"`
	FolderUID   string   `json:"folderUid,omitempty"`
	FolderTitle string   `json:"folderTitle,omitempty"`
}

type DashboardSaveResult struct {
	ID      int64   `json:"id"`
	UID     string  `json:"uid"`
	URL     string  `json:"url"`
	Status  string  `json:"status"`
	Version int     `json:"version"`
	Slug    string  `json:"slug"`
	Message string  `json:"message,omitempty"`
	Latency float64 `json:"latencyMs,omitempty"`
}

type DashboardDeleteResult struct {
	Title   string `json:"title"`
	Message string `json:"message"`
	Success bool   `json:"success"`
}

type AlertOperationResult struct {
	UID       string  `json:"uid"`
	Title     string  `json:"title,omitempty"`
	Message   string  `json:"message"`
	Status    string  `json:"status"`
	Success   bool    `json:"success"`
	LatencyMs float64 `json:"latencyMs,omitempty"`
}
