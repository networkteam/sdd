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
// targeting a non-scalar leaf (or a slice/map value) is rejected.
//
// Path uses dotted segments (e.g. "participant", "llm.provider"). No
// support for array indexing or filter expressions — keep paths flat.
// Empty segments (leading/trailing/consecutive dots) are rejected.
//
// The port of the JSONPath-capable vignet patcher was intentional: SDD's
// settings are flat config, so the simpler dotted-path form covers the
// need without the extra dependency.
func SetYAMLField(existing []byte, path string, value any) ([]byte, error) {
	if path == "" {
		return nil, fmt.Errorf("path is required")
	}
	segments := strings.Split(path, ".")
	if slices.Contains(segments, "") {
		return nil, fmt.Errorf("invalid path %q: empty segment", path)
	}

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

	target, err := descendMapping(root.Content[0], segments)
	if err != nil {
		return nil, err
	}

	valueNode := &yaml.Node{Kind: yaml.ScalarNode}
	if err := valueNode.Encode(value); err != nil {
		return nil, fmt.Errorf("encoding value: %w", err)
	}
	if valueNode.Kind != yaml.ScalarNode {
		return nil, fmt.Errorf("value for %q is not scalar", path)
	}

	target.Kind = yaml.ScalarNode
	target.Tag = valueNode.Tag
	target.Value = valueNode.Value
	target.Style = valueNode.Style

	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(&root); err != nil {
		return nil, fmt.Errorf("encoding YAML: %w", err)
	}
	if err := enc.Close(); err != nil {
		return nil, fmt.Errorf("closing YAML encoder: %w", err)
	}
	return buf.Bytes(), nil
}

// descendMapping walks segments through mapping nodes, creating missing
// intermediate mappings and the final scalar placeholder. Returns the
// scalar node the caller should populate. The root mapping node that
// follows the DocumentNode wrapper (yaml.Node.Content[0]) is the expected
// starting point.
func descendMapping(node *yaml.Node, segments []string) (*yaml.Node, error) {
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
			if found.Kind != yaml.ScalarNode {
				return nil, fmt.Errorf("target at %q is a %s, not a scalar", strings.Join(segments, "."), kindName(found.Kind))
			}
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
