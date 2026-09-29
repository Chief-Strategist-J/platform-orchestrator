/*
Package traefik provides lifecycle management, dynamic routing, middleware inspection, and diagnostic probing for the Traefik Ingress Gateway.

ALGORITHM BLUEPRINT:
1. Module Initialization: Exports NewTraefikModule factory.
2. Invariants:
   - Zero inline comments inside function bodies.
*/
package traefik

import (
	"github.com/Chief-Strategist-J/platform-orchestrator/src/features/traefik/services"
	"github.com/Chief-Strategist-J/platform-orchestrator/src/shared/ports"
)

type TraefikModule struct {
	Service *services.TraefikService
}

func NewTraefikModule(tracer ports.TracerPort, baseDir string) *TraefikModule {
	return &TraefikModule{
		Service: services.NewTraefikService(tracer, baseDir),
	}
}
