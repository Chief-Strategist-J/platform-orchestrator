/*
Package dns provides domain discovery, host file orchestration, and connectivity checks for platform service routing.

ALGORITHM BLUEPRINT:
1. Module Initialization: Exports NewDNSModule factory.
2. Invariants:
   - Zero inline comments inside function bodies.
*/
package dns

import (
	"github.com/Chief-Strategist-J/platform-orchestrator/src/features/dns/services"
	"github.com/Chief-Strategist-J/platform-orchestrator/src/shared/ports"
)

type DNSModule struct {
	Service      *services.DNSService
	HostsService *services.DNSHostsService
	ProbeService *services.DNSProbeService
}

func NewDNSModule(tracer ports.TracerPort, baseDir string) *DNSModule {
	return &DNSModule{
		Service:      services.NewDNSService(tracer, baseDir),
		HostsService: services.NewDNSHostsService(tracer),
		ProbeService: services.NewDNSProbeService(tracer),
	}
}
