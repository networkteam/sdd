package llmops_test

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/networkteam/sdd/internal/basefacts"
	"github.com/networkteam/sdd/internal/llmops"
	"github.com/networkteam/sdd/internal/model"
	"github.com/networkteam/sdd/internal/viewlayout"
	"github.com/networkteam/sdd/pkg/llm"
)

// The checks declare their result shape as a response schema the provider
// enforces (20261004-134853-d-cpt-sw8), so the prompts describe what each
// field means and teach no JSON construction (20261004-135748-d-tac-ndh); a
// quote only has to locate its passage (20261005-170145-d-tac-gch).
func TestCheckerPromptContract(t *testing.T) {
	facts, err := basefacts.Entries(viewlayout.Vocabulary{})
	if err != nil {
		t.Fatal(err)
	}
	graph := model.NewGraph(facts)
	entry := &model.Entry{Type: model.TypeSignal, Kind: model.KindGap, Layer: model.LayerOperational, Content: "The loader drops the second option."}

	checkers := []struct {
		name       string
		run        func(llm.Runner) error
		fields     []string
		severities []string
		quoteRule  []string
	}{
		{
			name: "preflight",
			run: func(r llm.Runner) error {
				_, err := llmops.Preflight(t.Context(), r, entry, graph, "")
				return err
			},
			fields:     []string{"observation", "category", "severity"},
			severities: []string{string(llmops.SeverityHigh), string(llmops.SeverityMedium), string(llmops.SeverityLow)},
			quoteRule:  []string{"take enough of its words to locate the passage", "need not match it character for character"},
		},
		{
			name: "writing_guide",
			run: func(r llm.Runner) error {
				_, err := llmops.WritingGuide(t.Context(), r, entry, nil, llmops.ReferenceFacts{
					Source: responseFactSource{graph}, TypeSystemFactID: basefacts.OverviewFactID,
				})
				return err
			},
			fields:     []string{"reasoning", "axis", "quote", "repair", "severity"},
			severities: []string{string(llmops.GuideSubstantive), string(llmops.GuideMinor)},
			quoteRule: []string{
				"enough of the draft's own words to locate it unambiguously",
				"need not match the draft character for character",
				"the quote `gains this as a fixed section` locates the passage",
			},
		},
	}
	for _, c := range checkers {
		t.Run(c.name, func(t *testing.T) {
			var req llm.Request
			runner := llm.RunnerFunc(func(_ context.Context, r llm.Request) (llm.Result, error) {
				req = r
				return llm.Result{Text: `{"findings":[]}`}, nil
			})
			if err := c.run(runner); err != nil {
				t.Fatal(err)
			}

			schema := findingSchema(t, req.ResponseSchema)
			if !slices.Equal(schema.order, c.fields) {
				t.Errorf("finding properties = %v, want %v (strict decoding generates in schema order, reasoning first)", schema.order, c.fields)
			}
			if !slices.Equal(schema.required, c.fields) {
				t.Errorf("required = %v, want every field %v", schema.required, c.fields)
			}
			if !slices.Equal(schema.enum["severity"], c.severities) {
				t.Errorf("severity enum = %v, want the parser's %v", schema.enum["severity"], c.severities)
			}

			prompt := req.SystemPrompt + "\n" + req.UserPrompt
			for _, rule := range []string{"JSON", "escap", "code fence", "nothing before or after", `{"findings"`} {
				if strings.Contains(prompt, rule) {
					t.Errorf("prompt still teaches JSON construction: contains %q", rule)
				}
			}
			for _, want := range c.quoteRule {
				if !strings.Contains(req.SystemPrompt, want) {
					t.Errorf("system prompt missing the quote rule %q", want)
				}
			}
		})
	}
}

type parsedFindingSchema struct {
	order    []string
	required []string
	enum     map[string][]string
}

// findingSchema reads a findings schema's item properties in document order,
// which a decode into a Go map would lose.
func findingSchema(t *testing.T, raw json.RawMessage) parsedFindingSchema {
	t.Helper()
	var doc struct {
		AdditionalProperties *bool `json:"additionalProperties"`
		Properties           struct {
			Findings struct {
				Items struct {
					Properties           json.RawMessage `json:"properties"`
					Required             []string        `json:"required"`
					AdditionalProperties *bool           `json:"additionalProperties"`
				} `json:"items"`
			} `json:"findings"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("response schema is not valid JSON: %v\n%s", err, raw)
	}
	items := doc.Properties.Findings.Items
	if doc.AdditionalProperties == nil || *doc.AdditionalProperties || items.AdditionalProperties == nil || *items.AdditionalProperties {
		t.Errorf("every object must forbid additional properties, as strict mode requires: %s", raw)
	}

	dec := json.NewDecoder(strings.NewReader(string(items.Properties)))
	if _, err := dec.Token(); err != nil {
		t.Fatalf("finding properties: %v", err)
	}
	out := parsedFindingSchema{required: items.Required, enum: map[string][]string{}}
	for dec.More() {
		key, err := dec.Token()
		if err != nil {
			t.Fatalf("finding properties: %v", err)
		}
		var prop struct {
			Enum []string `json:"enum"`
		}
		if err := dec.Decode(&prop); err != nil {
			t.Fatalf("finding property %v: %v", key, err)
		}
		name := key.(string)
		out.order = append(out.order, name)
		if prop.Enum != nil {
			out.enum[name] = prop.Enum
		}
	}
	return out
}
