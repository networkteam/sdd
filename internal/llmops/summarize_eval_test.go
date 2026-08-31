//go:build eval

// Evaluation cases for entry summarization (d-tac-ma6). Run manually when
// comparing candidate configurations (costs real LLM calls):
//
//	go test -tags=eval -run TestSummarizeEval ./internal/llmops/... -v
//
// Summaries resist mechanical assertion, so each case is judged in two tiers:
// hard mechanical checks catch the semantic failures recorded usage cannot see
// (s-tac-k7d) — empty output, runaway length, the self-ID bug (s-cpt-ed2) —
// and an LLM judge verifies language, groundedness, and relationship coverage.
// The judge runs on its own fixed configuration (SDD_EVAL_JUDGE_PROVIDER /
// SDD_EVAL_JUDGE_MODEL, defaulting like the candidate defaults) so candidate
// selection never changes the yardstick, and its calls record under the
// eval-judge purpose to stay separable from candidate rows.

package llmops_test

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	internalllm "github.com/networkteam/sdd/internal/llm"
	"github.com/networkteam/sdd/internal/llm/factory"
	"github.com/networkteam/sdd/internal/llmops"
	"github.com/networkteam/sdd/internal/model"
	"github.com/networkteam/sdd/pkg/llm"
)

// summarizeCase is one summarization specimen: the entry to summarize and the
// graph context its prompt renders (referenced entries with summaries).
type summarizeCase struct {
	name    string
	entry   *model.Entry
	related []*model.Entry
	// wantRelationships tells the judge whether the source material carries
	// relationships (closes/refs) the summary must reflect.
	wantRelationships bool
}

func summarizeCases() []summarizeCase {
	return []summarizeCase{
		{
			name: "done signal closing a directive",
			entry: &model.Entry{
				ID:    "20260815-140000-s-tac-ev1",
				Type:  model.TypeSignal,
				Kind:  model.KindDone,
				Layer: model.LayerTactical,
				Refs: []model.Ref{
					{ID: "20260810-100000-d-tac-ev2", Kind: model.RefKindAddresses, Desc: "the batching directive this delivers"},
				},
				Closes:     []string{"20260810-100000-d-tac-ev2"},
				Confidence: "high",
				Content:    "The import pipeline now batches writes: rows accumulate per source file and flush in groups of 500, cutting a full import of the reference dataset from 41 minutes to 6. Delivered in commits 4f2a91c and 8823b0d; the per-row fsync that dominated the profile is gone, and the failure path now reports the failing batch's file and offset instead of a bare row number.",
			},
			related: []*model.Entry{
				{
					ID:      "20260810-100000-d-tac-ev2",
					Type:    model.TypeDecision,
					Kind:    model.KindDirective,
					Layer:   model.LayerTactical,
					Summary: "Commits to batching import writes in groups with a single fsync per batch, because the per-row fsync dominates import time.",
					Content: "Batch import writes.",
				},
			},
			wantRelationships: true,
		},
		{
			name: "directive with two refs",
			entry: &model.Entry{
				ID:    "20260816-090000-d-cpt-ev3",
				Type:  model.TypeDecision,
				Kind:  model.KindDirective,
				Layer: model.LayerConceptual,
				Refs: []model.Ref{
					{ID: "20260812-110000-s-cpt-ev4", Kind: model.RefKindGroundedIn, Desc: "the duplication survey this answers"},
					{ID: "20260813-150000-d-cpt-ev5", Kind: model.RefKindRelated, Desc: "the storage layout this composes with"},
				},
				Confidence: "high",
				Content:    "Validation rules live in one schema module and are compiled into both the form layer and the API layer; neither layer may define a rule of its own. The duplication survey found nine rules implemented twice with four semantic drifts between the copies, and each drift is a class of bug users hit as inconsistent behavior between the UI and the API.",
			},
			related: []*model.Entry{
				{
					ID:      "20260812-110000-s-cpt-ev4",
					Type:    model.TypeSignal,
					Kind:    model.KindInsight,
					Layer:   model.LayerConceptual,
					Summary: "Surveys validation logic across layers: nine rules exist twice, four have drifted semantically between form and API implementations.",
					Content: "Validation duplication survey.",
				},
				{
					ID:      "20260813-150000-d-cpt-ev5",
					Type:    model.TypeDecision,
					Kind:    model.KindDirective,
					Layer:   model.LayerConceptual,
					Summary: "Commits to a single storage layout definition consumed by migration and runtime code.",
					Content: "Single storage layout definition.",
				},
			},
			wantRelationships: true,
		},
		{
			name: "plain gap signal without refs",
			entry: &model.Entry{
				ID:         "20260817-160000-s-ops-ev6",
				Type:       model.TypeSignal,
				Kind:       model.KindGap,
				Layer:      model.LayerOperational,
				Confidence: "medium",
				Content:    "The nightly backup job logs success even when the upload step is skipped: the skip branch for an unreachable bucket returns nil, so the run counts as green while no artifact left the machine. Expected per the runbook: an unreachable bucket fails the run. Three of the last thirty runs took this branch.",
			},
			wantRelationships: false,
		},
	}
}

// judgeVerdict is the JSON shape the judge must answer with. The criteria
// mirror the summary prompt's own contract (summary_templates/summary.tmpl)
// so candidates are judged against the task as specified, not a paraphrase.
type judgeVerdict struct {
	LanguageOK        bool     `json:"language_ok"`
	Grounded          bool     `json:"grounded"`
	Relationships     bool     `json:"relationships_covered"`
	FirstSentenceLede bool     `json:"first_sentence_standalone"`
	Terse             bool     `json:"terse"`
	Verdict           string   `json:"verdict"`
	Problems          []string `json:"problems"`
}

// judgeRunner builds the fixed judge configuration — independent of the
// candidate env knobs, so every candidate is measured with the same yardstick.
// Default judge: Ollama Cloud's glm-5.3-flash:cloud at high effort (flat-rate,
// no per-token cost); override via SDD_EVAL_JUDGE_PROVIDER /
// SDD_EVAL_JUDGE_MODEL / SDD_EVAL_JUDGE_PARAMS.
func judgeRunner(t *testing.T) llm.Runner {
	t.Helper()
	cfg := evalBaseConfig(t)
	cfg.Provider = getenvOr("SDD_EVAL_JUDGE_PROVIDER", "ollama")
	cfg.Model = getenvOr("SDD_EVAL_JUDGE_MODEL", "glm-5.3-flash:cloud")
	cfg.Params = parseEvalParams(t, getenvOr("SDD_EVAL_JUDGE_PARAMS", "think=high"))
	runner, err := factory.New(cfg)
	if err != nil {
		t.Fatalf("building judge runner: %v", err)
	}
	return internalllm.Observed(runner, multiSink{evalFileSink(t), tLogSink{t}})
}

// judgeSummary asks the judge to verify one summary against the exact source
// material the candidate saw. One retry on unparseable judge output; a judge
// that stays unparseable fails the case as infrastructure, not as a candidate
// failure.
func judgeSummary(t *testing.T, judge llm.Runner, sourceMaterial, summary string, wantRelationships bool) (*judgeVerdict, error) {
	t.Helper()
	relLine := "The source carries no explicit relationships; report relationships_covered as true."
	if wantRelationships {
		relLine = "The source names related entries (refs/closes); the summary must reflect what the entry does to or with them."
	}
	req := llm.Request{
		Purpose: llm.Purpose("eval-judge"),
		SystemPrompt: `You judge one-entry summaries for a decision-graph tool against the summarizer's contract:

1. language_ok — the summary is written in the graph's configured authoring language — English for
   these cases — regardless of the language the source material appears in.
2. grounded — it stays strictly within the source material: no invented facts, numbers, intentions,
   or transitive context beyond the entry and its directly referenced entries.
3. relationships_covered — the remaining sentences reflect what the entry does to or with its direct
   refs/closes entries, citing them as parenthetical ID handles.
4. first_sentence_standalone — the FIRST sentence alone states what the entry observes, commits to,
   or did, leading with the entry's own substance (never bookkeeping like "supersedes X" or a list of
   references), is readable at a glance rather than a wall of clauses, and works standalone —
   overview lists display exactly this sentence.
5. terse — 2-3 sentences, roughly 50-100 words, no filler, no meta-prose ("This summary...",
   labels, markdown, quotation marks), every clause carrying information.

Answer with a single JSON object and nothing else:
{"language_ok": bool, "grounded": bool, "relationships_covered": bool, "first_sentence_standalone": bool, "terse": bool, "verdict": "pass"|"fail", "problems": ["..."]}
Set verdict to "fail" when any check fails, and name each problem concretely.`,
		UserPrompt: fmt.Sprintf("SOURCE MATERIAL (what the summarizer saw):\n%s\n\nNOTE: %s\n\nSUMMARY UNDER JUDGMENT:\n%s", sourceMaterial, relLine, summary),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 240*time.Second)
	defer cancel()

	var lastErr error
	for attempt := 0; attempt < 2; attempt++ {
		res, err := judge.Run(ctx, req)
		if err != nil {
			lastErr = err
			continue
		}
		text := strings.TrimSpace(res.Text)
		// Tolerate a fenced or prefixed JSON object.
		if start, end := strings.Index(text, "{"), strings.LastIndex(text, "}"); start >= 0 && end > start {
			text = text[start : end+1]
		}
		var v judgeVerdict
		if err := json.Unmarshal([]byte(text), &v); err != nil {
			lastErr = fmt.Errorf("unparseable judge output: %w (raw: %q)", err, res.Text)
			continue
		}
		return &v, nil
	}
	return nil, lastErr
}

// checkSummaryMechanics runs the hard checks no judge is needed for. These are
// the semantic failures that record as success rows (s-tac-k7d): empty or
// whitespace output, runaway or degenerate length, and the self-ID bug.
func checkSummaryMechanics(entryID, summary string) []string {
	var problems []string
	words := len(strings.Fields(summary))
	switch {
	case strings.TrimSpace(summary) == "":
		problems = append(problems, "summary is empty")
	case words < 25:
		problems = append(problems, fmt.Sprintf("summary degenerately short (%d words; contract says 50-100)", words))
	case words > 140:
		problems = append(problems, fmt.Sprintf("summary runaway length (%d words; contract says 50-100)", words))
	}
	if strings.Contains(summary, entryID) {
		problems = append(problems, "summary refers to the entry's own ID (self-ID bug, s-cpt-ed2)")
	}
	// The contract demands raw summary text: no labels, markdown, or quotes.
	trimmed := strings.TrimSpace(summary)
	switch {
	case strings.HasPrefix(trimmed, "\""), strings.HasPrefix(trimmed, "“"):
		problems = append(problems, "summary wrapped in quotation marks")
	case strings.HasPrefix(strings.ToLower(trimmed), "summary"):
		problems = append(problems, "summary carries a label prefix")
	}
	if strings.Contains(summary, "**") || strings.Contains(summary, "\n#") || strings.HasPrefix(trimmed, "#") {
		problems = append(problems, "summary contains markdown formatting")
	}
	return problems
}

// TestSummarizeJudgeCalibration feeds the judge deliberately broken summaries
// and expects fail verdicts — a judge that rubber-stamps these would make every
// candidate measure capable. Judge-only calls; no candidate involved.
func TestSummarizeJudgeCalibration(t *testing.T) {
	judge := judgeRunner(t)
	tc := summarizeCases()[0]
	graph := model.NewGraph(append([]*model.Entry{tc.entry}, tc.related...))
	req, err := llmops.RenderSummaryPrompt(tc.entry, graph, "")
	if err != nil {
		t.Fatalf("rendering source material: %v", err)
	}

	badSummaries := []struct {
		name    string
		summary string
	}{
		{
			name:    "wrong language",
			summary: "Die Import-Pipeline bündelt Schreibvorgänge jetzt in Gruppen von 500 Zeilen, wodurch ein vollständiger Import von 41 auf 6 Minuten sinkt. Damit ist die Batching-Direktive umgesetzt.",
		},
		{
			name:    "invented facts",
			summary: "The import pipeline now batches writes in groups of 2000 rows using a new Kafka-based queue, cutting imports from 41 to 2 minutes and reducing cloud spend by 40%. The team plans to extend this to the export path next quarter.",
		},
		{
			name:    "buried lede, wall-of-text first sentence",
			summary: "Referencing the batching directive (20260810-100000-d-tac-ev2), which it also closes, and delivered in commits 4f2a91c and 8823b0d, while additionally changing the failure path so that it now reports the failing batch's file and offset instead of a bare row number, this done signal, concerning the import pipeline and its write behavior, reports that rows now accumulate per source file and flush in groups of 500, which cut the reference dataset import from 41 minutes to 6.",
		},
	}

	for _, bad := range badSummaries {
		t.Run(bad.name, func(t *testing.T) {
			verdict, err := judgeSummary(t, judge, req.UserPrompt, bad.summary, tc.wantRelationships)
			if err != nil {
				t.Fatalf("judge unavailable: %v", err)
			}
			if verdict.Verdict != "fail" {
				t.Errorf("judge passed a deliberately broken summary (%s): language_ok=%v grounded=%v problems: %s",
					bad.name, verdict.LanguageOK, verdict.Grounded, strings.Join(verdict.Problems, "; "))
			}
		})
	}
}

func TestSummarizeEval(t *testing.T) {
	candidate := evalRunner(t)
	judge := judgeRunner(t)

	for _, tc := range summarizeCases() {
		t.Run(tc.name, func(t *testing.T) {
			graph := model.NewGraph(append([]*model.Entry{tc.entry}, tc.related...))

			ctx, cancel := context.WithTimeout(context.Background(), 240*time.Second)
			defer cancel()
			result, err := llmops.Summarize(ctx, candidate, tc.entry, graph, "")
			if err != nil {
				t.Fatalf("Summarize failed: %v", err)
			}
			t.Logf("summary: %s", result.Summary)

			if problems := checkSummaryMechanics(tc.entry.ID, result.Summary); len(problems) > 0 {
				t.Fatalf("mechanical checks failed: %s", strings.Join(problems, "; "))
			}

			// The judge sees exactly what the candidate saw.
			req, err := llmops.RenderSummaryPrompt(tc.entry, graph, "")
			if err != nil {
				t.Fatalf("rendering source material for judge: %v", err)
			}
			verdict, err := judgeSummary(t, judge, req.UserPrompt, result.Summary, tc.wantRelationships)
			if err != nil {
				t.Fatalf("judge unavailable: %v", err)
			}
			if verdict.Verdict != "pass" {
				t.Errorf("judge verdict: %s — language_ok=%v grounded=%v relationships_covered=%v first_sentence_standalone=%v terse=%v problems: %s",
					verdict.Verdict, verdict.LanguageOK, verdict.Grounded, verdict.Relationships,
					verdict.FirstSentenceLede, verdict.Terse,
					strings.Join(verdict.Problems, "; "))
			}
		})
	}
}
