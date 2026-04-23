package handlers_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/networkteam/sdd/internal/command"
	"github.com/networkteam/sdd/internal/handlers"
	"github.com/networkteam/sdd/internal/model"
	"github.com/networkteam/sdd/internal/query"
)

// fakeReader bundles every read the handler needs into one stub. Tests
// configure preflightResult/preflightErr and graph as needed; LoadWIPMarkers
// returns nil since the new-entry handler doesn't touch it.
type fakeReader struct {
	preflightResult *query.PreflightResult
	preflightErr    error
	graph           *model.Graph
}

func (f *fakeReader) LoadGraph(_ string) (*model.Graph, error) {
	if f.graph != nil {
		return f.graph, nil
	}
	return model.NewGraph(nil), nil
}

func (f *fakeReader) LoadWIPMarkers(_ string) ([]*model.WIPMarker, error) {
	return nil, nil
}

func (f *fakeReader) Preflight(ctx context.Context, q query.PreflightQuery) (*query.PreflightResult, error) {
	return f.preflightResult, f.preflightErr
}

func (f *fakeReader) SkillStatus(ctx context.Context, q query.SkillStatusQuery) (*query.SkillStatusResult, error) {
	return &query.SkillStatusResult{}, nil
}

// recordingCommitter captures commit calls so tests can assert whether a
// commit was attempted.
type recordingCommitter struct {
	calls int
}

func (r *recordingCommitter) Commit(message string, paths ...string) error {
	r.calls++
	return nil
}

// TestNewEntry_DryRun_SavesStdinOnValidationFailure is the regression test for
// d-prc-31v / d-tac-q5p: dry-run + validation failure must still save stdin.
// The bug was that the reportSavedStdin call was placed after graph validation,
// so an invalid ref would return early without persisting stdin — contradicting
// the plan's "save on every dry-run (pass or fail)" spec.
func TestNewEntry_DryRun_SavesStdinOnValidationFailure(t *testing.T) {
	tmp := t.TempDir()
	sddDir := filepath.Join(tmp, ".sdd")
	if err := os.MkdirAll(sddDir, 0755); err != nil {
		t.Fatal(err)
	}
	stderr := &bytes.Buffer{}
	committer := &recordingCommitter{}

	h := handlers.New(handlers.Options{
		GraphDir:  tmp,
		SDDDir:    sddDir,
		Reader:    &fakeReader{},
		Committer: committer,
		Stderr:    stderr,
	})

	stdinContent := []byte("plan content for iteration")
	onNewEntryCalled := false

	cmd := &command.NewEntryCmd{
		Type:        model.TypeSignal,
		Layer:       model.LayerTactical,
		Description: "test signal",
		Refs:        []string{"nonexistent-dangling-ref"}, // triggers validation failure
		Attachments: []command.Attachment{
			{Source: "-", Target: "plan.md", Data: stdinContent},
		},
		SkipPreflight: true, // skip preflight so this test doesn't need a runner
		DryRun:        true,
		OnNewEntry:    func(id string) { onNewEntryCalled = true },
	}

	err := h.NewEntry(context.Background(), cmd)
	if err == nil {
		t.Fatal("NewEntry: expected validation error, got nil")
	}
	if !strings.Contains(err.Error(), "validation failed") {
		t.Errorf("error = %v, want validation failure", err)
	}

	// The regression: on dry-run + validation failure, stdin MUST be saved.
	saveDir := filepath.Join(sddDir, "tmp")
	entries, err := os.ReadDir(saveDir)
	if err != nil {
		t.Fatalf(".sdd/tmp/ should exist after dry-run + stdin: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf(".sdd/tmp/ should contain 1 file, got %d", len(entries))
	}

	saved := entries[0].Name()
	if !strings.HasSuffix(saved, "-plan.md") {
		t.Errorf("saved file %q should end with -plan.md", saved)
	}
	data, err := os.ReadFile(filepath.Join(saveDir, saved))
	if err != nil {
		t.Fatalf("reading saved file: %v", err)
	}
	if string(data) != string(stdinContent) {
		t.Errorf("saved content = %q, want %q", string(data), string(stdinContent))
	}

	// Rejection path must NOT fire the success callback and NOT commit.
	if onNewEntryCalled {
		t.Error("OnNewEntry should not be called on validation failure")
	}
	if committer.calls != 0 {
		t.Errorf("Committer.Commit called %d times; want 0 on failure", committer.calls)
	}

	// Stderr should mention the save for the user's benefit.
	if !strings.Contains(stderr.String(), "stdin attachment saved (dry-run)") {
		t.Errorf("stderr should announce the dry-run save, got:\n%s", stderr.String())
	}
}

// TestNewEntry_DryRun_Pass_SavesStdinOnce verifies that dry-run with a passing
// pre-flight still saves stdin, and that the save message appears only once
// (the defer + explicit post-pre-flight call are idempotent).
func TestNewEntry_DryRun_Pass_SavesStdinOnce(t *testing.T) {
	tmp := t.TempDir()
	sddDir := filepath.Join(tmp, ".sdd")
	if err := os.MkdirAll(sddDir, 0755); err != nil {
		t.Fatal(err)
	}
	stderr := &bytes.Buffer{}

	h := handlers.New(handlers.Options{
		GraphDir: tmp,
		SDDDir:   sddDir,
		Reader:   &fakeReader{},
		Stderr:   stderr,
	})

	cmd := &command.NewEntryCmd{
		Type:        model.TypeSignal,
		Layer:       model.LayerTactical,
		Description: "valid signal",
		Attachments: []command.Attachment{
			{Source: "-", Target: "plan.md", Data: []byte("hello")},
		},
		SkipPreflight: true,
		DryRun:        true,
	}

	if err := h.NewEntry(context.Background(), cmd); err != nil {
		t.Fatalf("NewEntry: %v", err)
	}

	saveDir := filepath.Join(sddDir, "tmp")
	entries, err := os.ReadDir(saveDir)
	if err != nil {
		t.Fatalf(".sdd/tmp/ should exist: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf(".sdd/tmp/ should contain 1 file, got %d", len(entries))
	}

	// The save message should appear exactly once (idempotency).
	if got := strings.Count(stderr.String(), "stdin attachment saved"); got != 1 {
		t.Errorf("save message appears %d times, want 1; stderr:\n%s", got, stderr.String())
	}
}

// TestNewEntry_PreflightReject_SavesStdin covers the real-run rejection path:
// stdin gets saved for re-piping even when the user didn't ask for --dry-run.
func TestNewEntry_PreflightReject_SavesStdin(t *testing.T) {
	tmp := t.TempDir()
	sddDir := filepath.Join(tmp, ".sdd")
	if err := os.MkdirAll(sddDir, 0755); err != nil {
		t.Fatal(err)
	}
	stderr := &bytes.Buffer{}

	h := handlers.New(handlers.Options{
		GraphDir: tmp,
		SDDDir:   sddDir,
		Reader: &fakeReader{
			preflightResult: &query.PreflightResult{Findings: []query.Finding{
				{Severity: query.SeverityHigh, Category: "missing-ref", Observation: "missing ref"},
			}},
		},
		Stderr: stderr,
	})

	cmd := &command.NewEntryCmd{
		Type:        model.TypeSignal,
		Layer:       model.LayerTactical,
		Description: "signal the validator will reject",
		Attachments: []command.Attachment{
			{Source: "-", Target: "plan.md", Data: []byte("rejection test content")},
		},
	}

	err := h.NewEntry(context.Background(), cmd)
	if err == nil {
		t.Fatal("NewEntry: expected pre-flight rejection error")
	}
	if !strings.Contains(err.Error(), "pre-flight rejected") {
		t.Errorf("error = %v, want pre-flight rejection", err)
	}

	saveDir := filepath.Join(sddDir, "tmp")
	if _, err := os.Stat(saveDir); err != nil {
		t.Fatalf(".sdd/tmp/ should exist after rejection: %v", err)
	}
	entries, err := os.ReadDir(saveDir)
	if err != nil || len(entries) != 1 {
		t.Fatalf(".sdd/tmp/ should contain 1 file; err=%v, entries=%v", err, entries)
	}
	if !strings.Contains(stderr.String(), "stdin attachment saved (pre-flight rejected)") {
		t.Errorf("stderr should announce the pre-flight save, got:\n%s", stderr.String())
	}
}

// testEntry is a minimal helper that builds an entry for seeding the test graph.
// Parses type/layer from the ID so suffix-search resolution works the same way
// it does against a real graph.
func testEntry(id string) *model.Entry {
	parts, err := model.ParseID(id)
	if err != nil {
		panic(err)
	}
	typ := model.TypeFromAbbrev[parts.TypeCode]
	layer := model.LayerFromAbbrev[parts.LayerCode]
	kind := model.Kind("")
	if typ == model.TypeDecision {
		kind = model.KindDirective
	}
	return &model.Entry{
		ID:      id,
		Type:    typ,
		Layer:   layer,
		Kind:    kind,
		Content: id,
		Time:    parts.Time,
	}
}

// TestNewEntry_ResolvesShortRefs verifies that short-form IDs in --refs get
// resolved to full IDs before validation. Without resolution the dangling-ref
// check would surface a validation error for the short form; with it, the
// entry passes validation against the existing full-ID entry.
func TestNewEntry_ResolvesShortRefs(t *testing.T) {
	tmp := t.TempDir()
	sddDir := filepath.Join(tmp, ".sdd")
	if err := os.MkdirAll(sddDir, 0755); err != nil {
		t.Fatal(err)
	}
	stderr := &bytes.Buffer{}

	existing := testEntry("20260406-100000-s-stg-aaa")
	graph := model.NewGraph([]*model.Entry{existing})

	h := handlers.New(handlers.Options{
		GraphDir: tmp,
		SDDDir:   sddDir,
		Reader:   &fakeReader{graph: graph},
		Stderr:   stderr,
	})

	cmd := &command.NewEntryCmd{
		Type:          model.TypeDecision,
		Layer:         model.LayerTactical,
		Kind:          model.KindDirective,
		Description:   "new decision referencing short-form ID",
		Refs:          []string{"s-stg-aaa"},
		SkipPreflight: true,
		DryRun:        true, // stop before writing; we only care about resolution + validation
	}

	if err := h.NewEntry(context.Background(), cmd); err != nil {
		t.Fatalf("NewEntry: %v (short-form ref should resolve to an existing entry)", err)
	}

	// Stderr must not contain a validation-failure log about the short ref —
	// the handler logs each Warning before returning the validation error.
	if strings.Contains(stderr.String(), "dangling ref") {
		t.Errorf("stderr reports a dangling ref; resolution did not happen:\n%s", stderr.String())
	}
}

// TestNewEntry_AmbiguousShortRefErrors verifies that an ambiguous short-form
// ref in --refs is surfaced as a hard error (no fallback to "first match").
func TestNewEntry_AmbiguousShortRefErrors(t *testing.T) {
	tmp := t.TempDir()
	sddDir := filepath.Join(tmp, ".sdd")
	if err := os.MkdirAll(sddDir, 0755); err != nil {
		t.Fatal(err)
	}

	graph := model.NewGraph([]*model.Entry{
		testEntry("20260406-100000-s-stg-xyz"),
		testEntry("20260407-110000-s-stg-xyz"),
	})

	h := handlers.New(handlers.Options{
		GraphDir: tmp,
		SDDDir:   sddDir,
		Reader:   &fakeReader{graph: graph},
	})

	cmd := &command.NewEntryCmd{
		Type:          model.TypeDecision,
		Layer:         model.LayerTactical,
		Kind:          model.KindDirective,
		Description:   "decision with ambiguous short ref",
		Refs:          []string{"s-stg-xyz"},
		SkipPreflight: true,
		DryRun:        true,
	}

	err := h.NewEntry(context.Background(), cmd)
	if err == nil {
		t.Fatal("NewEntry: want error for ambiguous short ID in refs")
	}
	if !strings.Contains(err.Error(), "ambiguous") {
		t.Errorf("error = %v, want ambiguous message", err)
	}
}

// actorSeed returns a kind: actor signal to stand in as an existing
// actor head for tests that seed an actor-identity chain.
func actorSeed(id, canonical string) *model.Entry {
	e := &model.Entry{
		ID:        id,
		Type:      model.TypeSignal,
		Kind:      model.KindActor,
		Layer:     model.LayerProcess,
		Canonical: canonical,
		Content:   "actor " + canonical,
	}
	parts, _ := model.ParseID(id)
	e.Time = parts.Time
	return e
}

// TestNewEntry_WritesEntryByKind is a table-driven pin of the
// NewEntryCmd → handler → file write path for every kind in the 6+6
// system introduced by plan d-cpt-d34. Each scenario asserts that
// kind-specific frontmatter (canonical + aliases for actor, actor for
// role) and the common kind field round-trip through the writer. Model-
// level tests don't cover this stack; skipping it is what let actor
// and role capture flags ship broken.
func TestNewEntry_WritesEntryByKind(t *testing.T) {
	const (
		closeTargetID = "20260406-100000-d-tac-tgt"
		actorHeadID   = "20260410-120000-s-prc-act"
	)

	closeTarget := testEntry(closeTargetID)
	existingActor := actorSeed(actorHeadID, "Christopher")

	cases := []struct {
		name    string
		seed    []*model.Entry
		cmd     command.NewEntryCmd
		asserts func(t *testing.T, e *model.Entry)
	}{
		{
			name: "signal gap",
			cmd: command.NewEntryCmd{
				Type: model.TypeSignal, Layer: model.LayerConceptual, Kind: model.KindGap,
				Description: "A gap needing attention.", Confidence: "medium",
			},
			asserts: func(t *testing.T, e *model.Entry) {
				assertKindAndNoKindSpecificFields(t, e, model.KindGap)
			},
		},
		{
			name: "signal fact",
			cmd: command.NewEntryCmd{
				Type: model.TypeSignal, Layer: model.LayerConceptual, Kind: model.KindFact,
				Description: "A fact observed in practice.", Confidence: "high",
			},
			asserts: func(t *testing.T, e *model.Entry) {
				assertKindAndNoKindSpecificFields(t, e, model.KindFact)
			},
		},
		{
			name: "signal question",
			cmd: command.NewEntryCmd{
				Type: model.TypeSignal, Layer: model.LayerConceptual, Kind: model.KindQuestion,
				Description: "How should X behave under Y?", Confidence: "medium",
			},
			asserts: func(t *testing.T, e *model.Entry) {
				assertKindAndNoKindSpecificFields(t, e, model.KindQuestion)
			},
		},
		{
			name: "signal insight",
			cmd: command.NewEntryCmd{
				Type: model.TypeSignal, Layer: model.LayerConceptual, Kind: model.KindInsight,
				Description: "Synthesis across recent dialogue.", Confidence: "medium",
			},
			asserts: func(t *testing.T, e *model.Entry) {
				assertKindAndNoKindSpecificFields(t, e, model.KindInsight)
			},
		},
		{
			name: "signal done",
			seed: []*model.Entry{closeTarget},
			cmd: command.NewEntryCmd{
				Type: model.TypeSignal, Layer: model.LayerTactical, Kind: model.KindDone,
				Description: "Implemented the thing.", Confidence: "high",
				Closes: []string{closeTargetID},
			},
			asserts: func(t *testing.T, e *model.Entry) {
				assertKindAndNoKindSpecificFields(t, e, model.KindDone)
				if len(e.Closes) != 1 || e.Closes[0] != closeTargetID {
					t.Errorf("closes = %v, want [%s]", e.Closes, closeTargetID)
				}
			},
		},
		{
			name: "signal actor",
			cmd: command.NewEntryCmd{
				Type: model.TypeSignal, Layer: model.LayerProcess, Kind: model.KindActor,
				Description: "Christopher is the primary human contributor on the SDD project.",
				Confidence:  "high",
				Canonical:   "Christopher",
				Aliases:     []string{"Chris", "CH"},
			},
			asserts: func(t *testing.T, e *model.Entry) {
				if e.Kind != model.KindActor {
					t.Errorf("kind = %q, want actor", e.Kind)
				}
				if e.Canonical != "Christopher" {
					t.Errorf("canonical = %q, want Christopher", e.Canonical)
				}
				if len(e.Aliases) != 2 || e.Aliases[0] != "Chris" || e.Aliases[1] != "CH" {
					t.Errorf("aliases = %v, want [Chris CH]", e.Aliases)
				}
				if e.Actor != "" {
					t.Errorf("actor should be empty on an actor signal, got %q", e.Actor)
				}
			},
		},
		{
			name: "decision directive",
			cmd: command.NewEntryCmd{
				Type: model.TypeDecision, Layer: model.LayerConceptual, Kind: model.KindDirective,
				Description: "Go in direction X.", Confidence: "high",
			},
			asserts: func(t *testing.T, e *model.Entry) {
				assertKindAndNoKindSpecificFields(t, e, model.KindDirective)
			},
		},
		{
			name: "decision activity",
			cmd: command.NewEntryCmd{
				Type: model.TypeDecision, Layer: model.LayerTactical, Kind: model.KindActivity,
				Description: "Do the scoped work.", Confidence: "medium",
			},
			asserts: func(t *testing.T, e *model.Entry) {
				assertKindAndNoKindSpecificFields(t, e, model.KindActivity)
			},
		},
		{
			name: "decision plan",
			cmd: command.NewEntryCmd{
				Type: model.TypeDecision, Layer: model.LayerTactical, Kind: model.KindPlan,
				Description: "Plan body.\n\n## Acceptance criteria\n- [ ] finish X\n",
				Confidence:  "medium",
			},
			asserts: func(t *testing.T, e *model.Entry) {
				assertKindAndNoKindSpecificFields(t, e, model.KindPlan)
				if !strings.Contains(e.Content, "## Acceptance criteria") {
					t.Errorf("plan content missing AC section, got %q", e.Content)
				}
			},
		},
		{
			name: "decision contract",
			cmd: command.NewEntryCmd{
				Type: model.TypeDecision, Layer: model.LayerConceptual, Kind: model.KindContract,
				Description: "Always honor invariant Z.", Confidence: "high",
			},
			asserts: func(t *testing.T, e *model.Entry) {
				assertKindAndNoKindSpecificFields(t, e, model.KindContract)
			},
		},
		{
			name: "decision aspiration",
			cmd: command.NewEntryCmd{
				Type: model.TypeDecision, Layer: model.LayerStrategic, Kind: model.KindAspiration,
				Description: "Pull toward W.", Confidence: "medium",
			},
			asserts: func(t *testing.T, e *model.Entry) {
				assertKindAndNoKindSpecificFields(t, e, model.KindAspiration)
			},
		},
		{
			name: "decision role",
			seed: []*model.Entry{existingActor},
			cmd: command.NewEntryCmd{
				Type: model.TypeDecision, Layer: model.LayerProcess, Kind: model.KindRole,
				Description: "Christopher reviews architectural decisions.",
				Confidence:  "medium",
				Actor:       "Christopher",
				Refs:        []string{actorHeadID},
			},
			asserts: func(t *testing.T, e *model.Entry) {
				if e.Kind != model.KindRole {
					t.Errorf("kind = %q, want role", e.Kind)
				}
				if e.Actor != "Christopher" {
					t.Errorf("actor = %q, want Christopher", e.Actor)
				}
				if e.Canonical != "" || len(e.Aliases) != 0 {
					t.Errorf("role should not carry canonical/aliases, got canonical=%q aliases=%v", e.Canonical, e.Aliases)
				}
				if len(e.Refs) != 1 || e.Refs[0] != actorHeadID {
					t.Errorf("refs = %v, want [%s]", e.Refs, actorHeadID)
				}
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// End-to-end: seed graph → handler writes entry → parse file
			// back → assert. Inlined rather than factored to a helper so
			// the test body reads as "here's the path a capture travels"
			// instead of "here's a helper call and an assertion lambda."
			tmp := t.TempDir()
			sddDir := filepath.Join(tmp, ".sdd")
			if err := os.MkdirAll(sddDir, 0755); err != nil {
				t.Fatal(err)
			}

			h := handlers.New(handlers.Options{
				GraphDir: tmp,
				SDDDir:   sddDir,
				Reader:   &fakeReader{graph: model.NewGraph(tc.seed)},
			})

			var writtenID string
			cmd := tc.cmd
			cmd.SkipPreflight = true
			cmd.OnNewEntry = func(id string) { writtenID = id }

			if err := h.NewEntry(context.Background(), &cmd); err != nil {
				t.Fatalf("NewEntry: %v", err)
			}
			if writtenID == "" {
				t.Fatal("OnNewEntry was not invoked — no id captured")
			}

			rel, err := model.IDToRelPath(writtenID)
			if err != nil {
				t.Fatalf("IDToRelPath: %v", err)
			}
			data, err := os.ReadFile(filepath.Join(tmp, rel))
			if err != nil {
				t.Fatalf("read written entry: %v", err)
			}
			parsed, err := model.ParseEntry(writtenID+".md", string(data))
			if err != nil {
				t.Fatalf("ParseEntry: %v", err)
			}

			tc.asserts(t, parsed)
		})
	}
}

// assertKindAndNoKindSpecificFields covers the common assertion for kinds
// that carry no kind-specific frontmatter: the written kind matches, and
// actor/role-specific fields stay empty. Prevents regressions where a
// future kind accidentally inherits one of the dedicated fields.
func assertKindAndNoKindSpecificFields(t *testing.T, e *model.Entry, want model.Kind) {
	t.Helper()
	if e.Kind != want {
		t.Errorf("kind = %q, want %q", e.Kind, want)
	}
	if e.Canonical != "" {
		t.Errorf("%s should not carry canonical, got %q", want, e.Canonical)
	}
	if len(e.Aliases) != 0 {
		t.Errorf("%s should not carry aliases, got %v", want, e.Aliases)
	}
	if e.Actor != "" {
		t.Errorf("%s should not carry actor, got %q", want, e.Actor)
	}
}
