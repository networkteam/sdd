//go:build eval

// Temporary harness for the done-kind specimen sweep (d-tac-rvu). Runs the
// live writing guide over real graph entries and logs every finding, so the
// backwards check — would the guide have surfaced the corrections dialogue
// judged necessary? — has the guide's actual output to compare against.
// Not a pass-rate gate: it asserts nothing and is deleted once the sweep's
// proven discriminators land as real cases in writingguide_eval_test.go.
//
//	go test -tags=eval -run TestWritingGuideSweep ./internal/llmops/... -v

package llmops_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/networkteam/sdd/internal/model"
)

// loadSpecimen reads a real entry from the repository's own graph. ParseEntry
// takes the full ID as its filename, so the year and month the path layout
// carries as directories are folded back into the basename.
func loadSpecimen(t *testing.T, relPath string) *model.Entry {
	t.Helper()
	path := filepath.Join("..", "..", relPath)
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading specimen %s: %v", relPath, err)
	}
	parts := strings.Split(relPath, string(filepath.Separator))
	if len(parts) < 3 {
		t.Fatalf("specimen path %s lacks the year/month directories", relPath)
	}
	year, month, base := parts[len(parts)-3], parts[len(parts)-2], parts[len(parts)-1]
	entry, err := model.ParseEntry(year+month+base, string(content))
	if err != nil {
		t.Fatalf("parsing specimen %s: %v", relPath, err)
	}
	return entry
}

func TestWritingGuideSweep_DoneKind(t *testing.T) {
	specimens := []struct {
		name    string
		path    string
		runs    int
		expect  string
		nonFind string
		// closure describes what the specimen closes or supersedes, as the
		// production path derives it from the graph (model.Graph.ClosureTargets).
		closure []model.ClosureTarget
	}{
		{
			name:    "sux_dense_multi_commit_delivery",
			path:    ".sdd/graph/2026/08/10-145745-s-tac-sux.md",
			runs:    3,
			expect:  "one conflation on the a08266b4 overturned-diagnosis paragraph (own lifecycle); form at most minor",
			nonFind: "the five-commit batching itself; the commit hashes; the per-commit delivery descriptions",
			closure: []model.ClosureTarget{{
				Relation: "closes", ID: "20260731-081528-d-cpt-rw7",
				Type: model.TypeDecision, Kind: model.KindDirective,
				Summary: "Transport-level connection events are permanently excluded from the durable session log, which records only participant acts on the dialogue — conclude or abandon — with displacement derived live from the attachment stamp rather than read from a cause record.",
			}},
		},
		{
			name:    "wts_thin_retirement",
			path:    ".sdd/graph/2026/06/14-134025-s-tac-wts.md",
			runs:    3,
			expect:  "clean, or at most one minor on the close-vs-settled sentence",
			nonFind: "stranding for absent commit hashes (provenance follows the act: a retirement done points at the delivery done); stranding on the retired directive, which the closure edge now anchors",
			closure: []model.ClosureTarget{{
				Relation: "closes", ID: "20260608-001411-d-tac-w30",
				Type: model.TypeDecision, Kind: model.KindDirective,
				Summary: "This directive commits to a dedicated `sdd stats` command as the analytics surface for both graph activity and LLM/embedding usage, kept explicitly separate from `sdd view`, which continues to serve live graph entry surfacing.",
			}},
		},
	}

	for _, sp := range specimens {
		t.Run(sp.name, func(t *testing.T) {
			entry := loadSpecimen(t, sp.path)
			t.Logf("GROUND TRUTH expects: %s", sp.expect)
			t.Logf("GROUND TRUTH non-findings: %s", sp.nonFind)
			for i := 1; i <= sp.runs; i++ {
				result, raw, err := runGuideEvalOnce(t, entry, sp.closure...)
				if err != nil {
					t.Logf("run %d/%d infrastructure error: %v\nRaw:\n%s", i, sp.runs, err, raw)
					continue
				}
				if len(result.Findings) == 0 {
					t.Logf("run %d/%d: CLEAN (no findings)", i, sp.runs)
					continue
				}
				t.Logf("run %d/%d: %d finding(s)", i, sp.runs, len(result.Findings))
				for j, f := range result.Findings {
					t.Logf("  [%d] axis=%s severity=%s repair=%s\n      quote: %s\n      reasoning: %s",
						j+1, f.Axis, f.Severity, f.Repair, f.Quote, f.Reasoning)
				}
			}
		})
	}
}
