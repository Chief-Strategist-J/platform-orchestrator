/*
Package schema defines persistent storage directory contracts and volume layouts.

ALGORITHM BLUEPRINT:
1. Storage Directory Schema: Canonical list of persistent host subdirectories required for data plane services.
2. Environment Keys: Overrides for base volume directories.
3. Invariants:
   - All stateful services must have isolated subdirectories within the primary data folder.
   - Zero inline comments inside function bodies.
*/
package schema

const (
	EnvDataDir     = "LLMOBS_DATA_DIR"
	DefaultDataDir = "data"
)

var DefaultStorageSubdirs = []string{
	"alloydb/data",
	"alloydb/archive",
	"redis/data",
	"kafka/data",
	"clickhouse/data",
	"tempo/data",
	"grafana/data",
}
