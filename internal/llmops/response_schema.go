package llmops

import (
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
)

// The response schemas pre-flight and the writing guide declare on their
// requests (20261004-134853-d-cpt-sw8). Kept as JSON files, not Go values:
// strict decoding generates properties in schema order, which puts each
// finding's reasoning before its verdict, and a Go map would sort the keys.
//
//go:embed response_schemas/*.json
var responseSchemas embed.FS

var (
	preflightSchema    = mustSchema("preflight.json")
	writingGuideSchema = mustSchema("writing_guide.json")
)

// mustSchema loads an embedded response schema, compacted with its property
// order intact. The files ship in the binary, so a malformed one is a build
// defect and panics at init.
func mustSchema(name string) json.RawMessage {
	raw, err := responseSchemas.ReadFile("response_schemas/" + name)
	if err != nil {
		panic(fmt.Sprintf("reading response schema %s: %v", name, err))
	}
	var buf bytes.Buffer
	if err := json.Compact(&buf, raw); err != nil {
		panic(fmt.Sprintf("compacting response schema %s: %v", name, err))
	}
	return json.RawMessage(buf.Bytes())
}

// findingEnum returns the closed value set a findings schema declares for one
// property of a finding, so the parser accepts exactly what the schema allows.
func findingEnum(schema json.RawMessage, property string) map[string]bool {
	var s struct {
		Properties struct {
			Findings struct {
				Items struct {
					Properties map[string]struct {
						Enum []string `json:"enum"`
					} `json:"properties"`
				} `json:"items"`
			} `json:"findings"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(schema, &s); err != nil {
		panic(fmt.Sprintf("decoding response schema: %v", err))
	}
	values := s.Properties.Findings.Items.Properties[property].Enum
	if len(values) == 0 {
		panic(fmt.Sprintf("response schema declares no enum for finding property %q", property))
	}
	set := make(map[string]bool, len(values))
	for _, v := range values {
		set[v] = true
	}
	return set
}
