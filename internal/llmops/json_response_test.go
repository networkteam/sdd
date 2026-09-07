package llmops_test

import (
	"context"
	"strings"
	"testing"

	"github.com/networkteam/sdd/internal/basefacts"
	"github.com/networkteam/sdd/internal/llmops"
	"github.com/networkteam/sdd/internal/model"
	"github.com/networkteam/sdd/internal/viewlayout"
	"github.com/networkteam/sdd/pkg/llm"
)

type responseFactSource struct{ graph *model.Graph }

func (s responseFactSource) FactBody(id string) (string, error) {
	_, body, err := s.graph.FactBody(id)
	return body, err
}

func TestCheckersJSONResponse(t *testing.T) {
	facts, err := basefacts.Entries(viewlayout.Vocabulary{})
	if err != nil {
		t.Fatal(err)
	}
	graph := model.NewGraph(facts)
	entry := &model.Entry{
		Type: model.TypeSignal, Kind: model.KindGap, Layer: model.LayerOperational,
		Content: "The config ignores Options{Zebra: true}.",
	}
	checkers := []struct {
		name    string
		payload string
		run     func(llm.Runner) (int, error)
	}{
		{
			name:    "preflight",
			payload: `{"findings":[{"severity":"high","category":"missing-ref","observation":"The affected configuration decision is not referenced."}]}`,
			run: func(runner llm.Runner) (int, error) {
				result, err := llmops.Preflight(t.Context(), runner, entry, graph, "")
				if err != nil {
					return 0, err
				}
				return len(result.Findings), nil
			},
		},
		{
			name:    "writing_guide",
			payload: `{"findings":[{"reasoning":"The configuration is not identified.","axis":"stranding","quote":"The config","repair":"write-in","severity":"substantive"}]}`,
			run: func(runner llm.Runner) (int, error) {
				result, err := llmops.WritingGuide(t.Context(), runner, entry, nil, llmops.ReferenceFacts{
					Source: responseFactSource{graph}, TypeSystemFactID: basefacts.OverviewFactID,
				})
				if err != nil {
					return 0, err
				}
				return len(result.Findings), nil
			},
		},
	}
	for _, checker := range checkers {
		t.Run(checker.name, func(t *testing.T) {
			cases := []struct {
				name    string
				output  string
				want    int
				wantErr bool
			}{
				{name: "plain", output: checker.payload, want: 1},
				{name: "fenced_with_prose", output: "Here is the review:\n```json\n" + checker.payload + "\n```\nDone.", want: 1},
				{name: "literal_in_json_string", output: strings.Replace(checker.payload, "The", "Options{Zebra: true} affects the", 1), want: 1},
				{name: "literal_after_payload", output: checker.payload + "\nReviewed Options{Zebra: true}.", want: 1},
				{name: "go_literal_before_payload", output: "Reviewed Options{Zebra: true}.\n" + checker.payload, want: 1},
				{name: "user_literal_before_payload", output: "Reviewed User{ID: userID}.\n" + checker.payload, want: 1},
				{name: "empty_object_before_payload", output: "Reviewed {}.\n" + checker.payload, wantErr: true},
				{name: "unrelated_json_before_payload", output: "Reviewed {\"Zebra\":true}.\n" + checker.payload, wantErr: true},
				{name: "nested_code_literal", output: "Reviewed Options{Child: Child{Enabled: true}}.\n" + checker.payload, want: 1},
				{name: "quoted_prose", output: "The \"review\" follows.\n" + checker.payload, want: 1},
				{name: "escaped_strings", output: strings.Replace(checker.payload, "The", `Quoted \"{x}\" and C:\\tmp affect the`, 1), want: 1},
				{name: "two_results", output: checker.payload + "\n" + checker.payload, wantErr: true},
				{name: "clean_result_after_findings", output: checker.payload + `{"findings":[]}`, wantErr: true},
				{name: "empty_object_after_payload", output: checker.payload + "\n{}", wantErr: true},
				{name: "nested_result_is_not_promoted", output: `{"wrapper":` + checker.payload + `}`, wantErr: true},
				{name: "invalid_outer_object", output: `{wrapper:` + checker.payload + `}`, wantErr: true},
				{name: "unclosed_prose_brace", output: "Reviewed {\n" + checker.payload, wantErr: true},
				{name: "truncated_second_result", output: checker.payload + `{"findings":[`, wantErr: true},
				{name: "findings_object", output: `{"findings":{}}`, wantErr: true},
				{name: "findings_string", output: `{"findings":"[]"}`, wantErr: true},
				{name: "invalid_finding", output: `{"findings":[{}]}`, wantErr: true},
				{name: "no_object", output: "No findings.", wantErr: true},
				{name: "empty_response", output: "", wantErr: true},
				{name: "explicit_empty_findings", output: `{"findings":[]}`},
				{name: "missing_findings", output: `{}`, wantErr: true},
				{name: "null_findings", output: `{"findings":null}`, wantErr: true},
				{name: "malformed_findings", output: `{"findings":[{severity:high}]}`, wantErr: true},
			}
			for _, tc := range cases {
				t.Run(tc.name, func(t *testing.T) {
					runner := llm.RunnerFunc(func(context.Context, llm.Request) (llm.Result, error) {
						return llm.Result{Text: tc.output}, nil
					})
					got, err := checker.run(runner)
					if tc.wantErr {
						if err == nil {
							t.Fatalf("expected invalid response error, got %d findings", got)
						}
						return
					}
					if err != nil {
						t.Fatal(err)
					}
					if got != tc.want {
						t.Fatalf("got %d findings, want %d", got, tc.want)
					}
				})
			}
		})
	}
}
