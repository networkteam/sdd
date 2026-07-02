package engine

import (
	"testing"
)

func TestReportSchemaForStep(t *testing.T) {
	env := newFixtureEnv(t)
	step := env.spec.StepByID["assemble"]
	schema := env.spec.ReportSchemaForStep(step)

	props, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("schema has no properties: %v", schema)
	}

	// Collect fields are present with type and the declaration's desc.
	body, ok := props["body"].(map[string]any)
	if !ok || body["type"] != "string" {
		t.Errorf("body property = %v", props["body"])
	}
	if desc, _ := body["description"].(string); desc == "" {
		t.Error("body property should carry the declaration desc")
	}

	// Enumerated domain types advertise their closed sets.
	conf, _ := props["confidence"].(map[string]any)
	if conf == nil || len(conf["enum"].([]any)) != 3 {
		t.Errorf("confidence enum = %v", props["confidence"])
	}

	// refs is an array of ref objects with the closed kind set.
	refs, _ := props["refs"].(map[string]any)
	if refs == nil || refs["type"] != "array" {
		t.Fatalf("refs property = %v", props["refs"])
	}
	refItem, _ := refs["items"].(map[string]any)
	refProps, _ := refItem["properties"].(map[string]any)
	kindProp, _ := refProps["kind"].(map[string]any)
	if kinds, _ := kindProp["enum"].([]any); len(kinds) != 9 {
		t.Errorf("ref kind enum = %v, want the 9 capturable kinds", kindProp["enum"])
	}

	// Required = the step's non-optional collect fields; intent is optional.
	required, _ := schema["required"].([]string)
	reqSet := map[string]bool{}
	for _, r := range required {
		reqSet[r] = true
	}
	if !reqSet["body"] || !reqSet["widenReport"] || reqSet["intent"] {
		t.Errorf("required = %v", required)
	}

	// Batching: state fields beyond the step's collect list are accepted
	// properties (fidelityNote belongs to a later step).
	if _, ok := props["fidelityNote"]; !ok {
		t.Error("batched later-step fields must be accepted properties")
	}
	if schema["additionalProperties"] != false {
		t.Error("undeclared fields must be rejected by the schema")
	}
}
