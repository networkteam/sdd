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
	"encoding/json"
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
	TypeFactIndex         BaseType = "fact-index"
	TypePreflightFindings BaseType = "preflight-findings"
	TypeGuideFindings     BaseType = "guide-findings"
	TypeInvolvement       BaseType = "involvement"
	TypeInvolvementWhen   BaseType = "involvement-when"
)

// baseTypeOrder is the canonical enumeration of domain types; baseTypes
// derives from it so set and order share one declaration.
var baseTypeOrder = []BaseType{
	TypeText, TypeBool, TypeEntryID, TypeRef, TypeLabel, TypeParticipant,
	TypeEntryKind, TypeLayer, TypeConfidence, TypeIntent, TypeAttachmentHandle,
	TypeFactIndex, TypePreflightFindings, TypeGuideFindings, TypeInvolvement,
	TypeInvolvementWhen,
}

var baseTypes = func() map[BaseType]bool {
	set := make(map[BaseType]bool, len(baseTypeOrder))
	for _, t := range baseTypeOrder {
		set[t] = true
	}
	return set
}()

// BaseTypeValues lists the domain types in canonical order, for surfaces that
// render or generate from the enumeration instead of restating it.
func BaseTypeValues() []BaseType {
	return append([]BaseType(nil), baseTypeOrder...)
}

// baseTypeDesc carries each domain type's served meaning — the single
// declaration surfaces render from (a type without one fails the render).
// Semantics only: the concrete shape, enums included, is generated into the
// step's report schema (schemaForType), never restated here.
var baseTypeDesc = map[BaseType]string{
	TypeText:              "free prose — accounts, reports, syntheses; the default for anything narrative",
	TypeBool:              "true or false",
	TypeEntryID:           "a full entry identifier, resolvable against the graph",
	TypeRef:               "a reference: target id, relationship kind from the closed ref-kind set, optional why",
	TypeLabel:             "a topic label path (family/member)",
	TypeParticipant:       "a participant's canonical name",
	TypeEntryKind:         "one of the entry kinds",
	TypeLayer:             "one of the thinking layers",
	TypeConfidence:        "a confidence grade",
	TypeIntent:            "a directive's intent",
	TypeAttachmentHandle:  "a staged attachment's handle",
	TypeFactIndex:         "a fact's retrieval-index enrollment: title and topic",
	TypePreflightFindings: "write-gate findings — engine-written, never collected by a step",
	TypeGuideFindings:     "writing-guide findings — engine-written, never collected by a step",
	TypeInvolvement:       "a focus involvement: target entry, optional actors, optional time range",
	TypeInvolvementWhen:   "a from/to date range",
}

// Description returns the type's served meaning; empty for an unknown type.
func (t BaseType) Description() string {
	return baseTypeDesc[t]
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

	case TypeInvolvement:
		switch iv := v.(type) {
		case Involvement:
			return iv, nil
		case map[string]any:
			return involvementFromMap(iv)
		default:
			return nil, fmt.Errorf("expected involvement object {target, actors?, when?}")
		}

	case TypeInvolvementWhen:
		return whenFromValue(v)

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

	case TypeFactIndex:
		m, ok := v.(map[string]any)
		if !ok {
			if index, typed := v.(FactIndex); typed {
				m = map[string]any{"title": index.Title, "topic": index.Topic}
				ok = true
			}
		}
		if !ok {
			return nil, fmt.Errorf("expected fact-index object {title, topic}")
		}
		if len(m) != 2 {
			return nil, fmt.Errorf("expected fact-index object with exactly title and topic")
		}
		title, titleOK := m["title"].(string)
		topic, topicOK := m["topic"].(string)
		if !titleOK {
			return nil, fmt.Errorf("fact-index title must be a string")
		}
		if !topicOK {
			return nil, fmt.Errorf("fact-index topic must be a string")
		}
		index, err := model.NewFactIndex(title, topic)
		if err != nil {
			return nil, fmt.Errorf("fact-index: %w", err)
		}
		return FactIndex{Title: index.Title, Topic: index.Topic.String()}, nil

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
				if s, ok := findingField(m, "severity"); ok {
					f.Severity = query.Severity(s)
				}
				if s, ok := findingField(m, "category"); ok {
					f.Category = s
				}
				if s, ok := findingField(m, "observation"); ok {
					f.Observation = s
				}
				findings = append(findings, f)
			}
			return findings, nil
		default:
			return nil, fmt.Errorf("expected preflight findings list")
		}

	case TypeGuideFindings:
		// Engine-written by the writing-guide op; accept the typed form and
		// the replay/JSON form. Validation is shape-only — the axis, repair,
		// and severity vocabularies are owned by the query package.
		switch fv := v.(type) {
		case []query.GuideFinding:
			return fv, nil
		case []any:
			findings := make([]query.GuideFinding, 0, len(fv))
			for i, item := range fv {
				m, ok := item.(map[string]any)
				if !ok {
					return nil, fmt.Errorf("finding %d: expected object", i)
				}
				f := query.GuideFinding{}
				if s, ok := findingField(m, "reasoning"); ok {
					f.Reasoning = s
				}
				if s, ok := findingField(m, "axis"); ok {
					f.Axis = s
				}
				if s, ok := findingField(m, "quote"); ok {
					f.Quote = s
				}
				if s, ok := findingField(m, "repair"); ok {
					f.Repair = s
				}
				if s, ok := findingField(m, "severity"); ok {
					f.Severity = query.GuideSeverity(s)
				}
				findings = append(findings, f)
			}
			return findings, nil
		default:
			return nil, fmt.Errorf("expected writing-guide findings list")
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

type FactIndex struct {
	Title string `json:"title"`
	Topic string `json:"topic"`
}

// findingField reads a finding field by its document key, tolerating both the
// JSON-tag casing and the exported Go field name. query.Finding carries no JSON
// tags, so the store's normalized document form uses the capitalized field
// names; a report or a future tagged form would use the lowercase key.
func findingField(m map[string]any, key string) (string, bool) {
	if s, ok := m[key].(string); ok {
		return s, true
	}
	exported := strings.ToUpper(key[:1]) + key[1:]
	if s, ok := m[exported].(string); ok {
		return s, true
	}
	return "", false
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

// When is the engine-side value of an involvement-when: a temporal range that
// becomes a model.FocusWhen at the write gate. At least one end is set.
type When struct {
	From string `json:"from,omitempty"`
	To   string `json:"to,omitempty"`
}

func (w When) String() string {
	switch {
	case w.From != "" && w.To != "":
		return w.From + "→" + w.To
	case w.From != "":
		return w.From + "→"
	default:
		return "→" + w.To
	}
}

// Involvement is the engine-side value of an involvement-typed variable: one
// target a focus advances, with optional per-target actors and when, before it
// becomes a model.Involvement at the write gate. ActorsSet keeps the model's
// unset (inherit focus default) versus explicit-empty (pull-available)
// distinction across the JSON round-trip through the session log.
type Involvement struct {
	Target    string
	Actors    []string
	ActorsSet bool
	When      *When
}

func (i Involvement) String() string {
	s := i.Target
	if i.ActorsSet {
		s += " [" + strings.Join(i.Actors, ", ") + "]"
	}
	if i.When != nil {
		s += " (" + i.When.String() + ")"
	}
	return s
}

// MarshalJSON emits actors only when set, so the replay round-trip through
// map[string]any reconstructs ActorsSet from the key's presence — omitempty
// cannot tell an explicit empty list from an unset one.
func (i Involvement) MarshalJSON() ([]byte, error) {
	m := map[string]any{"target": i.Target}
	if i.ActorsSet {
		actors := i.Actors
		if actors == nil {
			actors = []string{}
		}
		m["actors"] = actors
	}
	if i.When != nil {
		m["when"] = i.When
	}
	return json.Marshal(m)
}

func involvementFromMap(m map[string]any) (Involvement, error) {
	target, _ := m["target"].(string)
	if _, err := model.ParseID(target); err != nil {
		return Involvement{}, fmt.Errorf("involvement target: not a full entry ID: %w", err)
	}
	inv := Involvement{Target: target}
	if raw, ok := m["actors"]; ok {
		inv.ActorsSet = true
		items, ok := raw.([]any)
		if !ok {
			return Involvement{}, fmt.Errorf("involvement actors: expected a list of participant canonicals")
		}
		inv.Actors = make([]string, 0, len(items))
		for i, item := range items {
			s, ok := item.(string)
			if !ok || strings.TrimSpace(s) == "" {
				return Involvement{}, fmt.Errorf("involvement actors[%d]: expected a non-empty participant canonical", i)
			}
			inv.Actors = append(inv.Actors, s)
		}
	}
	if raw, ok := m["when"]; ok {
		w, err := whenFromValue(raw)
		if err != nil {
			return Involvement{}, fmt.Errorf("involvement when: %w", err)
		}
		inv.When = w
	}
	return inv, nil
}

func whenFromValue(v any) (*When, error) {
	var from, to string
	switch w := v.(type) {
	case *When:
		if w == nil {
			return nil, fmt.Errorf("expected when object {from?, to?}")
		}
		from, to = w.From, w.To
	case When:
		from, to = w.From, w.To
	case map[string]any:
		from, _ = w["from"].(string)
		to, _ = w["to"].(string)
	default:
		return nil, fmt.Errorf("expected when object {from?, to?}")
	}
	if err := (&model.FocusWhen{From: from, To: to}).Validate(); err != nil {
		return nil, err
	}
	return &When{From: from, To: to}, nil
}
