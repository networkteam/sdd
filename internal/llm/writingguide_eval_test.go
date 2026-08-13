//go:build eval

// Seed evaluation cases for the writing guide (d-tac-rvu). Run manually when
// tuning the guide text (costs real LLM calls):
//
//	go test -tags=eval -run TestWritingGuideEval ./internal/llm/... -v
//
// Deliberately small: the guide's calibration ground truth is developed
// against dialogue-judged specimens (the sweeps d-tac-rvu commits), and every
// banked discriminator is expected to land here as a case alongside its
// insight entry. What is seeded now is only what is already settled — the
// anti-find-something invariant (a clean draft comes back empty; the failure
// mode the pre-flight corpus documents, s-prc-dix) and the stranding test the
// guide exists to run (d-cpt-20r). Provider/model selection, pass-rate tiers,
// and the runner all reuse the pre-flight eval's machinery in this package.

package llm_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/networkteam/sdd/internal/basefacts"
	"github.com/networkteam/sdd/internal/llm"
	"github.com/networkteam/sdd/internal/model"
	"github.com/networkteam/sdd/internal/viewlayout"
)

// evalFactSource serves the shipped base facts, so the eval exercises the
// prompt exactly as production renders it — reference knowledge included.
type evalFactSource map[string]string

func (s evalFactSource) FactBody(id string) (string, error) {
	body, ok := s[id]
	if !ok {
		return "", fmt.Errorf("reference fact %s does not resolve", id)
	}
	return body, nil
}

func evalReferenceFacts(t *testing.T, kind model.Kind) llm.ReferenceFacts {
	t.Helper()
	entries, err := basefacts.Entries(viewlayout.Vocabulary{})
	if err != nil {
		t.Fatalf("basefacts.Entries: %v", err)
	}
	source := evalFactSource{}
	for _, e := range entries {
		source[e.ID] = e.Content
	}
	return llm.ReferenceFacts{
		Source:           source,
		TypeSystemFactID: basefacts.OverviewFactID,
		KindFactID:       basefacts.AuthoringFactID(kind),
	}
}

// runGuideEvalOnce runs the real writing-guide pipeline once; infrastructure
// errors are returned, not fatal, so pass-rate cases count them as failed runs.
func runGuideEvalOnce(t *testing.T, draft *model.Entry, closure ...model.ClosureTarget) (*llm.WritingGuideResult, string, error) {
	t.Helper()
	runner := evalRunner(t)
	ctx, cancel := context.WithTimeout(context.Background(), 240*time.Second)
	defer cancel()
	result, err := llm.WritingGuide(ctx, runner, draft, closure, evalReferenceFacts(t, draft.Kind))
	return result, runner.lastText, err
}

// runGuideEvalPassRate mirrors runEvalPassRate for the writing guide.
func runGuideEvalPassRate(t *testing.T, draft *model.Entry, rate passRate, check func(*llm.WritingGuideResult) error) {
	t.Helper()
	rate = rate.withRunsOverride()
	passes := 0
	for i := 1; i <= rate.Runs; i++ {
		result, raw, err := runGuideEvalOnce(t, draft)
		if err != nil {
			t.Logf("run %d/%d FAIL (infrastructure): %v\nRaw output:\n%s", i, rate.Runs, err, raw)
			continue
		}
		if checkErr := check(result); checkErr != nil {
			t.Logf("run %d/%d FAIL: %v\nFindings: %+v\nRaw output:\n%s", i, rate.Runs, checkErr, result.Findings, raw)
			continue
		}
		passes++
		t.Logf("run %d/%d pass. Findings: %+v", i, rate.Runs, result.Findings)
	}
	if passes < rate.MinPasses {
		t.Errorf("pass rate %d/%d below required %d/%d", passes, rate.Runs, rate.MinPasses, rate.Runs)
	} else {
		t.Logf("pass rate %d/%d (required %d/%d)", passes, rate.Runs, rate.MinPasses, rate.Runs)
	}
}

func hasAxis(result *llm.WritingGuideResult, axis string) error {
	for _, f := range result.Findings {
		if f.Axis == axis {
			return nil
		}
	}
	return fmt.Errorf("no %s finding fired", axis)
}

func noFindings(result *llm.WritingGuideResult) error {
	if len(result.Findings) > 0 {
		return fmt.Errorf("findings fired on a clean draft: %+v", result.Findings)
	}
	return nil
}

// TestWritingGuideEval_CleanDraftStaysClean pins the anti-find-something
// invariant at the strict tier: a spurious finding on every capture is the
// noise pattern that killed trust in the old pre-flight.
func TestWritingGuideEval_CleanDraftStaysClean(t *testing.T) {
	draft := &model.Entry{
		Type: model.TypeDecision, Kind: model.KindDirective, Layer: model.LayerTactical,
		Intent: model.IntentPending,
		Refs: []model.Ref{
			{ID: "20260601-120000-d-prc-rel", Kind: model.RefKind("refines"), Desc: "the release procedure gaining the fixed section"},
		},
		Content: "Prerelease notes must carry the install command pinned to the published version, because a prerelease is installed deliberately rather than through the default channel. The release procedure (20260601-120000-d-prc-rel) gains this as a fixed section of its notes template. The alternative — linking the documentation's installation page — was rejected because that page always describes the latest stable release, which is exactly the version a prerelease installer must not get.",
	}
	runGuideEvalPassRate(t, draft, blockingTier, noFindings)
}

// TestWritingGuideEval_StrandedDraftYieldsStranding pins the check the guide
// exists to run (d-cpt-20r): meaning alive only in the dialogue must surface
// as a stranding finding.
func TestWritingGuideEval_StrandedDraftYieldsStranding(t *testing.T) {
	draft := &model.Entry{
		Type: model.TypeDecision, Kind: model.KindDirective, Layer: model.LayerTactical,
		Intent:  model.IntentPending,
		Content: "Switch the sync path over to the approach we discussed. The earlier attempt failed for the reasons Christopher noted in the call, so this run uses the second option instead. This also unblocks the migration.",
	}
	runGuideEvalPassRate(t, draft, advisoryTier, func(r *llm.WritingGuideResult) error {
		return hasAxis(r, "stranding")
	})
}

// TestWritingGuideEval_ConflatedDraftYieldsConflation pins the split test:
// two commitments with independent lifecycles fused into one body.
func TestWritingGuideEval_ConflatedDraftYieldsConflation(t *testing.T) {
	draft := &model.Entry{
		Type: model.TypeDecision, Kind: model.KindDirective, Layer: model.LayerTactical,
		Intent:  model.IntentPending,
		Content: "All handlers adopt structured logging through slog, replacing direct stderr prints, so log level and shape are uniform across commands. Additionally, the next release drops the 32-bit ARM builds from the artifact matrix, since no installation on that platform was reported and each release currently spends CI time cross-compiling for it.",
	}
	runGuideEvalPassRate(t, draft, advisoryTier, func(r *llm.WritingGuideResult) error {
		return hasAxis(r, "conflation")
	})
}
