/*
Package services provides the public feature facade for external services catalog and health probing.

ALGORITHM BLUEPRINT:
1. Public Facade: Exports domain types, schema definitions, validation rules, and service constructors.
2. Invariants:
   - Zero inline comments inside function bodies.
   - Internal persistence mechanics remain encapsulated.
*/
package services

import (
	"github.com/Chief-Strategist-J/platform-orchestrator/src/features/services/rules"
	"github.com/Chief-Strategist-J/platform-orchestrator/src/features/services/schema"
	"github.com/Chief-Strategist-J/platform-orchestrator/src/features/services/services"
	"github.com/Chief-Strategist-J/platform-orchestrator/src/features/services/types"
	"github.com/Chief-Strategist-J/platform-orchestrator/src/shared/ports"
)

type Service = *services.ServicesService

type ServiceDefinition = schema.ServiceDefinition
type ServiceFilterOptions = schema.ServiceFilterOptions
type HealthProbeResult = types.HealthProbeResult
type DeleteServiceResult = types.DeleteServiceResult
type ServiceSyncResult = types.ServiceSyncResult

func NewService(tracer ports.TracerPort, baseDir string) Service {
	return services.NewServicesService(tracer, baseDir)
}

func Normalize(svc ServiceDefinition) ServiceDefinition {
	return rules.NormalizeServiceDefinition(svc)
}

func Validate(svc ServiceDefinition) error {
	return rules.ValidateServiceDefinition(svc)
}
