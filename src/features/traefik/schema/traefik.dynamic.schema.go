/*
Package schema defines data contracts for the full dynamic.yml file model.

ALGORITHM BLUEPRINT:
1. DynamicConfiguration: Root model representing the complete Traefik dynamic configuration file.
2. HTTPConfiguration: Container for HTTP routers, services, middlewares, and serversTransports.
3. TCPConfiguration: Container for TCP routers, services, and middlewares.
4. TLSConfiguration: Container for certificates, stores, and options.
5. Invariants:
   - Zero inline comments inside function bodies.
   - YAML and JSON tag parity for disk persistence and wire serialization.
*/
package schema

type DynamicConfiguration struct {
	TLS  *TLSConfig         `json:"tls,omitempty" yaml:"tls,omitempty"`
	HTTP *HTTPConfiguration `json:"http,omitempty" yaml:"http,omitempty"`
	TCP  *TCPConfiguration  `json:"tcp,omitempty" yaml:"tcp,omitempty"`
}

type TLSConfig struct {
	Stores       map[string]interface{} `json:"stores,omitempty" yaml:"stores,omitempty"`
	Options      map[string]interface{} `json:"options,omitempty" yaml:"options,omitempty"`
	Certificates []CertificateConfig    `json:"certificates,omitempty" yaml:"certificates,omitempty"`
}

type CertificateConfig struct {
	CertFile string   `json:"certFile" yaml:"certFile"`
	KeyFile  string   `json:"keyFile" yaml:"keyFile"`
	Stores   []string `json:"stores,omitempty" yaml:"stores,omitempty"`
}

type HTTPConfiguration struct {
	Routers           map[string]HTTPRouterDefinition `json:"routers,omitempty" yaml:"routers,omitempty"`
	Services          map[string]ServiceDefinition    `json:"services,omitempty" yaml:"services,omitempty"`
	Middlewares       map[string]MiddlewareDefinition `json:"middlewares,omitempty" yaml:"middlewares,omitempty"`
	ServersTransports map[string]interface{}          `json:"serversTransports,omitempty" yaml:"serversTransports,omitempty"`
}

type TCPConfiguration struct {
	Routers     map[string]TCPRouterDefinition `json:"routers,omitempty" yaml:"routers,omitempty"`
	Services    map[string]ServiceDefinition   `json:"services,omitempty" yaml:"services,omitempty"`
	Middlewares map[string]interface{}         `json:"middlewares,omitempty" yaml:"middlewares,omitempty"`
}
