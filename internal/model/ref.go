package model

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// RefKind classifies the semantic relationship a reference describes.
// The closed vocabulary is enforced at capture (pre-flight uses
// IsCapturableRefKind); legacy entries with bare-string refs parse as
// RefKindUnknown for traversal compatibility but cannot be authored anew.
type RefKind string

const (
	RefKindGrounds   RefKind = "grounds"    // anchors to standing structure
	RefKindBuildsOn  RefKind = "builds-on"  // extends prior lineage (forward continuation)
	RefKindRefines   RefKind = "refines"    // sharpens, narrows, or clarifies an active target in place — the augmenting-directive pattern (d-prc-9ti)
	RefKindAddresses RefKind = "addresses"  // responds to a gap, question, or signal
	RefKindSurfaces  RefKind = "surfaces"   // created or discovered the referenced entry during this work
	RefKindEvidence  RefKind = "evidence"   // empirical observation supporting the claim
	RefKindDependsOn RefKind = "depends-on" // functional prerequisite
	RefKindRelated   RefKind = "related"    // parallel sibling, no other axis fits
	RefKindUnknown   RefKind = "unknown"    // legacy bare-string fallback — read-only, not authored
)

var refKinds = map[RefKind]bool{
	RefKindGrounds:   true,
	RefKindBuildsOn:  true,
	RefKindRefines:   true,
	RefKindAddresses: true,
	RefKindSurfaces:  true,
	RefKindEvidence:  true,
	RefKindDependsOn: true,
	RefKindRelated:   true,
	RefKindUnknown:   true,
}

// IsValidRefKind reports whether k is one of the closed-set kind values,
// including the legacy unknown sentinel. Parse-time and graph-traversal
// callers use this; capture-time callers (pre-flight) use IsCapturableRefKind
// to additionally reject unknown.
func IsValidRefKind(k RefKind) bool {
	return refKinds[k]
}

// IsCapturableRefKind reports whether k is a valid kind for a new entry.
// Excludes RefKindUnknown — that sentinel only exists for legacy bare-string
// refs already on disk and is rejected at capture pre-flight.
func IsCapturableRefKind(k RefKind) bool {
	return refKinds[k] && k != RefKindUnknown
}

// RefKindValues returns the capturable kind vocabulary in display order.
// Used to render the closed set in error messages and documentation.
func RefKindValues() []RefKind {
	return []RefKind{
		RefKindGrounds,
		RefKindBuildsOn,
		RefKindRefines,
		RefKindAddresses,
		RefKindSurfaces,
		RefKindEvidence,
		RefKindDependsOn,
		RefKindRelated,
	}
}

// Ref is one entry reference with its semantic kind and optional inline
// description. The object form ({id, kind, desc?}) is the canonical on-disk
// representation; bare-string entries (legacy) parse with Kind = RefKindUnknown
// so existing graphs keep working while new captures carry the metadata.
type Ref struct {
	ID   string
	Kind RefKind
	Desc string
}

// UnmarshalYAML accepts either a scalar string (legacy form; Kind defaults
// to RefKindUnknown) or a mapping with `id`, `kind`, and optional `desc`
// keys. Object form requires both `id` and `kind`; an invalid kind value
// fails to parse so malformed entries don't silently enter the graph.
func (r *Ref) UnmarshalYAML(node *yaml.Node) error {
	switch node.Kind {
	case yaml.ScalarNode:
		if strings.TrimSpace(node.Value) == "" {
			return fmt.Errorf("ref: empty string")
		}
		r.ID = node.Value
		r.Kind = RefKindUnknown
		return nil
	case yaml.MappingNode:
		var raw struct {
			ID   string `yaml:"id"`
			Kind string `yaml:"kind"`
			Desc string `yaml:"desc,omitempty"`
		}
		if err := node.Decode(&raw); err != nil {
			return fmt.Errorf("ref object: %w", err)
		}
		if raw.ID == "" {
			return fmt.Errorf("ref object: missing required `id`")
		}
		if raw.Kind == "" {
			return fmt.Errorf("ref object %q: missing required `kind`", raw.ID)
		}
		k := RefKind(raw.Kind)
		if !IsValidRefKind(k) {
			return fmt.Errorf("ref object %q: invalid kind %q (expected one of: %s)", raw.ID, raw.Kind, refKindList())
		}
		r.ID = raw.ID
		r.Kind = k
		r.Desc = raw.Desc
		return nil
	default:
		return fmt.Errorf("ref: expected string or mapping, got node kind %d", node.Kind)
	}
}

// MarshalYAML emits the canonical object form with id, kind, desc ordering.
// Bare-string output is never produced — writes always use object form even
// when carrying a legacy ref with Kind = RefKindUnknown (round-trip case).
func (r Ref) MarshalYAML() (any, error) {
	node := &yaml.Node{Kind: yaml.MappingNode}
	node.Content = append(node.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Value: "id"},
		&yaml.Node{Kind: yaml.ScalarNode, Value: r.ID},
		&yaml.Node{Kind: yaml.ScalarNode, Value: "kind"},
		&yaml.Node{Kind: yaml.ScalarNode, Value: string(r.Kind)},
	)
	if r.Desc != "" {
		node.Content = append(node.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Value: "desc"},
			&yaml.Node{Kind: yaml.ScalarNode, Value: r.Desc},
		)
	}
	return node, nil
}

// RefIDs extracts the bare ID strings from a slice of Refs. Traversal,
// summary rendering, and validator code that only cares about identity
// uses this helper instead of inlining the loop.
func RefIDs(refs []Ref) []string {
	out := make([]string, len(refs))
	for i, r := range refs {
		out[i] = r.ID
	}
	return out
}

// idOnlyList enforces bare-string sequence shape on a YAML field. The
// `closes` and `supersedes` fields use this — those relationships have
// uniform mechanical meaning and don't carry per-ref metadata, so writing
// object form there is rejected with a clear error pointing at refs.
type idOnlyList []string

func (l *idOnlyList) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind != yaml.SequenceNode {
		return fmt.Errorf("expected list of entry IDs, got node kind %d", node.Kind)
	}
	for _, item := range node.Content {
		if item.Kind != yaml.ScalarNode {
			return fmt.Errorf("object form not supported here — only `refs` accepts per-ref kind/desc metadata; use a bare-string ID")
		}
		*l = append(*l, item.Value)
	}
	return nil
}

func refKindList() string {
	values := RefKindValues()
	parts := make([]string, len(values))
	for i, k := range values {
		parts[i] = string(k)
	}
	return strings.Join(parts, ", ")
}
