package cliapp_test

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const appEntryRel = "decisions/2026/09/14-020000-s-tac-cli.md"

// newFillFixture is the app fixture in a git repository, its one entry
// summarized and untitled, its LLM a local listener answering every call with
// a title and a summary.
func newFillFixture(t *testing.T) *appFixture {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/generate" {
			_, _ = w.Write([]byte(`{"models":[]}`))
			return
		}
		_, _ = w.Write([]byte(`{"model":"m","response":"{\"title\":\"filled title\",\"summary\":\"A generated summary.\"}","done":true}`))
	}))
	t.Cleanup(srv.Close)

	f := newAppFixture(t)
	f.write(t, ".sdd/config.yaml", "repo_id: example.test/cli\ngraph_dir: decisions\ndefault_branch: main\nllm:\n  provider: ollama\n  model: m\n  ollama_endpoint: "+srv.URL+"\n")
	runGit(t, f.root, "init", "-q", "-b", "main")
	runGit(t, f.root, "config", "user.email", "test@example.com")
	runGit(t, f.root, "config", "user.name", "Test")
	runGit(t, f.root, "config", "commit.gpgsign", "false")
	runGit(t, f.root, "add", ".sdd", "decisions")
	runGit(t, f.root, "commit", "-q", "-m", "graph")
	return f
}

func (f *appFixture) read(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(f.root, filepath.FromSlash(name)))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestSummarizeCostLine(t *testing.T) {
	cases := []struct {
		name         string
		args         []string
		wantCostLine bool
		wantLine     string
		wantSummary  string
	}{
		{name: "a fill states its cost and keeps the stored summary", args: []string{"summarize", "--all"}, wantCostLine: true, wantLine: "  titled " + appEntryID, wantSummary: "summary: " + appSummary},
		{name: "--force regenerates without a fill's cost line", args: []string{"summarize", "--all", "--force"}, wantLine: "  summarized " + appEntryID, wantSummary: "summary: A generated summary."},
		{name: "a named entry regenerates without a fill's cost line", args: []string{"summarize", appEntryID}, wantLine: "  summarized " + appEntryID, wantSummary: "summary: A generated summary."},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newFillFixture(t)
			result := f.run(t, tc.args...)
			if result.err != nil {
				t.Fatalf("sdd summarize: %v\n%s", result.err, result.stderr)
			}
			costLine := "Filling: 1 entries get a title beside their stored summary, 0 entries get a summary beside their stored title, 0 entries get a summary and a title -> 1 model calls."
			if strings.Contains(result.stderr, costLine) != tc.wantCostLine {
				t.Errorf("only a fill should state its cost before it runs:\n%s", result.stderr)
			}
			if !strings.Contains(result.stderr, tc.wantLine) {
				t.Errorf("off a terminal each filled entry should get its own line:\n%s", result.stderr)
			}
			entry := f.read(t, appEntryRel)
			if !strings.Contains(entry, "title: Filled title") || !strings.Contains(entry, tc.wantSummary) {
				t.Errorf("the entry should gain its title beside the expected summary:\n%s", entry)
			}
			if status := runGit(t, f.root, "status", "--porcelain", "--", "decisions"); status != "" {
				t.Errorf("the filled entry should be committed:\n%s", status)
			}
		})
	}
}

func TestSummarizeCostLineCountsAStoredTitle(t *testing.T) {
	f := newFillFixture(t)
	f.write(t, appEntryRel, strings.Replace(f.read(t, appEntryRel), "summary: "+appSummary, "title: Hand title", 1))
	runGit(t, f.root, "commit", "-q", "-am", "titled by hand")

	result := f.run(t, "summarize", "--all")
	if result.err != nil {
		t.Fatalf("summarize --all: %v\n%s", result.err, result.stderr)
	}
	costLine := "Filling: 0 entries get a title beside their stored summary, 1 entries get a summary beside their stored title, 0 entries get a summary and a title -> 1 model calls."
	if !strings.Contains(result.stderr, costLine) {
		t.Errorf("the cost line should count a titled entry as getting its summary alone:\n%s", result.stderr)
	}
	if entry := f.read(t, appEntryRel); !strings.Contains(entry, "title: Hand title") || !strings.Contains(entry, "summary: A generated summary.") {
		t.Errorf("the fill should add the summary beside the stored title:\n%s", entry)
	}
}

func TestSummarizeTitleNeedsOneEntry(t *testing.T) {
	f := newAppFixture(t)
	cases := []struct {
		name string
		args []string
		want string
	}{
		{name: "--all", args: []string{"summarize", "--title", "A title", "--all"}, want: "--title cannot be combined with --all"},
		{name: "two entries", args: []string{"summarize", "--title", "A title", appEntryID, appEntryID}, want: "--title requires exactly one entry ID"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if result := f.run(t, tc.args...); result.err == nil || !strings.Contains(result.err.Error(), tc.want) {
				t.Errorf("--title should need exactly one entry, got %v", result.err)
			}
		})
	}
}
