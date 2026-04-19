package command

// WriteSchemaMetaCmd captures intent to write .sdd/meta.json on initial
// graph setup. The handler is a no-op when the file already exists — both
// graph_schema_version (bumped by migrations only) and minimum_version
// (set once at creation, preserved thereafter) are write-once semantics.
type WriteSchemaMetaCmd struct {
	// SDDDir is the absolute path to the .sdd/ directory.
	SDDDir string

	// SchemaVersion is the graph schema version to stamp into a newly
	// written meta.json. Ignored when the file already exists.
	SchemaVersion int

	// MinimumVersion is the oldest sdd binary permitted to write to this
	// graph. Written only when the meta file does not yet exist; passing
	// nil skips writing the field (dev builds use this to avoid pinning a
	// floor during local development).
	MinimumVersion *string

	// OnWritten fires after a fresh meta.json has been written with the
	// absolute path as argument. Does not fire for no-op runs against an
	// existing file.
	OnWritten func(path string)

	// OnPreserved fires when meta.json already existed and was left
	// untouched, carrying the absolute path.
	OnPreserved func(path string)
}
