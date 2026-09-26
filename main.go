/*
Package main provides the executable entrypoint for the platform orchestrator package.

ALGORITHM BLUEPRINT:
1. Entrypoint Execution: Delegates command-line arguments to cmd.Execute().
2. Invariants:
   - Exits with operating system status code 1 upon fatal execution failure.
*/
package main

import "github.com/Chief-Strategist-J/platform-orchestrator/src/cmd"

func main() {
	cmd.Execute()
}
