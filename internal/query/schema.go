package query

import "github.com/networkteam/sdd/internal/model"

// SchemaStatusQuery captures intent to evaluate the current binary against
// the graph's .sdd/meta.json compatibility fields.
//
// The dispatcher-level write-gate runs this query before any command that
// mutates the graph; read commands bypass the query entirely.
type SchemaStatusQuery struct {
	// SDDDir is the absolute path to the .sdd/ directory. The query resolves
	// meta.json relative to this path.
	SDDDir string

	// BinaryVersion is the running sdd binary's version string (semver for
	// releases, "dev" or similar for local builds). Dev builds bypass both
	// compatibility gates.
	BinaryVersion string

	// BinarySchemaVersion is the schema version the binary understands. The
	// call site passes model.CurrentGraphSchemaVersion.
	BinarySchemaVersion int
}

// SchemaStatusResult reports whether the binary is permitted to proceed and,
// when !Compatibility.Compatible, why.
type SchemaStatusResult struct {
	// MetaExists is false when .sdd/meta.json is not present on disk. In
	// that case Meta is nil and Compatibility is considered compatible (an
	// uninitialized graph will be set up on the next init).
	MetaExists bool

	// Meta is the parsed contents of .sdd/meta.json when MetaExists is true.
	Meta *model.SchemaMeta

	// Compatibility is the result of checking Meta against the binary's
	// version and schema version. Always populated (with Compatible = true)
	// when MetaExists is false.
	Compatibility model.CompatibilityResult
}
