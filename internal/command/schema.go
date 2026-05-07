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

// BumpMinimumVersionCmd captures intent to raise .sdd/meta.json's
// minimum_version field to the running binary's version. Used by
// `sdd init --bump` to lock contributors out of older binaries after a
// breaking change has shipped — the inverse of the regular write path
// (which preserves whatever was set at graph creation).
//
// The handler refuses dev builds: only released binaries are permitted
// to raise the floor, since otherwise local development could pin a
// graph to a non-released version no contributor outside the dev tree
// could match.
type BumpMinimumVersionCmd struct {
	// SDDDir is the absolute path to the .sdd/ directory.
	SDDDir string

	// BinaryVersion is the running sdd binary's version. Must be a
	// released semver — dev builds are rejected.
	BinaryVersion string

	// OnBumped fires when the field was updated, carrying the previous
	// value (empty string when no minimum was recorded) and the new value.
	OnBumped func(previous, current string)

	// OnUnchanged fires when the binary version equals the recorded
	// minimum (or is lower than it — a no-op rather than a regression).
	OnUnchanged func(current string)
}
