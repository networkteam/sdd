package handlers_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/networkteam/sdd/internal/command"
	"github.com/networkteam/sdd/internal/handlers"
	"github.com/networkteam/sdd/internal/model"
	"github.com/networkteam/sdd/pkg/llm"
)

// stubRunner answers the summary call with a fixed title and summary and the
// title-only call with the title, counting invocations per purpose. It stands
// in for the LLM so the batch summarize tests stay deterministic.
type stubRunner struct {
	mu   sync.Mutex
	text string
	// numbered appends the call's number to the title, so a batch's titles
	// repeat none another call wrote.
	numbered bool
	calls    int
	byUse    map[llm.Purpose]int
}

// stubTitle is the stub runner's lowercase answer; files carry it capitalized.
const (
	stubTitle   = "generated title"
	storedTitle = "Generated title"
)

func (r *stubRunner) Run(_ context.Context, request llm.Request) (llm.Result, error) {
	r.mu.Lock()
	r.calls++
	if r.byUse == nil {
		r.byUse = map[llm.Purpose]int{}
	}
	r.byUse[request.Purpose]++
	title := stubTitle
	if r.numbered {
		title = fmt.Sprintf("%s %d", stubTitle, r.calls)
	}
	r.mu.Unlock()
	answer := map[string]string{"title": title, "summary": r.text}
	if request.Purpose == llm.PurposeTitle {
		answer = map[string]string{"title": title}
	}
	data, err := json.Marshal(answer)
	if err != nil {
		return llm.Result{}, err
	}
	return llm.Result{Text: string(data), Identity: llm.Identity{Provider: "test", Model: "test-model"}}, nil
}

func (r *stubRunner) purposeCount(purpose llm.Purpose) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.byUse[purpose]
}

func (r *stubRunner) callCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.calls
}

func readEntryFile(t *testing.T, graphDir, id string) string {
	t.Helper()
	rel, err := model.IDToRelPath(id)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(graphDir, rel))
	if err != nil {
		t.Fatalf("reading %s: %v", id, err)
	}
	return string(data)
}

const (
	summarizedEntry = "20260101-120000-s-prc-aaa" // ships with a summary, no title
	emptyEntry      = "20260101-120001-s-prc-bbb" // has neither
	titledEntry     = "20260101-120002-s-prc-ccc" // has both
)

func writeSummarizePair(t *testing.T, graphDir string) {
	t.Helper()
	writeEntryFile(t, graphDir, summarizedEntry, "---\ntype: signal\nlayer: process\nkind: gap\nsummary: Existing summary.\n---\n\nHas a summary already.\n")
	writeEntryFile(t, graphDir, emptyEntry, "---\ntype: signal\nlayer: process\nkind: gap\n---\n\nNeeds a summary.\n")
}

func writeTitledEntry(t *testing.T, graphDir string) {
	t.Helper()
	writeEntryFile(t, graphDir, titledEntry, "---\ntype: signal\nlayer: process\nkind: gap\ntitle: Kept title\nsummary: Kept summary.\n---\n\nHas both already.\n")
}

// TestSummarize_All_FillsWhatIsMissing covers the --all fill semantics
// (d-cpt-4qi): an entry with neither gets summary and title from one call, an
// entry with a summary gets a title-only call that leaves the summary as
// stored, and an entry with both is skipped.
func TestSummarize_All_FillsWhatIsMissing(t *testing.T) {
	graphDir := t.TempDir()
	writeSummarizePair(t, graphDir)
	writeTitledEntry(t, graphDir)

	runner := &stubRunner{text: "GENERATED", numbered: true}
	h := handlers.New(handlers.Options{
		GraphDir:  graphDir,
		Reader:    &graphReader{},
		Committer: &recordingCommitter{},
		LLMRunner: runner,
	})

	var summarized, titled, skipped []string
	scmd := &command.SummarizeCmd{
		OnSummarized: func(id, _ string) { summarized = append(summarized, id) },
		OnTitled:     func(id, _ string) { titled = append(titled, id) },
		OnSkipped:    func(id string) { skipped = append(skipped, id) },
	}
	if err := h.Summarize(context.Background(), scmd); err != nil {
		t.Fatalf("Summarize: %v", err)
	}

	if runner.purposeCount(llm.PurposeSummarize) != 1 || runner.purposeCount(llm.PurposeTitle) != 1 {
		t.Errorf("one paired and one title-only call expected, got %v", runner.byUse)
	}
	if len(summarized) != 1 || summarized[0] != emptyEntry {
		t.Errorf("only the entry without a summary should be summarized, got %v", summarized)
	}
	if len(titled) != 1 || titled[0] != summarizedEntry {
		t.Errorf("only the entry with a summary and no title should be titled, got %v", titled)
	}
	if len(skipped) != 1 || skipped[0] != titledEntry {
		t.Errorf("only the entry with both should be skipped, got %v", skipped)
	}

	withSummary := readEntryFile(t, graphDir, summarizedEntry)
	if !strings.Contains(withSummary, "summary: Existing summary.") || !strings.Contains(withSummary, "title: "+storedTitle) {
		t.Errorf("a stored summary should survive while the title is added:\n%s", withSummary)
	}
	empty := readEntryFile(t, graphDir, emptyEntry)
	if !strings.Contains(empty, "summary: GENERATED") || !strings.Contains(empty, "title: "+storedTitle) {
		t.Errorf("an entry without a summary should gain summary and title:\n%s", empty)
	}
	if kept := readEntryFile(t, graphDir, titledEntry); !strings.Contains(kept, "title: Kept title") || !strings.Contains(kept, "summary: Kept summary.") {
		t.Errorf("an entry with both should be untouched:\n%s", kept)
	}
}

// commitArgs runs the commit command a failure names through the shell, with
// printf standing in for git, and returns the arguments git would receive.
func commitArgs(t *testing.T, err error) []string {
	t.Helper()
	if err == nil || !strings.Contains(err.Error(), "git commit: signing timed out") {
		t.Fatalf("the failed commit should surface, got %v", err)
	}
	_, line, ok := strings.Cut(err.Error(), "commit them with: git ")
	if !ok {
		t.Fatalf("the error should name the command that commits the written files, got %v", err)
	}
	out, runErr := exec.Command("sh", "-c", `printf '%s\n' `+line).Output()
	if runErr != nil {
		t.Fatalf("the named command should parse in a shell: %v", runErr)
	}
	return strings.Split(strings.TrimSuffix(string(out), "\n"), "\n")
}

func entryPath(t *testing.T, graphDir, id string) string {
	t.Helper()
	rel, err := model.IDToRelPath(id)
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Join(graphDir, rel)
}

func TestSummarize_CommitFailureNamesTheUncommittedFiles(t *testing.T) {
	graphDir := filepath.Join(t.TempDir(), "the team's graph")
	writeSummarizePair(t, graphDir)
	h := handlers.New(handlers.Options{
		GraphDir: graphDir, Reader: &graphReader{}, Committer: &recordingCommitter{err: errors.New("signing timed out")},
		LLMRunner: &stubRunner{text: "GENERATED", numbered: true},
	})
	args := commitArgs(t, h.Summarize(context.Background(), &command.SummarizeCmd{}))
	want := []string{"commit", "-m", "sdd: summarize 2 entries", "--", entryPath(t, graphDir, summarizedEntry), entryPath(t, graphDir, emptyEntry)}
	if !slices.Equal(args, want) {
		t.Errorf("the named command should commit exactly the written files, got %q", args)
	}
}

func TestSummarize_ExplicitCommitFailureNamesTheUncommittedFile(t *testing.T) {
	graphDir := filepath.Join(t.TempDir(), "the team's graph")
	writeTitledEntry(t, graphDir)
	h := handlers.New(handlers.Options{
		GraphDir: graphDir, Reader: &graphReader{}, Committer: &recordingCommitter{err: errors.New("signing timed out")},
	})
	title := "Hand title"
	args := commitArgs(t, h.Summarize(context.Background(), &command.SummarizeCmd{EntryIDs: []string{titledEntry}, ExplicitTitle: &title}))
	want := []string{"commit", "-m", "sdd: summarize " + titledEntry + " (manual)", "--", entryPath(t, graphDir, titledEntry)}
	if !slices.Equal(args, want) {
		t.Errorf("the named command should commit exactly the written file, got %q", args)
	}
}

func TestSummarize_NamedOnlyKeepsTheOtherField(t *testing.T) {
	cases := []struct {
		name                   string
		only                   model.DescriptionFields
		purpose                llm.Purpose
		wantTitle, wantSummary string
	}{
		{name: "title", only: model.DescriptionFields{Title: true}, purpose: llm.PurposeTitle, wantTitle: storedTitle, wantSummary: "Kept summary."},
		{name: "summary", only: model.DescriptionFields{Summary: true}, purpose: llm.PurposeSummarize, wantTitle: "Kept title", wantSummary: "GENERATED"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			graphDir := t.TempDir()
			writeTitledEntry(t, graphDir)
			runner := &stubRunner{text: "GENERATED"}
			h := handlers.New(handlers.Options{GraphDir: graphDir, Reader: &graphReader{}, Committer: &recordingCommitter{}, LLMRunner: runner})
			if err := h.Summarize(context.Background(), &command.SummarizeCmd{EntryIDs: []string{titledEntry}, Only: tc.only}); err != nil {
				t.Fatalf("Summarize: %v", err)
			}
			if runner.callCount() != 1 || runner.purposeCount(tc.purpose) != 1 {
				t.Errorf("one call of the expected purpose should run, got %v", runner.byUse)
			}
			entry, err := model.ParseEntry(titledEntry+".md", readEntryFile(t, graphDir, titledEntry))
			if err != nil {
				t.Fatal(err)
			}
			if entry.Title != tc.wantTitle || entry.Summary != tc.wantSummary {
				t.Errorf("only the named field should be regenerated, got title %q summary %q", entry.Title, entry.Summary)
			}
		})
	}
}

// A title the run discards is never re-run, even when it repeats another
// entry's title.
func TestSummarize_OnlySummaryRunsNoTitleRerun(t *testing.T) {
	graphDir := t.TempDir()
	writeEntryFile(t, graphDir, emptyEntry, "---\ntype: signal\nlayer: process\nkind: gap\n---\n\nNeeds a summary.\n")
	writeEntryFile(t, graphDir, titledEntry, "---\ntype: signal\nlayer: process\nkind: gap\ntitle: "+storedTitle+"\nsummary: Kept summary.\n---\n\nHolds the repeated title.\n")
	runner := &stubRunner{text: "GENERATED"}
	h := handlers.New(handlers.Options{GraphDir: graphDir, Reader: &graphReader{}, Committer: &recordingCommitter{}, LLMRunner: runner})
	if err := h.Summarize(context.Background(), &command.SummarizeCmd{EntryIDs: []string{emptyEntry}, Only: model.DescriptionFields{Summary: true}}); err != nil {
		t.Fatalf("Summarize: %v", err)
	}
	if runner.callCount() != 1 {
		t.Errorf("only the summary call should run, got %v", runner.byUse)
	}
	if text := readEntryFile(t, graphDir, emptyEntry); strings.Contains(text, "title:") || !strings.Contains(text, "summary: GENERATED") {
		t.Errorf("the entry should gain the summary alone:\n%s", text)
	}
}

func TestSummarize_AllOnlyFillsTheNamedField(t *testing.T) {
	const titledUnsummarized = "20260101-120003-s-prc-ddd"
	type fields struct{ title, summary string }
	cases := []struct {
		name  string
		force bool
		only  model.DescriptionFields
		want  map[string]fields
	}{
		{name: "titles", only: model.DescriptionFields{Title: true}, want: map[string]fields{
			summarizedEntry: {storedTitle, "Existing summary."}, emptyEntry: {storedTitle, ""},
			titledUnsummarized: {"Hand title", ""}, titledEntry: {"Kept title", "Kept summary."},
		}},
		{name: "summaries", only: model.DescriptionFields{Summary: true}, want: map[string]fields{
			summarizedEntry: {"", "Existing summary."}, emptyEntry: {"", "GENERATED"},
			titledUnsummarized: {"Hand title", "GENERATED"}, titledEntry: {"Kept title", "Kept summary."},
		}},
		{name: "forced titles", force: true, only: model.DescriptionFields{Title: true}, want: map[string]fields{
			summarizedEntry: {storedTitle, "Existing summary."}, emptyEntry: {storedTitle, ""},
			titledUnsummarized: {storedTitle, ""}, titledEntry: {storedTitle, "Kept summary."},
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			graphDir := t.TempDir()
			writeSummarizePair(t, graphDir)
			writeTitledEntry(t, graphDir)
			writeEntryFile(t, graphDir, titledUnsummarized, "---\ntype: signal\nlayer: process\nkind: gap\ntitle: Hand title\n---\n\nTitled by hand.\n")
			h := handlers.New(handlers.Options{GraphDir: graphDir, Reader: &graphReader{}, Committer: &recordingCommitter{}, LLMRunner: &stubRunner{text: "GENERATED"}})
			if err := h.Summarize(context.Background(), &command.SummarizeCmd{Force: tc.force, Only: tc.only, Concurrency: 1}); err != nil {
				t.Fatalf("Summarize: %v", err)
			}
			for id, want := range tc.want {
				entry, err := model.ParseEntry(id+".md", readEntryFile(t, graphDir, id))
				if err != nil {
					t.Fatal(err)
				}
				if entry.Title != want.title || entry.Summary != want.summary {
					t.Errorf("%s should get only the named field, got title %q summary %q", id, entry.Title, entry.Summary)
				}
			}
		})
	}
}

func TestSummarize_All_KeepsAStoredTitle(t *testing.T) {
	graphDir := t.TempDir()
	writeEntryFile(t, graphDir, emptyEntry, "---\ntype: signal\nlayer: process\nkind: gap\ntitle: Hand title\n---\n\nNeeds a summary.\n")

	runner := &stubRunner{text: "GENERATED"}
	h := handlers.New(handlers.Options{GraphDir: graphDir, Reader: &graphReader{}, Committer: &recordingCommitter{}, LLMRunner: runner})
	var summarized []string
	scmd := &command.SummarizeCmd{OnSummarized: func(id, _ string) { summarized = append(summarized, id) }}
	if err := h.Summarize(context.Background(), scmd); err != nil {
		t.Fatalf("Summarize: %v", err)
	}

	if runner.purposeCount(llm.PurposeSummarize) != 1 || runner.callCount() != 1 {
		t.Errorf("one paired call should write the missing summary, got %v", runner.byUse)
	}
	if len(summarized) != 1 || summarized[0] != emptyEntry {
		t.Errorf("the entry should be reported summarized, got %v", summarized)
	}
	if text := readEntryFile(t, graphDir, emptyEntry); !strings.Contains(text, "title: Hand title") || !strings.Contains(text, "summary: GENERATED") {
		t.Errorf("the fill should add the summary and keep the stored title:\n%s", text)
	}
}

// TestSummarize_AllForce_RegeneratesAll covers `--all --force`: every
// non-embedded entry is regenerated regardless of an existing summary.
func TestSummarize_AllForce_RegeneratesAll(t *testing.T) {
	graphDir := t.TempDir()
	writeSummarizePair(t, graphDir)

	runner := &stubRunner{text: "GENERATED", numbered: true}
	h := handlers.New(handlers.Options{
		GraphDir:  graphDir,
		Reader:    &graphReader{},
		Committer: &recordingCommitter{},
		LLMRunner: runner,
	})

	var skipped []string
	scmd := &command.SummarizeCmd{
		Force:     true,
		OnSkipped: func(id string) { skipped = append(skipped, id) },
	}
	if err := h.Summarize(context.Background(), scmd); err != nil {
		t.Fatalf("Summarize: %v", err)
	}

	if runner.callCount() != 2 {
		t.Errorf("runner calls = %d, want 2", runner.callCount())
	}
	if len(skipped) != 0 {
		t.Errorf("skipped = %v, want none with --force", skipped)
	}
	if runner.purposeCount(llm.PurposeTitle) != 0 {
		t.Errorf("--force should regenerate both fields in paired calls, got %v", runner.byUse)
	}
	for _, id := range []string{summarizedEntry, emptyEntry} {
		if text := readEntryFile(t, graphDir, id); !strings.Contains(text, "summary: GENERATED") || !strings.Contains(text, "title: "+storedTitle) {
			t.Errorf("%s should regenerate summary and title:\n%s", id, text)
		}
	}
}

// TestSummarize_NamedEntry_RegeneratesUnconditionally covers `sdd summarize
// <id>`: a named entry regenerates even when it already has a summary and even
// without --force.
func TestSummarize_NamedEntry_RegeneratesUnconditionally(t *testing.T) {
	graphDir := t.TempDir()
	writeSummarizePair(t, graphDir)

	runner := &stubRunner{text: "GENERATED"}
	h := handlers.New(handlers.Options{
		GraphDir:  graphDir,
		Reader:    &graphReader{},
		Committer: &recordingCommitter{},
		LLMRunner: runner,
	})

	var summarized, skipped []string
	scmd := &command.SummarizeCmd{
		EntryIDs:     []string{summarizedEntry},
		OnSummarized: func(id, _ string) { summarized = append(summarized, id) },
		OnSkipped:    func(id string) { skipped = append(skipped, id) },
	}
	if err := h.Summarize(context.Background(), scmd); err != nil {
		t.Fatalf("Summarize: %v", err)
	}

	if runner.callCount() != 1 {
		t.Errorf("runner calls = %d, want 1", runner.callCount())
	}
	if len(summarized) != 1 || summarized[0] != summarizedEntry {
		t.Errorf("summarized = %v, want [%s]", summarized, summarizedEntry)
	}
	if len(skipped) != 0 {
		t.Errorf("skipped = %v, want none for a named entry", skipped)
	}
	if text := readEntryFile(t, graphDir, summarizedEntry); !strings.Contains(text, "summary: GENERATED") || !strings.Contains(text, "title: "+storedTitle) {
		t.Errorf("named entry should regenerate summary and title:\n%s", text)
	}
}

// failingTitleRunner answers the paired call and fails the title-only call,
// so one entry of a batch fails while another succeeds.
type pathRecordingCommitter struct{ paths []string }

func (c *pathRecordingCommitter) Commit(_ string, paths ...string) error {
	c.paths = append(c.paths, paths...)
	return nil
}

type failingTitleRunner struct{ stubRunner }

func (r *failingTitleRunner) Run(ctx context.Context, request llm.Request) (llm.Result, error) {
	if request.Purpose == llm.PurposeTitle {
		return llm.Result{}, errors.New("title route unavailable")
	}
	return r.stubRunner.Run(ctx, request)
}

func TestSummarize_All_CommitsWhatItWroteBeforeAFailure(t *testing.T) {
	graphDir := t.TempDir()
	writeSummarizePair(t, graphDir)

	committer := &pathRecordingCommitter{}
	h := handlers.New(handlers.Options{
		GraphDir:  graphDir,
		Reader:    &graphReader{},
		Committer: committer,
		LLMRunner: &failingTitleRunner{stubRunner{text: "GENERATED"}},
	})
	err := h.Summarize(context.Background(), &command.SummarizeCmd{})
	if err == nil || !strings.Contains(err.Error(), "title route unavailable") {
		t.Fatalf("a failed entry should fail the run, got %v", err)
	}
	if !strings.Contains(readEntryFile(t, graphDir, emptyEntry), "summary: GENERATED") {
		t.Fatal("the entry that succeeded should be written")
	}
	emptyRel, _ := model.IDToRelPath(emptyEntry)
	if len(committer.paths) != 1 || committer.paths[0] != filepath.Join(graphDir, emptyRel) {
		t.Errorf("what the run wrote should be committed despite the failure, got %v", committer.paths)
	}
}

func TestSummarize_All_PlansWhatItFills(t *testing.T) {
	graphDir := t.TempDir()
	writeSummarizePair(t, graphDir)
	writeTitledEntry(t, graphDir)
	h := handlers.New(handlers.Options{GraphDir: graphDir, Reader: &graphReader{}, Committer: &recordingCommitter{}, LLMRunner: &stubRunner{text: "GENERATED", numbered: true}})

	planned := -1
	if err := h.Summarize(context.Background(), &command.SummarizeCmd{OnPlanned: func(n int) { planned = n }}); err != nil {
		t.Fatalf("Summarize: %v", err)
	}
	if planned != 2 {
		t.Errorf("the plan should count only the entries the run fills, got %d", planned)
	}
}
