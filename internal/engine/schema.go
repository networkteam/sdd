package engine

import (
	"sort"

	"github.com/networkteam/sdd/internal/model"
)

// entryIDSchemaPattern is the JSON-Schema pattern for a full entry ID, shared
// by every string property that carries one (entry-id, ref.id, involvement.target).
const entryIDSchemaPattern = `^\d{8}-\d{6}-[sd]-(stg|cpt|tac|ops|prc)-[a-z0-9]+$`

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
		properties[name] = schemaForState(decl)
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

// AnswerSchemaForStep generates the JSON Schema for a chooser step's answer:
// the chooser to name (its step id), the choice to pick, the user's verbatim
// words for a user chooser, and the option-collected fields nested under
// `fields`. A flat report schema at a chooser step misled callers into placing
// collected fields at the top level (rejected); this envelope makes the
// required nesting explicit. `fields` unions every option's collect fields —
// each option enforces its own required set at answer time, so the envelope
// marks none of them required.
func (s *Spec) AnswerSchemaForStep(step *Step) map[string]any {
	choices := make([]any, 0, len(step.Options))
	fieldProps := make(map[string]any)
	for _, o := range step.Options {
		choices = append(choices, o.Choice)
		for _, cf := range o.Collect {
			if _, seen := fieldProps[cf.Name]; seen {
				continue
			}
			if decl, ok := s.State[cf.Name]; ok {
				fieldProps[cf.Name] = schemaForState(decl)
			}
		}
	}

	properties := map[string]any{
		"chooser": map[string]any{
			"type":        "string",
			"const":       step.ID,
			"description": "the pending chooser's step id — copy it verbatim from pending_chooser.chooser",
		},
		"choice": map[string]any{
			"type":        "string",
			"enum":        choices,
			"description": "the option to take",
		},
	}
	required := []string{"choice", "chooser"}
	if step.Chooser == ChooserUser {
		properties["userWords"] = map[string]any{
			"type":        "string",
			"description": "the user's answer, relayed verbatim",
		}
		required = append(required, "userWords")
	}
	if len(fieldProps) > 0 {
		properties["fields"] = map[string]any{
			"type":                 "object",
			"properties":           fieldProps,
			"additionalProperties": false,
			"description":          "state fields the chosen option collects — nested here, not at the top level",
		}
	}

	return map[string]any{
		"type":                 "object",
		"properties":           properties,
		"required":             required,
		"additionalProperties": false,
	}
}

func schemaForState(decl VarDecl) map[string]any {
	schema := schemaForType(decl.Type, "")
	if decl.Optional {
		schema = map[string]any{"anyOf": []any{schema, map[string]any{"type": "null"}}}
	}
	if decl.Desc != "" {
		schema["description"] = decl.Desc
	}
	return schema
}

// procedureSpecSchema is the advertised shape of a workflow declaration —
// hand-written beside the type like every fragment here, mirroring the YAML
// intermediate structs the engine decodes (spec.go); ParseSpec is the
// enforcement, so a drift here mis-advertises but never mis-accepts.
func procedureSpecSchema() map[string]any {
	declBlock := map[string]any{
		"type":        "object",
		"description": "field name → declaration",
		"additionalProperties": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"type":     map[string]any{"type": "string"},
				"optional": map[string]any{"type": "boolean"},
				"desc":     map[string]any{"type": "string"},
				"default":  map[string]any{},
			},
			"required":             []string{"type"},
			"additionalProperties": false,
		},
	}
	inject := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"id":       map[string]any{"type": "string"},
			"fn":       map[string]any{"type": "string"},
			"args":     map[string]any{"type": "object"},
			"maxBytes": map[string]any{"type": "integer", "minimum": 0},
			"maxItems": map[string]any{"type": "integer", "minimum": 0},
		},
		"required":             []string{"fn"},
		"additionalProperties": false,
	}
	transition := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"when":      map[string]any{"type": "string"},
			"otherwise": map[string]any{"type": "string"},
			"to":        map[string]any{"type": "string"},
		},
		"additionalProperties": false,
	}
	option := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"choice":  map[string]any{"type": "string"},
			"call":    map[string]any{"type": "string"},
			"collect": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
			"to":      map[string]any{"type": "string"},
			"dispatch": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"procedure": map[string]any{"type": "string"},
					"seed":      map[string]any{"type": "object", "additionalProperties": map[string]any{"type": "string"}},
				},
				"required":             []string{"seed"},
				"additionalProperties": false,
			},
		},
		"required":             []string{"choice", "to"},
		"additionalProperties": false,
	}
	step := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"id":          map[string]any{"type": "string"},
			"collect":     map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
			"inject":      map[string]any{"type": "array", "items": inject},
			"render":      map[string]any{"type": "string"},
			"chooser":     map[string]any{"type": "string", "enum": []any{"gate", "agent", "user"}},
			"options":     map[string]any{"type": "array", "items": option},
			"guard":       map[string]any{"type": "string"},
			"op":          map[string]any{"type": "string"},
			"transitions": map[string]any{"type": "array", "items": transition},
			"goal":        map[string]any{"type": "string"},
			"serveDelta":  map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
		},
		"required":             []string{"id"},
		"additionalProperties": false,
	}
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"params":  declBlock,
			"state":   declBlock,
			"steps":   map[string]any{"type": "array", "items": step},
			"framing": map[string]any{"type": "array", "items": inject},
			"serveBudget": map[string]any{
				"type": "integer", "minimum": 0,
				"description": "declared worst-case serve total in bytes; a larger total than the engine default silences the authoring-arithmetic finding and records the trade",
			},
		},
		"required":             []string{"steps"},
		"additionalProperties": false,
	}
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
			"pattern": entryIDSchemaPattern,
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
					"pattern": entryIDSchemaPattern,
				},
				"kind": map[string]any{"type": "string", "enum": enum},
				"desc": map[string]any{"type": "string"},
			},
			"required":             []string{"id", "kind"},
			"additionalProperties": false,
		}
	case TypeProcedureSpec:
		schema = procedureSpecSchema()
	case TypeInvolvement:
		schema = map[string]any{
			"type": "object",
			"properties": map[string]any{
				"target": map[string]any{
					"type":    "string",
					"pattern": entryIDSchemaPattern,
				},
				"actors": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
				"when":   involvementWhenSchema(),
			},
			"required":             []string{"target"},
			"additionalProperties": false,
		}
	case TypeInvolvementWhen:
		schema = involvementWhenSchema()
	case TypeSearchReplace:
		schema = map[string]any{
			"type": "object",
			"properties": map[string]any{
				"old": map[string]any{"type": "string", "minLength": 1, "description": "exact text to replace — must match exactly once in the target as it stands when this pair applies"},
				"new": map[string]any{"type": "string", "description": "replacement text; empty deletes the old text"},
			},
			"required":             []string{"old", "new"},
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
	case TypeFactIndex:
		schema = map[string]any{
			"type": "object",
			"properties": map[string]any{
				"title": map[string]any{"type": "string", "minLength": 1},
				"topic": map[string]any{"type": "string", "minLength": 1},
			},
			"required":             []string{"title", "topic"},
			"additionalProperties": false,
		}
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
	case TypeGuideFindings:
		schema = map[string]any{
			"type": "array",
			"items": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"reasoning": map[string]any{"type": "string"},
					"axis":      map[string]any{"type": "string", "enum": []any{"stranding", "dilution", "conflation", "pointing", "form"}},
					"quote":     map[string]any{"type": "string"},
					"repair":    map[string]any{"type": "string", "enum": []any{"cut", "write-in", "split", "point", "reword"}},
					"severity":  map[string]any{"type": "string", "enum": []any{"substantive", "minor"}},
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

// involvementWhenSchema is the {from, to} ISO-date object shared by the
// involvement type's nested when and the standalone involvement-when type.
// minProperties matches FocusWhen.Validate, which rejects an empty range — an
// empty when must be omitted, not spelled as {}.
func involvementWhenSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"from": map[string]any{"type": "string", "pattern": `^\d{4}-\d{2}-\d{2}$`},
			"to":   map[string]any{"type": "string", "pattern": `^\d{4}-\d{2}-\d{2}$`},
		},
		"minProperties":        1,
		"additionalProperties": false,
	}
}
