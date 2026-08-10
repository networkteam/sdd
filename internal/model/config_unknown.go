package model

import (
	"reflect"
	"strings"

	"gopkg.in/yaml.v3"
)

// KnownYAMLKey reports whether the dotted path names something t declares: a
// field, a block on the way to one, or a key below a map-valued field, where
// arbitrary names are the value.
//
// This and UnknownYAMLKeys read the same struct walk the effective-config view
// uses, so "a config key" is defined in one place. Judging one key at a time
// is what lets a file hold a key from another version and stay writable
// (20260810-144515-s-tac-8ae).
func KnownYAMLKey(path string, t reflect.Type) bool {
	for _, leaf := range collectConfigLeaves(t, nil, nil) {
		switch {
		case path == leaf.path:
			return true
		case strings.HasPrefix(leaf.path, path+"."):
			return true
		case leaf.kind == leafMap && strings.HasPrefix(path, leaf.path+"."):
			return true
		}
	}
	return false
}

// UnknownYAMLKeys returns the dotted paths in data that t does not declare,
// in document order.
func UnknownYAMLKeys(data []byte, t reflect.Type) ([]string, error) {
	var root yaml.Node
	if err := yaml.Unmarshal(data, &root); err != nil {
		return nil, err
	}
	if root.Kind != yaml.DocumentNode || len(root.Content) == 0 {
		return nil, nil
	}

	known := map[string]bool{}
	for _, leaf := range collectConfigLeaves(t, nil, nil) {
		known[leaf.path] = true
	}
	// A struct-valued field contributes no leaf of its own, only its children,
	// so `llm` is declared by virtue of `llm.provider`.
	branches := map[string]bool{}
	for path := range known {
		segments := strings.Split(path, ".")
		for i := 1; i < len(segments); i++ {
			branches[strings.Join(segments[:i], ".")] = true
		}
	}

	var unknown []string
	var walk func(node *yaml.Node, prefix string)
	walk = func(node *yaml.Node, prefix string) {
		if node.Kind != yaml.MappingNode {
			return
		}
		for i := 0; i+1 < len(node.Content); i += 2 {
			key, value := node.Content[i].Value, node.Content[i+1]
			path := key
			if prefix != "" {
				path = prefix + "." + key
			}
			// A leaf ends the descent whatever its kind — below a map, the
			// keys are the value.
			if known[path] {
				continue
			}
			if branches[path] {
				walk(value, path)
				continue
			}
			unknown = append(unknown, path)
		}
	}
	walk(root.Content[0], "")
	return unknown, nil
}
