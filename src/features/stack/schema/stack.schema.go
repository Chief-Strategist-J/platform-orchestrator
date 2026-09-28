/*
Package schema defines data entities, commands, and operational status definitions for stack orchestration.

ALGORITHM BLUEPRINT:
1. Operational Status Constants: Defines canonical lifecycle state strings.
2. StackUpCommand: Entity defining requested profiles, detach flag, environment overrides, and network configurations.
3. StackActionOutcome: Result entity returning status, message, and affected service names.
4. Invariants:
   - Zero inline comments inside function bodies.
*/
package schema

const (
	StatusRunning   = "RUNNING"
	StatusStopped   = "STOPPED"
	StatusRestarted = "RESTARTED"
	StatusError     = "ERROR"

	MsgStackStarted   = "Stack started successfully"
	MsgStackStopped   = "All stack containers stopped"
	MsgStackRestarted = "Stack restarted successfully"
)

type StackUpCommand struct {
	Profiles       []string          `json:"profiles"`
	Detach         bool              `json:"detach"`
	EnvVars        map[string]string `json:"envVars,omitempty"`
	NetworkName    string            `json:"networkName,omitempty"`
	NetworkSubnet  string            `json:"networkSubnet,omitempty"`
	NetworkGateway string            `json:"networkGateway,omitempty"`
}

type StackActionOutcome struct {
	Status         string   `json:"status"`
	Message        string   `json:"message"`
	ActiveServices []string `json:"activeServices"`
}
