/*
Package rules provides declarative normalization and validation rules for external service definitions and connections.

ALGORITHM BLUEPRINT:
1. NormalizeServiceDefinition: Resolves default category, health check settings, and generates deterministic slugs.
2. ValidateServiceDefinition: Enforces mandatory fields (ID/Name, Type, URL/Host) and port bounds.
3. Invariants:
   - Zero inline comments inside function bodies.
   - Timeout defaults to 5 seconds when <= 0.
   - Names converted to deterministic kebab-case IDs if missing.
*/
package rules

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/Chief-Strategist-J/platform-orchestrator/src/features/services/schema"
)

var nonAlphaNumRegex = regexp.MustCompile(`[^a-z0-9_-]+`)

func NormalizeServiceDefinition(svc schema.ServiceDefinition) schema.ServiceDefinition {
	svc.Name = strings.TrimSpace(svc.Name)
	svc.Type = strings.ToLower(strings.TrimSpace(svc.Type))
	svc.Category = strings.ToLower(strings.TrimSpace(svc.Category))
	svc.URL = strings.TrimSpace(svc.URL)
	svc.Host = strings.TrimSpace(svc.Host)
	svc.Database = strings.TrimSpace(svc.Database)

	if svc.ID == "" {
		slug := strings.ToLower(svc.Name)
		slug = nonAlphaNumRegex.ReplaceAllString(slug, "-")
		svc.ID = strings.Trim(slug, "-")
	}

	if svc.Category == "" {
		switch svc.Type {
		case "postgres", "postgresql", "mysql", "mariadb", "clickhouse", "mongodb", "alloydb":
			svc.Category = "database"
		case "redis", "memcached", "valkey":
			svc.Category = "cache"
		case "tempo", "jaeger", "prometheus", "grafana", "loki", "opentelemetry":
			svc.Category = "monitoring"
		case "openai", "anthropic", "cohere", "groq", "ollama", "bedrock":
			svc.Category = "llm"
		case "qdrant", "pinecone", "milvus", "weaviate", "chroma":
			svc.Category = "vector-db"
		case "kafka", "redpanda", "rabbitmq", "nats":
			svc.Category = "queue"
		default:
			svc.Category = "custom"
		}
	}

	if svc.HealthCheck.Type == "" {
		if svc.URL != "" && (strings.HasPrefix(svc.URL, "http://") || strings.HasPrefix(svc.URL, "https://")) {
			svc.HealthCheck.Type = "http"
		} else {
			svc.HealthCheck.Type = "tcp"
		}
	}
	svc.HealthCheck.Type = strings.ToLower(strings.TrimSpace(svc.HealthCheck.Type))

	if svc.HealthCheck.TimeoutSec <= 0 {
		svc.HealthCheck.TimeoutSec = 5
	}
	if svc.HealthCheck.ExpectedStatus <= 0 && svc.HealthCheck.Type == "http" {
		svc.HealthCheck.ExpectedStatus = 200
	}
	if svc.HealthCheck.Method == "" && svc.HealthCheck.Type == "http" {
		svc.HealthCheck.Method = "GET"
	}

	if svc.CreatedAt.IsZero() {
		svc.CreatedAt = time.Now().UTC()
	}
	svc.UpdatedAt = time.Now().UTC()

	if svc.Metadata == nil {
		svc.Metadata = make(map[string]interface{})
	}
	if svc.Tags == nil {
		svc.Tags = []string{}
	}

	return svc
}

func ValidateServiceDefinition(svc schema.ServiceDefinition) error {
	if strings.TrimSpace(svc.Name) == "" {
		return fmt.Errorf("service name is required and cannot be empty")
	}
	if strings.TrimSpace(svc.Type) == "" {
		return fmt.Errorf("service type is required (e.g. postgres, clickhouse, redis, http, grpc, openai, qdrant, custom)")
	}
	if strings.TrimSpace(svc.URL) == "" && strings.TrimSpace(svc.Host) == "" {
		return fmt.Errorf("either service URL or Host must be provided")
	}
	if svc.Port < 0 || svc.Port > 65535 {
		return fmt.Errorf("port must be between 0 and 65535, received: %d", svc.Port)
	}
	return nil
}
