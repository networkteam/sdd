package model

import (
	"fmt"
	"strings"
	"unicode"
)

// TopicPath is a hierarchical topic label expressed as path components.
// Internal representation is []string of components (e.g. "infrastructure/cli"
// is []string{"infrastructure", "cli"}); I/O representation joins components
// with "/". Comparison is case-insensitive on each component, but the original
// casing is preserved on the value so first-seen casing can win as canonical
// for display.
//
// Component validation: each component matches [\p{L}\p{N}\-]+ (Unicode letter
// or number plus hyphen). Empty components — leading, trailing, or consecutive
// "/" — are rejected.
type TopicPath struct {
	// Components are the path elements in order, preserving original casing.
	// Length is always >= 1 for a valid TopicPath; the zero value is invalid.
	Components []string
}

// ParseTopicPath parses a "/"-joined topic path string into a TopicPath.
// Empty input, leading/trailing "/", consecutive "//", or components that
// violate the component-character rule return an error.
func ParseTopicPath(s string) (TopicPath, error) {
	if s == "" {
		return TopicPath{}, fmt.Errorf("topic path: empty")
	}
	parts := strings.Split(s, "/")
	for i, p := range parts {
		if p == "" {
			return TopicPath{}, fmt.Errorf("topic path %q: empty component at position %d", s, i)
		}
		if err := validateTopicComponent(p); err != nil {
			return TopicPath{}, fmt.Errorf("topic path %q: %w", s, err)
		}
	}
	return TopicPath{Components: parts}, nil
}

// validateTopicComponent reports whether a single path component conforms to
// the [\p{L}\p{N}\-]+ rule. Returns a descriptive error on failure.
func validateTopicComponent(c string) error {
	if c == "" {
		return fmt.Errorf("empty component")
	}
	for _, r := range c {
		if r == '-' {
			continue
		}
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			continue
		}
		return fmt.Errorf("component %q: invalid character %q (allowed: Unicode letter, digit, or '-')", c, r)
	}
	return nil
}

// String returns the "/"-joined I/O form. Original casing preserved.
func (t TopicPath) String() string {
	return strings.Join(t.Components, "/")
}

// IsZero reports whether t is the zero value (no components).
func (t TopicPath) IsZero() bool {
	return len(t.Components) == 0
}

// Equal reports whether two paths match component-wise, case-insensitively.
// Length must be identical; comparison is per-component.
func (t TopicPath) Equal(o TopicPath) bool {
	if len(t.Components) != len(o.Components) {
		return false
	}
	for i := range t.Components {
		if !strings.EqualFold(t.Components[i], o.Components[i]) {
			return false
		}
	}
	return true
}

// HasPrefix reports whether t starts with prefix component-wise, case-
// insensitively. A path is a prefix of itself. Used by the topic(L) filter:
// `topic("UX")` matches `UX`, `UX/CLI`, `UX/CLI/Status`; does NOT match
// `UXTesting` (because comparison is component-wise, not raw-string).
func (t TopicPath) HasPrefix(prefix TopicPath) bool {
	if len(prefix.Components) > len(t.Components) {
		return false
	}
	for i := range prefix.Components {
		if !strings.EqualFold(t.Components[i], prefix.Components[i]) {
			return false
		}
	}
	return true
}

// FoldKey returns a comparable key for case-insensitive path deduplication.
func (t TopicPath) FoldKey() string {
	parts := make([]string, len(t.Components))
	for i, c := range t.Components {
		parts[i] = caseFoldKey(c)
	}
	return strings.Join(parts, "/")
}

// caseFoldKey returns one representative for each strings.EqualFold class.
func caseFoldKey(s string) string {
	return strings.Map(func(r rune) rune {
		canonical := r
		for folded := unicode.SimpleFold(r); folded != r; folded = unicode.SimpleFold(folded) {
			if folded < canonical {
				canonical = folded
			}
		}
		return canonical
	}, s)
}

// CanonicalizeTopicPaths walks paths in order and returns a deduplicated
// slice where the first-seen casing of each fold-key wins. Used by display
// rendering and by graph traversal merging inline topics with annotation
// memberships.
func CanonicalizeTopicPaths(paths []TopicPath) []TopicPath {
	if len(paths) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(paths))
	var out []TopicPath
	for _, p := range paths {
		if p.IsZero() {
			continue
		}
		k := p.FoldKey()
		if _, ok := seen[k]; ok {
			continue
		}
		seen[k] = struct{}{}
		out = append(out, p)
	}
	return out
}
