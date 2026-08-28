package model

import (
	"bytes"
	"fmt"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"
)

// SetYAMLField sets the scalar at dotted path to value in an existing YAML
// document, returning new bytes. Unknown keys, user comments, and the
// order of sibling keys are preserved — only the targeted scalar (or the
// minimal set of missing ancestors) is touched. An empty input is treated
// as an empty document; the result is a fresh mapping containing just the
// target key chain.
//
// Value may be anything yaml.Node.Encode accepts as a scalar (string, int,
// bool, nil, etc.). Sequences and nested mappings are out of scope —
// targeting a non-scalar leaf (or a slice/map value) is rejected. Use
// SetYAMLSequence for list-valued fields.
//
// Path uses dotted segments (e.g. "participant", "llm.provider"). No
// support for array indexing or filter expressions — keep paths flat.
// Empty segments (leading/trailing/consecutive dots) are rejected.
//
// The port of the JSONPath-capable vignet patcher was intentional: SDD's
// settings are flat config, so the simpler dotted-path form covers the
// need without the extra dependency.
func SetYAMLField(existing []byte, path string, value any) ([]byte, error) {
	return patchYAML(existing, path, func(target *yaml.Node) error {
		if target.Kind != yaml.ScalarNode {
			return fmt.Errorf("target at %q is a %s, not a scalar", path, kindName(target.Kind))
		}
		var encoded yaml.Node
		if err := encoded.Encode(value); err != nil {
			return fmt.Errorf("encoding value: %w", err)
		}
		if encoded.Kind != yaml.ScalarNode {
			return fmt.Errorf("value for %q is not scalar", path)
		}
		replaceNodeValue(target, &encoded)
		return nil
	})
}

// SetYAMLValue sets the value at dotted path to anything yaml.Node.Encode
// accepts — including a sequence of mappings, the shape the global config's
// `repos` list needs. The general form SetYAMLField and SetYAMLSequence
// constrain; it preserves the same way they do.
func SetYAMLValue(existing []byte, path string, value any) ([]byte, error) {
	return patchYAML(existing, path, func(target *yaml.Node) error {
		var encoded yaml.Node
		if err := encoded.Encode(value); err != nil {
			return fmt.Errorf("encoding value: %w", err)
		}
		replaceNodeValue(target, &encoded)
		return nil
	})
}

// SetYAMLSequence sets the value at dotted path to a flow-style sequence of
// string scalars (e.g. "supported_agents: [claude, codex]") in an existing
// YAML document. Like SetYAMLField it preserves unknown keys, comments, and
// sibling order, touching only the target. An existing scalar or sequence at
// the leaf is replaced wholesale; a non-mapping intermediate segment is
// rejected. Empty input yields a fresh mapping holding just the target.
//
// Flow style matches how FormatConfig renders the same field on a fresh init,
// so a sequence upsert and a fresh write produce the same on-disk shape.
func SetYAMLSequence(existing []byte, path string, values []string) ([]byte, error) {
	return patchYAML(existing, path, func(target *yaml.Node) error {
		items := make([]*yaml.Node, 0, len(values))
		for _, v := range values {
			items = append(items, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: v})
		}
		replaceNodeValue(target, &yaml.Node{Kind: yaml.SequenceNode, Style: yaml.FlowStyle, Content: items})
		return nil
	})
}

// DeleteYAMLField removes the key at dotted path from an existing YAML
// document, returning new bytes. Comments and sibling order elsewhere are
// preserved. A path that does not resolve is an error, so a caller can
// report "not set" truthfully; an emptied parent mapping stays in place.
func DeleteYAMLField(existing []byte, path string) ([]byte, error) {
	segments, err := splitYAMLPath(path)
	if err != nil {
		return nil, err
	}
	root, err := parseYAMLRoot(existing)
	if err != nil {
		return nil, err
	}
	current := root.Content[0]
	for i, segment := range segments {
		if current.Kind != yaml.MappingNode {
			return nil, fmt.Errorf("%q is not set", path)
		}
		idx := -1
		for j := 0; j+1 < len(current.Content); j += 2 {
			if current.Content[j].Value == segment {
				idx = j
				break
			}
		}
		if idx < 0 {
			return nil, fmt.Errorf("%q is not set", path)
		}
		if i == len(segments)-1 {
			current.Content = append(current.Content[:idx], current.Content[idx+2:]...)
			return encodeYAML(root)
		}
		current = current.Content[idx+1]
	}
	return nil, fmt.Errorf("unreachable: path not resolved")
}

// patchYAML is the single splice every setter runs through, and the reason
// they all preserve: only the one node mutate is handed gets rewritten.
func patchYAML(existing []byte, path string, mutate func(target *yaml.Node) error) ([]byte, error) {
	segments, err := splitYAMLPath(path)
	if err != nil {
		return nil, err
	}
	root, err := parseYAMLRoot(existing)
	if err != nil {
		return nil, err
	}
	target, err := descendToLeaf(root.Content[0], segments)
	if err != nil {
		return nil, err
	}
	if err := mutate(target); err != nil {
		return nil, err
	}
	return encodeYAML(root)
}

// replaceNodeValue copies field by field rather than assigning the node, so
// the comments attached to target survive.
func replaceNodeValue(target, src *yaml.Node) {
	target.Kind = src.Kind
	target.Tag = src.Tag
	target.Value = src.Value
	target.Style = src.Style
	target.Content = src.Content
	target.Anchor = ""
	target.Alias = nil
}

// splitYAMLPath validates a dotted path and returns its segments. Empty paths
// and empty segments (leading/trailing/consecutive dots) are rejected.
func splitYAMLPath(path string) ([]string, error) {
	if path == "" {
		return nil, fmt.Errorf("path is required")
	}
	segments := strings.Split(path, ".")
	if slices.Contains(segments, "") {
		return nil, fmt.Errorf("invalid path %q: empty segment", path)
	}
	return segments, nil
}

// parseYAMLRoot unmarshals existing into a DocumentNode whose single child is
// the root mapping. Empty input becomes a fresh document with an empty mapping.
func parseYAMLRoot(existing []byte) (*yaml.Node, error) {
	var root yaml.Node
	if len(existing) > 0 {
		if err := yaml.Unmarshal(existing, &root); err != nil {
			return nil, fmt.Errorf("parsing YAML: %w", err)
		}
	}
	if root.Kind == 0 {
		// Empty input — build a document with an empty mapping.
		root = yaml.Node{
			Kind:    yaml.DocumentNode,
			Content: []*yaml.Node{{Kind: yaml.MappingNode}},
		}
	}
	if root.Kind != yaml.DocumentNode || len(root.Content) != 1 {
		return nil, fmt.Errorf("expected a single-document YAML root")
	}
	return &root, nil
}

// encodeYAML serialises a DocumentNode back to bytes with two-space indent.
func encodeYAML(root *yaml.Node) ([]byte, error) {
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(root); err != nil {
		return nil, fmt.Errorf("encoding YAML: %w", err)
	}
	if err := enc.Close(); err != nil {
		return nil, fmt.Errorf("closing YAML encoder: %w", err)
	}
	return buf.Bytes(), nil
}

// descendToLeaf walks segments through mapping nodes, creating missing
// intermediate mappings and a scalar placeholder for a missing leaf. It
// returns the leaf value node without asserting its kind — the scalar vs
// sequence setters decide what is admissible there. Intermediate segments
// that are not mappings are rejected. The root mapping node that follows the
// DocumentNode wrapper (yaml.Node.Content[0]) is the expected starting point.
func descendToLeaf(node *yaml.Node, segments []string) (*yaml.Node, error) {
	if node.Kind != yaml.MappingNode {
		// yaml.Unmarshal on an empty document gives a null scalar; accept
		// it here as "empty mapping" for the first write into a fresh
		// file.
		if node.Kind == yaml.ScalarNode && node.Tag == "!!null" {
			node.Kind = yaml.MappingNode
			node.Tag = ""
			node.Value = ""
		} else {
			return nil, fmt.Errorf("expected a mapping at path root, got %s", kindName(node.Kind))
		}
	}

	current := node
	for i, segment := range segments {
		var found *yaml.Node
		for j := 0; j+1 < len(current.Content); j += 2 {
			if current.Content[j].Value == segment {
				found = current.Content[j+1]
				break
			}
		}

		isLeaf := i == len(segments)-1
		if found == nil {
			keyNode := &yaml.Node{Kind: yaml.ScalarNode, Value: segment}
			if isLeaf {
				valueNode := &yaml.Node{Kind: yaml.ScalarNode}
				current.Content = append(current.Content, keyNode, valueNode)
				return valueNode, nil
			}
			mappingNode := &yaml.Node{Kind: yaml.MappingNode}
			current.Content = append(current.Content, keyNode, mappingNode)
			current = mappingNode
			continue
		}

		if isLeaf {
			return found, nil
		}

		if found.Kind != yaml.MappingNode {
			return nil, fmt.Errorf("intermediate segment %q is a %s, not a mapping", segment, kindName(found.Kind))
		}
		current = found
	}
	// Unreachable: the loop returns on the leaf.
	return nil, fmt.Errorf("unreachable: path not resolved")
}

func kindName(k yaml.Kind) string {
	switch k {
	case yaml.DocumentNode:
		return "document"
	case yaml.SequenceNode:
		return "sequence"
	case yaml.MappingNode:
		return "mapping"
	case yaml.ScalarNode:
		return "scalar"
	case yaml.AliasNode:
		return "alias"
	default:
		return fmt.Sprintf("kind(%d)", int(k))
	}
}
