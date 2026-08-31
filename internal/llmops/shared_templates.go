package llmops

import "embed"

// Partials included by more than one prompt set. Static text only: every
// including block is a byte-stable cacheable prefix (20260604-164527-d-tac-fah),
// and the shared JSON escaping rule exists so no prompt teaches avoidance where
// JSON requires escaping (20260827-224853-s-tac-giv).
//
//go:embed shared_templates/*.tmpl
var sharedPromptTemplates embed.FS
