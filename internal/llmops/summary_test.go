package llmops_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/networkteam/sdd/internal/llmops"
	"github.com/networkteam/sdd/internal/model"
	"github.com/networkteam/sdd/pkg/llm"
)

// scriptedRunner answers each call from a script and records what it was asked.
type scriptedRunner struct {
	replies  []string
	requests []llm.Request
}

func (r *scriptedRunner) Run(_ context.Context, req llm.Request) (llm.Result, error) {
	r.requests = append(r.requests, req)
	i := len(r.requests) - 1
	if i >= len(r.replies) {
		return llm.Result{}, fmt.Errorf("scripted runner exhausted after %d calls", len(r.replies))
	}
	return llm.Result{Text: r.replies[i], Identity: llm.Identity{Provider: "scripted", Model: "m"}}, nil
}

func titledFixture() (*model.Entry, *model.Graph) {
	related := &model.Entry{
		ID: "20260101-100000-s-tac-rel", Type: model.TypeSignal, Kind: model.KindGap, Layer: model.LayerTactical,
		Title: "Daemon project removal", Summary: "The daemon project is slated for removal.", Content: "Remove it.",
	}
	entry := &model.Entry{
		ID: "20260102-100000-d-tac-ent", Type: model.TypeDecision, Kind: model.KindDirective, Layer: model.LayerTactical,
		Refs:    []model.Ref{{ID: related.ID, Kind: model.RefKindAddresses, Desc: "the ref description"}},
		Summary: "A stored summary that may have been corrected.",
		Content: "The daemon project goes.",
	}
	return entry, model.NewGraph([]*model.Entry{related, entry})
}

func untitledCopy(entry *model.Entry) *model.Entry {
	untitled := *entry
	untitled.Title = ""
	return &untitled
}

func unsummarizedCopy(entry *model.Entry) *model.Entry {
	unsummarized := *entry
	unsummarized.Summary = ""
	unsummarized.Title = ""
	return &unsummarized
}

func TestNewDescriptionChoosesTheCall(t *testing.T) {
	entry, graph := titledFixture()
	cases := []struct {
		name          string
		entry         *model.Entry
		regenerate    bool
		wantPurpose   llm.Purpose
		wantTitleOnly bool
	}{
		{name: "no summary", entry: unsummarizedCopy(entry), wantPurpose: llm.PurposeSummarize},
		{name: "summary without title", entry: untitledCopy(entry), wantPurpose: llm.PurposeTitle, wantTitleOnly: true},
		{name: "summary without title, regenerated", entry: untitledCopy(entry), regenerate: true, wantPurpose: llm.PurposeSummarize},
		{name: "titled and summarized, regenerated", entry: entry, regenerate: true, wantPurpose: llm.PurposeSummarize},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			description, err := llmops.NewDescription(tc.entry, graph, "", tc.regenerate, nil)
			if err != nil {
				t.Fatalf("NewDescription: %v", err)
			}
			if description.Request.Purpose != tc.wantPurpose || description.TitleOnly() != tc.wantTitleOnly {
				t.Errorf("the entry should get the call its state needs, got purpose %q, title only %v", description.Request.Purpose, description.TitleOnly())
			}
		})
	}
}

func TestNewDescriptionRefusesADescribedEntry(t *testing.T) {
	entry, graph := titledFixture()
	entry.Title = "Daemon removal"
	_, err := llmops.NewDescription(entry, graph, "", false, nil)
	if !errors.Is(err, llmops.ErrDescribed) {
		t.Fatalf("an entry holding title and summary should be refused unless regenerated, got %v", err)
	}
}

func TestRenderSummaryPromptShowsRelatedTitles(t *testing.T) {
	entry, graph := titledFixture()
	description, err := llmops.NewDescription(unsummarizedCopy(entry), graph, "", false, nil)
	if err != nil {
		t.Fatalf("NewDescription: %v", err)
	}
	req := description.Request
	if !strings.Contains(req.UserPrompt, "Title: Daemon project removal") {
		t.Errorf("related entries should carry their titles, so a differing subject can be named apart:\n%s", req.UserPrompt)
	}
	if !strings.Contains(req.UserPrompt, entry.Content) {
		t.Errorf("the paired call should read the full entry:\n%s", req.UserPrompt)
	}
}

func TestRenderTitlePromptCarriesTheStoredSummary(t *testing.T) {
	entry, graph := titledFixture()
	related := graph.ByID["20260101-100000-s-tac-rel"]
	description, err := llmops.NewDescription(untitledCopy(entry), graph, "de", false, nil)
	if err != nil {
		t.Fatalf("NewDescription: %v", err)
	}
	combined := description.Request.Combined()
	for _, want := range []string{entry.Summary, "Type: decision", "Layer: tactical", "Kind: directive", related.Title, "`de`"} {
		if !strings.Contains(combined, want) {
			t.Errorf("the title call should carry %q:\n%s", want, combined)
		}
	}
	for _, unwanted := range []string{entry.Content, related.Summary, related.Content, "the ref description"} {
		if strings.Contains(combined, unwanted) {
			t.Errorf("the title call should be fed the stored summary only, not %q:\n%s", unwanted, combined)
		}
	}
}

// Both calls state the title rule and constrain their answer through the
// request's response schema, so neither prompt teaches an answer format.
func TestDescriptionsStateTheTitleRule(t *testing.T) {
	entry, graph := titledFixture()
	for name, candidate := range map[string]*model.Entry{"title": untitledCopy(entry), "summary": unsummarizedCopy(entry)} {
		t.Run(name, func(t *testing.T) {
			description, err := llmops.NewDescription(candidate, graph, "", false, nil)
			if err != nil {
				t.Fatalf("NewDescription: %v", err)
			}
			prompt := description.Request.SystemPrompt
			for _, want := range []string{model.TitleRule, "no trailing punctuation but a question mark"} {
				if !strings.Contains(prompt, want) {
					t.Errorf("the prompt should say %q:\n%s", want, prompt)
				}
			}
			combined := description.Request.Combined()
			for _, rule := range []string{`{"title"`, "JSON object", "code fence"} {
				if strings.Contains(combined, rule) {
					t.Errorf("the prompt should teach no answer format, found %q:\n%s", rule, combined)
				}
			}
		})
	}
}

// A title repeating any other entry's title is re-run (Retitle), so neither
// prompt may allow reusing a related entry's title.
func TestDescriptionsForbidReusingARelatedTitle(t *testing.T) {
	entry, graph := titledFixture()
	for name, candidate := range map[string]*model.Entry{"title": untitledCopy(entry), "summary": unsummarizedCopy(entry)} {
		t.Run(name, func(t *testing.T) {
			description, err := llmops.NewDescription(candidate, graph, "", false, nil)
			if err != nil {
				t.Fatalf("NewDescription: %v", err)
			}
			prompt := description.Request.SystemPrompt
			if !strings.Contains(prompt, "Never reuse a related entry's title") || strings.Contains(prompt, "Reuse a related entry's title only") {
				t.Errorf("the prompt should forbid reusing a related entry's title:\n%s", prompt)
			}
		})
	}
}

// readSchema reads a response schema file as it ships in the binary.
func readSchema(t *testing.T, name string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("response_schemas", name))
	if err != nil {
		t.Fatal(err)
	}
	var schema map[string]any
	if err := json.Unmarshal(data, &schema); err != nil {
		t.Fatalf("%s is not valid JSON: %v", name, err)
	}
	return schema
}

// The title rule lives in model.TitleRule and reaches the model through the
// prompt; the schema's field description only points at it.
func TestTitleSchemasPointAtTheTitleRule(t *testing.T) {
	for _, name := range []string{"summary.json", "title.json"} {
		t.Run(name, func(t *testing.T) {
			title, _ := readSchema(t, name)["properties"].(map[string]any)["title"].(map[string]any)
			description, _ := title["description"].(string)
			if !strings.Contains(description, "title rule") {
				t.Errorf("the title field should point at the title rule, got %q", description)
			}
			if strings.Contains(description, "words") || strings.Contains(description, "characters") {
				t.Errorf("the title field should not state its own length guidance, got %q", description)
			}
		})
	}
}

// Each call constrains its answer to the fields its parser reads; the
// title-only call must not be asked for a summary it would discard.
func TestDescriptionsDeclareTheirSchema(t *testing.T) {
	entry, graph := titledFixture()
	for _, tc := range []struct {
		name     string
		entry    *model.Entry
		file     string
		required []any
	}{
		{name: "summary", entry: unsummarizedCopy(entry), file: "summary.json", required: []any{"title", "summary"}},
		{name: "title", entry: untitledCopy(entry), file: "title.json", required: []any{"title"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			description, err := llmops.NewDescription(tc.entry, graph, "", false, nil)
			if err != nil {
				t.Fatalf("NewDescription: %v", err)
			}
			var sent map[string]any
			if err := json.Unmarshal(description.Request.ResponseSchema, &sent); err != nil {
				t.Fatalf("the request should carry a JSON schema: %v", err)
			}
			if fmt.Sprint(sent) != fmt.Sprint(readSchema(t, tc.file)) {
				t.Errorf("the request should carry %s, got %s", tc.file, description.Request.ResponseSchema)
			}
			if got, _ := sent["required"].([]any); !slices.Equal(got, tc.required) {
				t.Errorf("the schema should require exactly the fields the parser reads, got %v", got)
			}
		})
	}
}

func TestParseSummaryResponse(t *testing.T) {
	entry, graph := titledFixture()
	description, err := llmops.NewDescription(unsummarizedCopy(entry), graph, "", false, nil)
	if err != nil {
		t.Fatalf("NewDescription: %v", err)
	}
	cases := []struct {
		name        string
		output      string
		wantTitle   string
		wantSummary string
		wantErr     string
	}{
		{name: "paired answer", output: `{"title": "daemon project removal", "summary": "The daemon project is removed."}`, wantTitle: "Daemon project removal", wantSummary: "The daemon project is removed."},
		{name: "answer wrapped in prose", output: "Here it is:\n```json\n{\"title\": \"daemon project removal\", \"summary\": \"The daemon project is removed.\"}\n```", wantTitle: "Daemon project removal", wantSummary: "The daemon project is removed."},
		{name: "trailing period dropped from the title", output: `{"title": "daemon project removal.", "summary": "S."}`, wantTitle: "Daemon project removal", wantSummary: "S."},
		{name: "a question mark kept", output: `{"title": "should the daemon go?", "summary": "S."}`, wantTitle: "Should the daemon go?", wantSummary: "S."},
		{name: "quotation marks removed", output: `{"title": "the \"daemon\" removal", "summary": "S."}`, wantTitle: "The daemon removal", wantSummary: "S."},
		{name: "only punctuation", output: `{"title": " … ", "summary": "S."}`, wantErr: "title is empty"},
		{name: "plain prose", output: "The daemon project is removed.", wantErr: "no JSON object"},
		{name: "empty title", output: `{"title": " ", "summary": "S."}`, wantErr: "title is empty"},
		{name: "multi-line title", output: `{"title": "daemon\nremoval", "summary": "S."}`, wantErr: "more than one line"},
		{name: "empty summary", output: `{"title": "daemon project removal", "summary": ""}`, wantErr: "empty summary"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := description.Parse(tc.output)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("the answer should be refused naming its defect, got %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			if got.Title != tc.wantTitle || got.Summary != tc.wantSummary {
				t.Errorf("title and summary should be read from the answer, got %+v", got)
			}
		})
	}
}

func TestParseTitleResponse(t *testing.T) {
	entry, graph := titledFixture()
	description, err := llmops.NewDescription(untitledCopy(entry), graph, "", false, nil)
	if err != nil {
		t.Fatalf("NewDescription: %v", err)
	}
	got, err := description.Parse(`{"title": " daemon project removal "}`)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if got.Title != "Daemon project removal" || got.Summary != "" {
		t.Errorf("the title should be read trimmed and capitalized, leaving the summary alone, got %+v", got)
	}
	if _, err := description.Parse(`{"title": ""}`); err == nil {
		t.Error("an empty title should be refused")
	}
}

func TestDescribeGivesTitleAndSummaryFromOneCall(t *testing.T) {
	entry, graph := titledFixture()
	runner := &scriptedRunner{replies: []string{`{"title": "daemon project shutdown", "summary": "The daemon project is removed."}`}}
	got, err := llmops.Describe(context.Background(), runner, unsummarizedCopy(entry), graph, "", false, nil)
	if err != nil {
		t.Fatalf("Describe: %v", err)
	}
	if len(runner.requests) != 1 || got.Title != "Daemon project shutdown" || got.Summary != "The daemon project is removed." {
		t.Errorf("an entry without a summary should get both from one call, got %+v after %d calls", got, len(runner.requests))
	}
}

const similarHeading = "## Titles of the most similar entries"

const similarRule = "The new title must not conflict with any of these titles: it must not repeat one or be mistakable for one. Name what sets this entry apart."

// listedSimilarTitles reads the similar-titles list out of a prompt, nil when
// the prompt has no such section.
func listedSimilarTitles(prompt string) []string {
	_, section, ok := strings.Cut(prompt, similarHeading)
	if !ok {
		return nil
	}
	var titles []string
	for _, line := range strings.Split(section, "\n") {
		if title, ok := strings.CutPrefix(line, "- "); ok {
			titles = append(titles, title)
		}
	}
	return titles
}

func TestDescriptionsListSimilarTitles(t *testing.T) {
	entry, graph := titledFixture()
	similar := []string{"Search index layout", "Title backfill batches"}
	for name, candidate := range map[string]*model.Entry{"title": untitledCopy(entry), "summary": unsummarizedCopy(entry)} {
		t.Run(name, func(t *testing.T) {
			with, err := llmops.NewDescription(candidate, graph, "", false, similar)
			if err != nil {
				t.Fatalf("NewDescription: %v", err)
			}
			prompt := with.Request.UserPrompt
			if !strings.Contains(prompt, similarRule) {
				t.Errorf("the prompt should forbid a title conflicting with the similar ones:\n%s", prompt)
			}
			if got := listedSimilarTitles(prompt); !slices.Equal(got, similar) {
				t.Errorf("the prompt should list the similar titles in rank order, got %q:\n%s", got, prompt)
			}
			if !strings.Contains(prompt, "Daemon project removal") {
				t.Errorf("the linked entries should still be listed apart:\n%s", prompt)
			}

			without, err := llmops.NewDescription(candidate, graph, "", false, nil)
			if err != nil {
				t.Fatalf("NewDescription: %v", err)
			}
			combined := without.Request.Combined()
			if strings.Contains(combined, similarHeading) || strings.Contains(combined, similarRule) {
				t.Errorf("without similar titles the prompt should carry neither their section nor their rule:\n%s", combined)
			}
		})
	}
}

// repeatFixture is a graph whose entries hold titles the description's answer
// can repeat; the entry described holds the title "Index segment cache".
func repeatFixture() (*model.Entry, *model.Graph) {
	entry, graph := titledFixture()
	entry.Title = "Index segment cache"
	other := &model.Entry{
		ID: "20260103-100000-s-tac-oth", Type: model.TypeSignal, Kind: model.KindGap, Layer: model.LayerTactical,
		Title: "Daemon shutdown order", Summary: "The daemon stops in the wrong order.", Content: "Order matters.",
	}
	return entry, model.NewGraph(append(slices.Clone(graph.Entries), other))
}

func TestDescribeRerunsARepeatedTitleOnce(t *testing.T) {
	previous := []string{"First", "Second", "Third", "Fourth", "Fifth"}
	cases := []struct {
		name      string
		similar   []string
		replies   []string
		wantList  []string
		wantTitle string
	}{
		{
			name:      "repeated title outside the list",
			similar:   previous,
			replies:   []string{`{"title": " daemon SHUTDOWN order ", "summary": "The daemon stops first."}`, `{"title": "Daemon stop sequence"}`},
			wantList:  []string{"Daemon shutdown order", "First", "Second", "Third", "Fourth"},
			wantTitle: "Daemon stop sequence",
		},
		{
			name:      "repeated title inside the list",
			similar:   []string{"First", "Daemon shutdown order", "Third", "Fourth", "Fifth"},
			replies:   []string{`{"title": "Daemon shutdown order", "summary": "The daemon stops first."}`, `{"title": "Daemon stop sequence"}`},
			wantList:  []string{"Daemon shutdown order", "First", "Third", "Fourth", "Fifth"},
			wantTitle: "Daemon stop sequence",
		},
		{
			name:      "re-run repeats again",
			similar:   previous,
			replies:   []string{`{"title": "Daemon shutdown order", "summary": "The daemon stops first."}`, `{"title": "daemon project removal"}`},
			wantList:  []string{"Daemon shutdown order", "First", "Second", "Third", "Fourth"},
			wantTitle: "Daemon project removal",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			entry, graph := repeatFixture()
			runner := &scriptedRunner{replies: tc.replies}
			got, err := llmops.Describe(context.Background(), runner, unsummarizedCopy(entry), graph, "", false, tc.similar)
			if err != nil {
				t.Fatalf("Describe: %v", err)
			}
			if len(runner.requests) != 2 {
				t.Fatalf("a repeated title should be re-run exactly once, got %d calls", len(runner.requests))
			}
			rerun := runner.requests[1]
			if rerun.Purpose != llm.PurposeTitle {
				t.Errorf("the re-run should be the title-only call, got %q", rerun.Purpose)
			}
			if !strings.Contains(rerun.UserPrompt, "The daemon stops first.") {
				t.Errorf("the re-run should title the summary the paired call wrote:\n%s", rerun.UserPrompt)
			}
			if list := listedSimilarTitles(rerun.UserPrompt); !slices.Equal(list, tc.wantList) {
				t.Errorf("the re-run should list the repeated title first, then the earlier titles without the last, got %q", list)
			}
			if got.Title != tc.wantTitle || got.Summary != "The daemon stops first." {
				t.Errorf("the re-run's title should be stored beside the paired call's summary, got %+v", got)
			}
		})
	}
}

func TestDescribeKeepsATitleNoOtherEntryHolds(t *testing.T) {
	cases := []struct {
		name       string
		entry      func(*model.Entry) *model.Entry
		regenerate bool
		reply      string
	}{
		{name: "free title", entry: unsummarizedCopy, reply: `{"title": "Daemon stop sequence", "summary": "S."}`},
		{name: "the entry's own title", entry: func(e *model.Entry) *model.Entry { return e }, regenerate: true, reply: `{"title": "index segment cache", "summary": "S."}`},
		{name: "a title the caller keeps", entry: func(e *model.Entry) *model.Entry {
			kept := unsummarizedCopy(e)
			kept.Title = "Caller title"
			return kept
		}, reply: `{"title": "Daemon shutdown order", "summary": "S."}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			entry, graph := repeatFixture()
			runner := &scriptedRunner{replies: []string{tc.reply, `{"title": "Another title"}`}}
			_, err := llmops.Describe(context.Background(), runner, tc.entry(entry), graph, "", tc.regenerate, []string{"First"})
			if err != nil {
				t.Fatalf("Describe: %v", err)
			}
			if len(runner.requests) != 1 {
				t.Errorf("a title repeating no other entry's title should not be re-run, got %d calls", len(runner.requests))
			}
		})
	}
}

// A summary drifts when it carries a claim the entry never made, often one
// lent from a related entry; the prompt holds every claim to the entry.
func TestPairedDescriptionClaimsOnlyWhatTheEntrySays(t *testing.T) {
	entry, graph := titledFixture()
	description, err := llmops.NewDescription(unsummarizedCopy(entry), graph, "", false, nil)
	if err != nil {
		t.Fatalf("NewDescription: %v", err)
	}
	want := "Claim only what the entry itself states: add no cause, consequence, actor or example it does not name, and never lend it a related entry's claims."
	if !strings.Contains(description.Request.SystemPrompt, want) {
		t.Errorf("the prompt should hold every summary claim to the entry:\n%s", description.Request.SystemPrompt)
	}
}
