/*
Package schema defines data structures and contracts for Grafana dashboard operations.

ALGORITHM BLUEPRINT:
1. DashboardPayload: Schema for creating or updating dashboards.
2. DashboardDetail: Full model returned when fetching a dashboard by UID.
3. DashboardImportOptions: Parameters for importing dashboards from files or remote URLs.
4. Invariants:
   - Zero inline comments inside function bodies.
   - All fields support deterministic JSON marshaling.
*/
package schema

type DashboardPayload struct {
	Dashboard map[string]interface{} `json:"dashboard"`
	FolderID  int64                  `json:"folderId,omitempty"`
	FolderUID string                 `json:"folderUid,omitempty"`
	Message   string                 `json:"message,omitempty"`
	Overwrite bool                   `json:"overwrite"`
}

type DashboardDetail struct {
	Meta      map[string]interface{} `json:"meta"`
	Dashboard map[string]interface{} `json:"dashboard"`
}

type DashboardImportOptions struct {
	SourcePathOrURL string `json:"sourcePathOrUrl"`
	FolderUID       string `json:"folderUid,omitempty"`
	Overwrite       bool   `json:"overwrite"`
	TitleOverride   string `json:"titleOverride,omitempty"`
}
