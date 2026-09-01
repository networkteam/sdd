//go:build eval

// Wild sweep for stored summaries (evaluation of d-cpt-75a): judges a
// stratified sample of this graph's real, stored summaries against the summary
// contract, measuring how far the wild state sits from the decided target and
// which failure axes dominate — the evidence base for the template-tuning
// round. Judge-only calls (no candidate model); run manually:
//
//	go test -tags=eval -run TestSummarizeWildSweep ./internal/llmops/... -v
//
// SDD_EVAL_WILD_GRAPH selects the graph directory (default .sdd/graph of this
// repo, resolved from the package directory); SDD_EVAL_WILD_SAMPLE bounds the
// sample size (default 40). Selection is deterministic: per kind, evenly
// spaced over the ID-sorted (i.e. chronological) list, so re-runs after
// regeneration compare like for like.

package llmops_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/networkteam/sdd/internal/finders"
	"github.com/networkteam/sdd/internal/llmops"
	"github.com/networkteam/sdd/internal/model"
	"github.com/networkteam/sdd/pkg/llm"
)

// wildVerdict mirrors the d-cpt-75a contract axis by axis.
type wildVerdict struct {
	Language bool     `json:"language_ok"`
	Grounded bool     `json:"grounded"`
	Fit      bool     `json:"fit_covered"`
	Lede     bool     `json:"lede_standalone"`
	Terse    bool     `json:"terse"`
	Concrete bool     `json:"concrete_wording"`
	Verdict  string   `json:"verdict"`
	Problems []string `json:"problems"`
}

// judgeWildSummary judges one stored summary against the summary contract
// (d-cpt-75a), criteria rendered from the directive's own wording.
func judgeWildSummary(t *testing.T, judge llm.Runner, sourceMaterial, summary string, hasNeighbors bool) (*wildVerdict, error) {
	t.Helper()
	relLine := "The source carries no refs or closes; report fit_covered as true."
	if hasNeighbors {
		relLine = "The source names related entries; after the first sentence the summary should describe how the entry fits among them (IDs as handles where they carry that fit), never enumerating edges mechanically and never adding transitive context."
	}
	req := llm.Request{
		Purpose: llm.Purpose("eval-judge"),
		SystemPrompt: `You judge stored one-entry summaries for a decision-graph tool against the project's summary contract.
A summary is a pointer: it lets a reader orient and decide without opening the entry. Judge these axes:

1. language_ok — written in the graph's configured authoring language (English for these cases).
2. grounded — asserts nothing the source material does not state: no invented actors, no
   graph-positioning commentary, no corrected identifiers. It owes NO completeness: preserving the
   main meaning and pointing at what the full entry will tell is correct, so never fail a summary
   for omitting secondary content.
3. fit_covered — after the first sentence, the summary describes how the entry fits among its
   directly referenced entries, when the source carries such relationships.
4. lede_standalone — the FIRST sentence alone states what the entry observes, commits to, or did,
   leading with its substance, compressed by selection: the single most important meaning in the
   shortest sentence that carries it. Fail a lede that stuffs several facts into stacked clauses,
   opens with bookkeeping or metadata, or buries the substance.
5. terse — at most three sentences, no filler, every clause carrying information.
6. concrete_wording — exact, concrete words: no jargon, no metaphor, no vague or inflated phrasing;
   sloppy wording propagates into future sessions, so hold a high standard.

Answer with a single JSON object and nothing else:
{"language_ok": bool, "grounded": bool, "fit_covered": bool, "lede_standalone": bool, "terse": bool, "concrete_wording": bool, "verdict": "pass"|"fail", "problems": ["..."]}
Set verdict to "fail" when any axis fails, and name each problem concretely.`,
		UserPrompt: fmt.Sprintf("SOURCE MATERIAL (the entry and its direct neighbors):\n%s\n\nNOTE: %s\n\nSTORED SUMMARY UNDER JUDGMENT:\n%s", sourceMaterial, relLine, summary),
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
		if start, end := strings.Index(text, "{"), strings.LastIndex(text, "}"); start >= 0 && end > start {
			text = text[start : end+1]
		}
		var v wildVerdict
		if err := json.Unmarshal([]byte(text), &v); err != nil {
			lastErr = fmt.Errorf("unparseable judge output: %w (raw: %q)", err, res.Text)
			continue
		}
		return &v, nil
	}
	return nil, lastErr
}

// wildSample draws the deterministic stratified sample: per kind, evenly
// spaced over the chronological (ID-sorted) list of summarized entries.
func wildSample(graph *model.Graph, size int) []*model.Entry {
	byKind := map[model.Kind][]*model.Entry{}
	total := 0
	for _, e := range graph.Entries {
		if e.Summary == "" || e.Kind == "" {
			continue
		}
		byKind[e.Kind] = append(byKind[e.Kind], e)
		total++
	}
	if total == 0 {
		return nil
	}

	kinds := make([]model.Kind, 0, len(byKind))
	for k := range byKind {
		sort.Slice(byKind[k], func(i, j int) bool { return byKind[k][i].ID < byKind[k][j].ID })
		kinds = append(kinds, k)
	}
	sort.Slice(kinds, func(i, j int) bool { return kinds[i] < kinds[j] })

	var sample []*model.Entry
	for _, k := range kinds {
		entries := byKind[k]
		// Proportional share, at least one per kind present.
		share := (len(entries)*size + total - 1) / total
		if share < 1 {
			share = 1
		}
		if share > len(entries) {
			share = len(entries)
		}
		step := float64(len(entries)) / float64(share)
		for i := 0; i < share; i++ {
			sample = append(sample, entries[int(float64(i)*step)])
		}
	}
	sort.Slice(sample, func(i, j int) bool { return sample[i].ID < sample[j].ID })
	return sample
}

func TestSummarizeWildSweep(t *testing.T) {
	dir := os.Getenv("SDD_EVAL_WILD_GRAPH")
	if dir == "" {
		dir = filepath.Join("..", "..", ".sdd", "graph")
	}
	size := 40
	if v := os.Getenv("SDD_EVAL_WILD_SAMPLE"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			t.Fatalf("malformed SDD_EVAL_WILD_SAMPLE %q", v)
		}
		size = n
	}

	graph, err := finders.New(finders.Options{}).LoadGraph(dir)
	if err != nil {
		t.Fatalf("loading graph %s: %v", dir, err)
	}
	sample := wildSample(graph, size)
	if len(sample) == 0 {
		t.Fatalf("no summarized entries in %s", dir)
	}
	t.Logf("wild sweep: %d stored summaries from %s", len(sample), dir)

	judge := judgeRunner(t)

	axisFails := map[string]int{}
	mechFails, judgeFails, infraFails := 0, 0, 0
	for _, entry := range sample {
		req, err := llmops.RenderSummaryPrompt(entry, graph, "")
		if err != nil {
			t.Fatalf("rendering source for %s: %v", entry.ID, err)
		}

		var problems []string
		if mech := checkSummaryMechanics(entry.ID, entry.Summary); len(mech) > 0 {
			mechFails++
			problems = append(problems, mech...)
		}

		hasNeighbors := len(entry.Refs) > 0 || len(entry.Closes) > 0 || len(entry.Supersedes) > 0
		v, err := judgeWildSummary(t, judge, req.UserPrompt, entry.Summary, hasNeighbors)
		if err != nil {
			infraFails++
			t.Logf("%s [%s/%s]: judge unavailable: %v", entry.ID, entry.Kind, entry.Layer, err)
			continue
		}
		if v.Verdict != "pass" {
			judgeFails++
			for axis, ok := range map[string]bool{
				"language": v.Language, "grounded": v.Grounded, "fit": v.Fit,
				"lede": v.Lede, "terse": v.Terse, "concrete": v.Concrete,
			} {
				if !ok {
					axisFails[axis]++
				}
			}
			problems = append(problems, v.Problems...)
		}
		if len(problems) > 0 {
			t.Logf("%s [%s/%s] FAIL: %s\n  summary: %s", entry.ID, entry.Kind, entry.Layer, strings.Join(problems, "; "), entry.Summary)
		} else {
			t.Logf("%s [%s/%s] pass", entry.ID, entry.Kind, entry.Layer)
		}
	}

	t.Logf("wild sweep result: %d judged, %d judge-fail, %d mechanical-fail, %d infra-fail; axis failures: %v",
		len(sample)-infraFails, judgeFails, mechFails, infraFails, axisFails)
}
