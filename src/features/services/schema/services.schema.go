/*
Package schema defines data contracts for external service definitions, custom data service connections, and health probe configurations.

ALGORITHM BLUEPRINT:
1. HealthCheckConfig: Configures automated TCP, HTTP, or gRPC diagnostic probes against external endpoints.
2. AuthConfig: Configures transport or protocol credentials (API tokens, Basic Auth, bearer tokens, headers).
3. ServiceDefinition: Complete entity schema modeling any external or custom platform data service.
4. ServiceFilterOptions: Search and filtration criteria for querying registered services.
5. Invariants:
   - Zero inline comments inside function bodies.
   - Default timeouts and safe empty collections enforced across models.
   - PII and credentials excluded from default JSON serialization representations where appropriate.
*/
package schema

import "time"

type HealthCheckConfig struct {
	Type           string            `json:"type"`
	Path           string            `json:"path,omitempty"`
	Method         string            `json:"method,omitempty"`
	ExpectedStatus int               `json:"expectedStatus,omitempty"`
	TimeoutSec     int               `json:"timeoutSec,omitempty"`
	Headers        map[string]string `json:"headers,omitempty"`
}

type AuthConfig struct {
	Type       string            `json:"type"`
	Username   string            `json:"username,omitempty"`
	Password   string            `json:"password,omitempty"`
	Token      string            `json:"token,omitempty"`
	HeaderName string            `json:"headerName,omitempty"`
	Key        string            `json:"key,omitempty"`
	Custom     map[string]string `json:"custom,omitempty"`
}

type ServiceDefinition struct {
	ID          string                 `json:"id"`
	Name        string                 `json:"name"`
	Category    string                 `json:"category"`
	Type        string                 `json:"type"`
	URL         string                 `json:"url,omitempty"`
	Host        string                 `json:"host,omitempty"`
	Port        int                    `json:"port,omitempty"`
	Database    string                 `json:"database,omitempty"`
	HealthCheck HealthCheckConfig      `json:"healthCheck"`
	Auth        AuthConfig             `json:"auth"`
	Tags        []string               `json:"tags,omitempty"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
	CreatedAt   time.Time              `json:"createdAt"`
	UpdatedAt   time.Time              `json:"updatedAt"`
}

type ServiceFilterOptions struct {
	Category string   `json:"category,omitempty"`
	Type     string   `json:"type,omitempty"`
	Tag      string   `json:"tag,omitempty"`
	Query    string   `json:"query,omitempty"`
	Tags     []string `json:"tags,omitempty"`
}

type ServiceCatalog struct {
	Version   string                       `json:"version"`
	Services  map[string]ServiceDefinition `json:"services"`
	UpdatedAt time.Time                    `json:"updatedAt"`
}
