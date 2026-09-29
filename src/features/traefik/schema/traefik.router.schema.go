/*
Package schema defines data contracts for Traefik routers, services, and middlewares.

ALGORITHM BLUEPRINT:
1. HTTPRouterDefinition: Schema for dynamic HTTP ingress routing rules.
2. TCPRouterDefinition: Schema for raw TCP/gRPC routing rules.
3. ServiceDefinition: Load balancer server coordinate models.
4. MiddlewareDefinition: Security, rate limiting, and circuit breaking middleware models.
5. Invariants:
   - Zero inline comments inside function bodies.
   - Deterministic JSON marshaling for all fields.
*/
package schema

type HTTPRouterDefinition struct {
	Name        string                 `json:"name" yaml:"name,omitempty"`
	Rule        string                 `json:"rule" yaml:"rule"`
	Service     string                 `json:"service" yaml:"service"`
	EntryPoints []string               `json:"entryPoints" yaml:"entryPoints,omitempty"`
	Middlewares []string               `json:"middlewares,omitempty" yaml:"middlewares,omitempty"`
	Priority    int                    `json:"priority,omitempty" yaml:"priority,omitempty"`
	TLS         map[string]interface{} `json:"tls,omitempty" yaml:"tls,omitempty"`
	Status      string                 `json:"status,omitempty" yaml:"status,omitempty"`
	Using       []string               `json:"using,omitempty" yaml:"using,omitempty"`
	Err         []string               `json:"error,omitempty" yaml:"error,omitempty"`
}

type TCPRouterDefinition struct {
	Name        string                 `json:"name" yaml:"name,omitempty"`
	Rule        string                 `json:"rule" yaml:"rule"`
	Service     string                 `json:"service" yaml:"service"`
	EntryPoints []string               `json:"entryPoints" yaml:"entryPoints,omitempty"`
	TLS         map[string]interface{} `json:"tls,omitempty" yaml:"tls,omitempty"`
	Status      string                 `json:"status,omitempty" yaml:"status,omitempty"`
	Err         []string               `json:"error,omitempty" yaml:"error,omitempty"`
}

type LoadBalancerServer struct {
	URL     string `json:"url,omitempty" yaml:"url,omitempty"`
	Address string `json:"address,omitempty" yaml:"address,omitempty"`
}

type ServiceDefinition struct {
	Name         string                 `json:"name" yaml:"name,omitempty"`
	Type         string                 `json:"type,omitempty" yaml:"type,omitempty"`
	Status       string                 `json:"status,omitempty" yaml:"status,omitempty"`
	LoadBalancer *LoadBalancerConfig    `json:"loadBalancer,omitempty" yaml:"loadBalancer,omitempty"`
	Err          []string               `json:"error,omitempty" yaml:"error,omitempty"`
}

type LoadBalancerConfig struct {
	Servers          []LoadBalancerServer   `json:"servers,omitempty" yaml:"servers,omitempty"`
	PassHostHeader   *bool                  `json:"passHostHeader,omitempty" yaml:"passHostHeader,omitempty"`
	ServersTransport string                 `json:"serversTransport,omitempty" yaml:"serversTransport,omitempty"`
	HealthCheck      map[string]interface{} `json:"healthCheck,omitempty" yaml:"healthCheck,omitempty"`
}

type MiddlewareDefinition struct {
	Name   string                 `json:"name" yaml:"name,omitempty"`
	Type   string                 `json:"type,omitempty" yaml:"type,omitempty"`
	Status string                 `json:"status,omitempty" yaml:"status,omitempty"`
	Config map[string]interface{} `json:"config,omitempty" yaml:"config,omitempty"`
	Err    []string               `json:"error,omitempty" yaml:"error,omitempty"`
}
