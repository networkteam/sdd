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
