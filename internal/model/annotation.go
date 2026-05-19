package model

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

// AnnotationTopic is one topic assignment carried by a kind: annotation entry's
// topics:[] frontmatter list. Two YAML shapes are supported per item:
//
//   - Plain string: "<label>" — applies to all of the annotation's refs.
//     Members is nil in this case.
//   - Mapping: {label: <string>, members: [<id>, ...]} — applies the topic to
//     the listed members specifically. Members must be a subset of the
//     annotation's refs (validated at handler / pre-flight time, not here).
//
// The label string must be a valid TopicPath; parsing happens at consumption
// time (display rendering, topic filter), not here, so an invalid label
// surfaces with a precise error rather than corrupting the entry's other
// fields at load time.
type AnnotationTopic struct {
	Label   string
	Members []string
}

// UnmarshalYAML accepts either a scalar string (plain label form) or a
// mapping with `label` and optional `members` keys.
func (a *AnnotationTopic) UnmarshalYAML(node *yaml.Node) error {
	switch node.Kind {
	case yaml.ScalarNode:
		a.Label = node.Value
		return nil
	case yaml.MappingNode:
		var raw struct {
			Label   string   `yaml:"label"`
			Members []string `yaml:"members,omitempty"`
		}
		if err := node.Decode(&raw); err != nil {
			return fmt.Errorf("annotation topics item: %w", err)
		}
		if raw.Label == "" {
			return fmt.Errorf("annotation topics item: missing required `label`")
		}
		a.Label = raw.Label
		a.Members = raw.Members
		return nil
	default:
		return fmt.Errorf("annotation topics item: expected string or mapping, got node kind %d", node.Kind)
	}
}

// MarshalYAML emits the most compact form: a scalar string when no members
// are set, or a mapping otherwise.
func (a AnnotationTopic) MarshalYAML() (any, error) {
	if len(a.Members) == 0 {
		return a.Label, nil
	}
	return struct {
		Label   string   `yaml:"label"`
		Members []string `yaml:"members"`
	}{Label: a.Label, Members: a.Members}, nil
}

// IsAnnotation reports whether this signal records a structural topic
// annotation. Annotation entries are excluded from catch-up narrative
// rendering and surface only via topic queries.
func (e *Entry) IsAnnotation() bool {
	return e.Type == TypeSignal && e.Kind == KindAnnotation
}

// MembersFor returns the entry IDs the annotation assigns to topic at index i
// — explicit members if set, otherwise the annotation's own refs (since the
// plain-string item form means "applies to all refs").
func (e *Entry) MembersFor(t AnnotationTopic) []string {
	if len(t.Members) > 0 {
		return t.Members
	}
	return RefIDs(e.Refs)
}
