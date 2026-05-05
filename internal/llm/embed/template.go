package embed

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

// applyTemplate substitutes the {text} placeholder with each input. An
// empty template short-circuits to the unchanged slice. The placeholder
// is replaced everywhere it occurs (string-substitution, not formatted
// argument), so users can repeat {text} inside the template without
// special handling.
func applyTemplate(tmpl string, texts []string) []string {
	if tmpl == "" {
		return texts
	}
	out := make([]string, len(texts))
	for i, t := range texts {
		out[i] = strings.ReplaceAll(tmpl, "{text}", t)
	}
	return out
}

// appendDocTemplateHash appends a short hash of the document template to
// the embedder's base fingerprint when the template is non-empty.
// Returns base unchanged when template is empty (untemplated models keep
// their familiar fingerprint shape). Six hex chars is enough to
// disambiguate among templates a user might iterate on without bloating
// fingerprints to obscure-on-the-eye lengths.
func appendDocTemplateHash(base, documentTemplate string) string {
	if documentTemplate == "" {
		return base
	}
	h := sha256.Sum256([]byte(documentTemplate))
	return base + "/d:" + hex.EncodeToString(h[:3])
}
