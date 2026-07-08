// Package engine implements the v1 workflow engine core: procedure specs
// parsed from kind: procedure graph entries, a typed variable store per
// running instance, guards as boolean combinations of named Go predicates, a
// closed function registry (predicates, queries, commands), gate-transition
// cascade with chooser stops, and append-only JSONL session persistence.
//
// Everything semantic lives in named Go functions composed by name in the
// procedure spec — the spec language has no expressions, no literals, no
// assignment. See the v1 surface spec (plan 20260702-220449-d-tac-ry0) and
// the engine directive (20260702-174833-d-cpt-3yw).
package engine

import (
	"fmt"
	"strings"

	"github.com/networkteam/sdd/internal/model"
	"github.com/networkteam/sdd/internal/query"
)

// BaseType is one of the closed set of domain types a procedure variable can
// declare. Adding a domain type means adding it here, once, with its
// validation — no other types exist.
type BaseType string

const (
	TypeText              BaseType = "text"
	TypeBool              BaseType = "bool"
	TypeEntryID           BaseType = "entry-id"
	TypeRef               BaseType = "ref"
	TypeLabel             BaseType = "label"
	TypeParticipant       BaseType = "participant"
	TypeEntryKind         BaseType = "entry-kind"
	TypeLayer             BaseType = "layer"
	TypeConfidence        BaseType = "confidence"
	TypeIntent            BaseType = "intent"
	TypeAttachmentHandle  BaseType = "attachment-handle"
	TypePreflightFindings BaseType = "preflight-findings"
)

var baseTypes = map[BaseType]bool{
	TypeText:              true,
	TypeBool:              true,
	TypeEntryID:           true,
	TypeRef:               true,
	TypeLabel:             true,
	TypeParticipant:       true,
	TypeEntryKind:         true,
	TypeLayer:             true,
	TypeConfidence:        true,
	TypeIntent:            true,
	TypeAttachmentHandle:  true,
	TypePreflightFindings: true,
}

// VarType is a declared variable type: a base domain type, optionally
// wrapped as list<T>.
type VarType struct {
	Base BaseType
	List bool
}

func (t VarType) String() string {
	if t.List {
		return "list<" + string(t.Base) + ">"
	}
	return string(t.Base)
}

// ParseVarType parses a type string from a variable declaration: a base
// domain type name or list<T> around one.
func ParseVarType(s string) (VarType, error) {
	s = strings.TrimSpace(s)
	if inner, ok := strings.CutPrefix(s, "list<"); ok {
		inner, ok = strings.CutSuffix(inner, ">")
		if !ok {
			return VarType{}, fmt.Errorf("malformed list type %q (expected list<T>)", s)
		}
		base := BaseType(strings.TrimSpace(inner))
		if !baseTypes[base] {
			return VarType{}, fmt.Errorf("unknown domain type %q in %q", inner, s)
		}
		return VarType{Base: base, List: true}, nil
	}
	base := BaseType(s)
	if !baseTypes[base] {
		return VarType{}, fmt.Errorf("unknown domain type %q", s)
	}
	return VarType{Base: base}, nil
}

// ValidateValue checks a raw value (as decoded from a JSON report or supplied
// by Go code) against the type, returning a normalized value. Lists validate
// per element.
func (t VarType) ValidateValue(v any) (any, error) {
	if t.List {
		items, ok := v.([]any)
		if !ok {
			// Accept typed slices from Go callers by rejecting only
			// genuinely non-list values; the common JSON path decodes to
			// []any.
			return nil, fmt.Errorf("expected a list of %s", t.Base)
		}
		out := make([]any, len(items))
		for i, item := range items {
			nv, err := VarType{Base: t.Base}.ValidateValue(item)
			if err != nil {
				return nil, fmt.Errorf("item %d: %w", i, err)
			}
			out[i] = nv
		}
		return out, nil
	}
	return validateBaseValue(t.Base, v)
}

func validateBaseValue(base BaseType, v any) (any, error) {
	switch base {
	case TypeText:
		s, ok := v.(string)
		if !ok {
			return nil, fmt.Errorf("expected text (string)")
		}
		return s, nil

	case TypeBool:
		b, ok := v.(bool)
		if !ok {
			return nil, fmt.Errorf("expected bool")
		}
		return b, nil

	case TypeEntryID:
		s, ok := v.(string)
		if !ok {
			return nil, fmt.Errorf("expected entry ID (string)")
		}
		if _, err := model.ParseID(s); err != nil {
			return nil, fmt.Errorf("not a full entry ID: %w", err)
		}
		return s, nil

	case TypeRef:
		m, ok := v.(map[string]any)
		if !ok {
			if r, isRef := v.(Ref); isRef {
				m = map[string]any{"id": r.ID, "kind": r.Kind, "desc": r.Desc}
			} else {
				return nil, fmt.Errorf("expected ref object {id, kind, desc?}")
			}
		}
		ref, err := refFromMap(m)
		if err != nil {
			return nil, err
		}
		return ref, nil

	case TypeLabel:
		s, ok := v.(string)
		if !ok {
			return nil, fmt.Errorf("expected topic label (string)")
		}
		if _, err := model.ParseTopicPath(s); err != nil {
			return nil, err
		}
		return s, nil

	case TypeParticipant:
		s, ok := v.(string)
		if !ok || strings.TrimSpace(s) == "" {
			return nil, fmt.Errorf("expected participant canonical (non-empty string)")
		}
		return s, nil

	case TypeEntryKind:
		s, ok := v.(string)
		if !ok {
			return nil, fmt.Errorf("expected entry kind (string)")
		}
		k := model.Kind(s)
		if !model.IsValidKindForType(model.TypeSignal, k) && !model.IsValidKindForType(model.TypeDecision, k) || s == "" {
			return nil, fmt.Errorf("unknown entry kind %q", s)
		}
		return s, nil

	case TypeLayer:
		s, ok := v.(string)
		if !ok {
			return nil, fmt.Errorf("expected layer (string)")
		}
		if full, isAbbrev := model.LayerFromAbbrev[s]; isAbbrev {
			return string(full), nil
		}
		if _, isFull := model.LayerAbbrev[model.Layer(s)]; isFull {
			return s, nil
		}
		return nil, fmt.Errorf("unknown layer %q", s)

	case TypeConfidence:
		s, ok := v.(string)
		if !ok || (s != "high" && s != "medium" && s != "low") {
			return nil, fmt.Errorf("expected confidence high|medium|low")
		}
		return s, nil

	case TypeIntent:
		s, ok := v.(string)
		if !ok || !model.IsValidIntent(s) {
			return nil, fmt.Errorf("expected intent pending|guiding|settled")
		}
		return s, nil

	case TypeAttachmentHandle:
		s, ok := v.(string)
		if !ok || strings.TrimSpace(s) == "" {
			return nil, fmt.Errorf("expected attachment handle (non-empty string)")
		}
		return s, nil

	case TypePreflightFindings:
		// Engine-written by the write gate; accept the typed form and the
		// replay/JSON form. Validation is shape-only — severity vocabulary is
		// owned by the query package.
		switch fv := v.(type) {
		case []query.Finding:
			return fv, nil
		case []any:
			findings := make([]query.Finding, 0, len(fv))
			for i, item := range fv {
				m, ok := item.(map[string]any)
				if !ok {
					return nil, fmt.Errorf("finding %d: expected object", i)
				}
				f := query.Finding{}
				if s, ok := m["severity"].(string); ok {
					f.Severity = query.Severity(s)
				}
				if s, ok := m["category"].(string); ok {
					f.Category = s
				}
				if s, ok := m["observation"].(string); ok {
					f.Observation = s
				}
				findings = append(findings, f)
			}
			return findings, nil
		default:
			return nil, fmt.Errorf("expected preflight findings list")
		}

	default:
		return nil, fmt.Errorf("unknown domain type %q", base)
	}
}

// Ref is the engine-side value of a ref-typed variable: the same closed-kind
// reference shape entries carry, before it becomes a model.Ref at the write
// gate.
type Ref struct {
	ID   string `json:"id"`
	Kind string `json:"kind"`
	Desc string `json:"desc,omitempty"`
}

func refFromMap(m map[string]any) (Ref, error) {
	id, _ := m["id"].(string)
	kind, _ := m["kind"].(string)
	desc, _ := m["desc"].(string)
	// A ref may point across the repo boundary (<repo-id>:<entry-id>);
	// entry-id typed fields (closes, supersedes, anchors) stay local-only.
	if model.IsCrossRepoID(id) {
		if err := model.ValidateCrossRepoID(id); err != nil {
			return Ref{}, fmt.Errorf("ref id: %w", err)
		}
	} else if _, err := model.ParseID(id); err != nil {
		return Ref{}, fmt.Errorf("ref id: not a full entry ID: %w", err)
	}
	if !model.IsCapturableRefKind(model.RefKind(kind)) {
		return Ref{}, fmt.Errorf("ref kind %q is not in the closed ref-kind set", kind)
	}
	return Ref{ID: id, Kind: kind, Desc: desc}, nil
}
