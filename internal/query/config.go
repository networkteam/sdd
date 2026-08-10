package query

// EffectiveConfigQuery asks for the fully resolved config overlay with
// per-value provenance: which layer — global, project (committed), local,
// or a baked default — supplied each effective value. SDDDir empty means
// the caller is outside an sdd repo: global settings and defaults only.
type EffectiveConfigQuery struct {
	SDDDir string
	// Key restricts the result to one dotted key (e.g. "llm.model").
	// Empty returns every effective entry.
	Key string
}

// ConfigEntry is one row of the effective-config view. Secret entries carry
// a masked Value on every surface.
type ConfigEntry struct {
	Key    string
	Value  string
	Source string
	Secret bool
}

// EffectiveConfigResult carries the resolved entries in schema order.
type EffectiveConfigResult struct {
	Entries []ConfigEntry
}

// UnknownConfigKeysQuery asks which keys in the config files this binary
// reads it does not recognise. SDDDir empty checks the user-global layer
// alone.
type UnknownConfigKeysQuery struct {
	SDDDir string
}

// UnknownConfigKey is one unrecognised key and the file carrying it.
type UnknownConfigKey struct {
	File string
	Key  string
}

// UnknownConfigKeysResult lists the unrecognised keys across every layer,
// global first. Empty means every key in every layer is understood.
type UnknownConfigKeysResult struct {
	Keys []UnknownConfigKey
}
