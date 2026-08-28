package command

// ConfigSetCmd writes one config key to a chosen layer file with
// comment-preserving upsert semantics. The write is validated against the
// target layer's schema before it lands, so a key that does not belong in
// that file (repo_id in the global config, repos in a per-repo file) is
// rejected instead of silently accepted.
type ConfigSetCmd struct {
	// Target layer: "global" (user-global config, created on demand) or
	// "local" (.sdd/config.local.yaml).
	Target string
	// Key is the dotted path (e.g. "participant", "llm.model").
	Key string
	// Value is the scalar as typed by the user; numeric/bool forms are
	// written as their YAML scalar types.
	Value string
}

// ConfigUnsetCmd removes one config key from a chosen layer file, comments
// and sibling keys preserved. A key that is not set in that file is an
// error, never a silent no-op.
type ConfigUnsetCmd struct {
	// Target layer: "global" or "local", as in ConfigSetCmd.
	Target string
	// Key is the dotted path to remove.
	Key string
}
