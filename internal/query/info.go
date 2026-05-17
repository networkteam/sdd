package query

// InfoQuery captures intent to read session framing — participant
// identity, search capability, and configured graph language. No fields
// today; declared as a struct so the surface evolves consistently with
// the rest of the CQRS layer.
type InfoQuery struct{}

// InfoResult is the structured snapshot of session framing.
//
// LocalParticipant surfaces the canonical participant name from
// .sdd/config.local.yaml so agents reading the header see ground truth
// without inferring from entries (which may carry drift). Empty means
// "not configured".
//
// Language surfaces the configured graph language (locale code) from
// .sdd/config.yaml so the /sdd skill knows whether to load a translation
// vocabulary and author entries in the configured language. Empty means
// English (default) — the line is suppressed at render time.
//
// Search renders as `text` when only text mode is available and
// `vector,text` when an embedding provider is configured.
type InfoResult struct {
	LocalParticipant string
	Language         string
	Search           string
}
