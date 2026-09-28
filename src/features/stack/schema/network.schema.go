/*
Package schema defines Docker network specifications and Compose file topology contracts.

ALGORITHM BLUEPRINT:
1. Docker Network Configuration: Environment keys and fallback defaults for cluster networking.
2. Compose File Specifications: Filename contracts for base, stateless, production, and ingress overlays.
3. Invariants:
   - Zero inline comments inside function bodies.
*/
package schema

const (
	DefaultComposeFile           = "docker-compose.yml"
	DefaultStatelessComposeFile  = "docker-compose.stateless.yml"
	DefaultProdComposeFile       = "docker-compose.prod.yml"
	DefaultCloudflareComposeFile = "docker-compose.cloudflare.yml"
)

const (
	EnvDockerNetworkName        = "LLMOBS_NETWORK_NAME"
	DefaultDockerNetworkName    = "llmobs-network"
	EnvDockerNetworkSubnet      = "LLMOBS_NETWORK_SUBNET"
	DefaultDockerNetworkSubnet  = "172.28.0.0/16"
	EnvDockerNetworkGateway     = "LLMOBS_NETWORK_GATEWAY"
	DefaultDockerNetworkGateway = "172.28.0.1"
)
