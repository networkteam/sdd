package engine

import (
	"sort"

	"github.com/networkteam/sdd/internal/model"
)

// ReportSchemaForStep generates the JSON Schema for a step's report from the
// spec's variable declarations: the step's collect fields are the named,
// described properties (required unless marked optional), and every other
// declared state field is accepted too — reports may batch fields for later
// steps, and the cascade rule makes the one-shot full draft as fast as
// today. The same desc that feeds instruction text feeds the schema.
func (s *Spec) ReportSchemaForStep(step *Step) map[string]any {
	properties := make(map[string]any)
	var required []string

	addField := func(name string) {
		decl, ok := s.State[name]
		if !ok {
			return // load validation rejects undeclared collect names
		}
		properties[name] = schemaForType(decl.Type, decl.Desc)
	}

	if step != nil {
		for _, cf := range step.Collect {
			addField(cf.Name)
			if !cf.Optional {
				required = append(required, cf.Name)
			}
		}
	}
	for name := range s.State {
		if _, ok := properties[name]; !ok {
			addField(name)
		}
	}
	sort.Strings(required)

	schema := map[string]any{
		"type":                 "object",
		"properties":           properties,
		"additionalProperties": false,
	}
	if len(required) > 0 {
		schema["required"] = required
	}
	return schema
}

// schemaForType maps a domain type to its JSON Schema fragment. Validation
// on arrival is VarType.ValidateValue — the schema is the advertised
// contract, the store write is the enforcement.
func schemaForType(t VarType, desc string) map[string]any {
	if t.List {
		item := schemaForType(VarType{Base: t.Base}, "")
		schema := map[string]any{
			"type":  "array",
			"items": item,
		}
		if desc != "" {
			schema["description"] = desc
		}
		return schema
	}

	var schema map[string]any
	switch t.Base {
	case TypeBool:
		schema = map[string]any{"type": "boolean"}
	case TypeEntryID:
		schema = map[string]any{
			"type":    "string",
			"pattern": `^\d{8}-\d{6}-[sd]-(stg|cpt|tac|ops|prc)-[a-z0-9]+$`,
		}
	case TypeRef:
		kinds := model.RefKindValues()
		enum := make([]any, 0, len(kinds))
		for _, k := range kinds {
			enum = append(enum, string(k))
		}
		schema = map[string]any{
			"type": "object",
			"properties": map[string]any{
				"id": map[string]any{
					"type":    "string",
					"pattern": `^\d{8}-\d{6}-[sd]-(stg|cpt|tac|ops|prc)-[a-z0-9]+$`,
				},
				"kind": map[string]any{"type": "string", "enum": enum},
				"desc": map[string]any{"type": "string"},
			},
			"required":             []string{"id", "kind"},
			"additionalProperties": false,
		}
	case TypeEntryKind:
		schema = map[string]any{"type": "string", "enum": []any{
			"gap", "fact", "question", "insight", "done", "actor", "annotation",
			"directive", "activity", "plan", "contract", "aspiration", "role", "focus", "procedure",
		}}
	case TypeLayer:
		schema = map[string]any{"type": "string", "enum": []any{
			"strategic", "conceptual", "tactical", "operational", "process",
			"stg", "cpt", "tac", "ops", "prc",
		}}
	case TypeConfidence:
		schema = map[string]any{"type": "string", "enum": []any{"high", "medium", "low"}}
	case TypeIntent:
		schema = map[string]any{"type": "string", "enum": []any{"pending", "guiding", "settled"}}
	case TypePreflightFindings:
		schema = map[string]any{
			"type": "array",
			"items": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"severity":    map[string]any{"type": "string", "enum": []any{"high", "medium", "low"}},
					"category":    map[string]any{"type": "string"},
					"observation": map[string]any{"type": "string"},
				},
			},
		}
	default:
		// text, label, participant, attachment-handle
		schema = map[string]any{"type": "string"}
	}
	if desc != "" {
		schema["description"] = desc
	}
	return schema
}
