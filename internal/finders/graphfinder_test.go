package finders

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/networkteam/sdd/internal/model"
	"gopkg.in/yaml.v3"
)

// corpus is one fixture entry expressed as its full ID and canonical markdown.
// The parity test feeds the same corpus through both a filesystem source (raw
// bytes) and a structured source (decoded frontmatter + body) and asserts the
// two graphs are indistinguishable.
type corpusEntry struct {
	id       string
	markdown string
}

var parityCorpus = []corpusEntry{
	{
		id: "20260101-100000-d-tac-aaa",
		markdown: "---\ntype: decision\nlayer: tactical\nkind: directive\n" +
			"summary: A clean directive.\n---\n\nBody of the directive.\n",
	},
	{
		// A done signal whose only closes target does not exist: a dangling-ref
		// warning produced by the shared validation path — proves warnings ride
		// identically through both sources.
		id: "20260101-110000-s-tac-bbb",
		markdown: "---\ntype: signal\nlayer: tactical\nkind: done\n" +
			"summary: Closes a phantom.\ncloses:\n  - 20260101-100000-d-tac-zzz\n---\n\nDone body.\n",
	},
	{
		// An actor signal missing its required canonical field: another
		// warning-producing shape from the graph-level validators.
		id: "20260101-120000-s-prc-ccc",
		markdown: "---\ntype: signal\nlayer: process\nkind: actor\n" +
			"summary: Nameless actor.\n---\n\nActor body.\n",
	},
}

// structuredCorpusSource is an in-test structured DocumentSource: it hands the
// corpus as decoded frontmatter + body, the way application.SnapshotData (and a
// future DB) does — the counterpart to the filesystem source's raw bytes.
type structuredCorpusSource struct {
	entries []corpusEntry
}

func (s structuredCorpusSource) GraphDocuments() (GraphDocuments, error) {
	var docs GraphDocuments
	for _, e := range s.entries {
		frontmatter, body := splitFrontmatter(e.markdown)
		var fm map[string]any
		if err := yaml.Unmarshal([]byte(frontmatter), &fm); err != nil {
			return GraphDocuments{}, err
		}
		docs.Entries = append(docs.Entries, EntryDocument{ID: e.id, Frontmatter: fm, Body: body})
	}
	return docs, nil
}

func splitFrontmatter(md string) (frontmatter, body string) {
	trimmed := strings.TrimPrefix(md, "---\n")
	end := strings.Index(trimmed, "\n---")
	if end < 0 {
		return "", md
	}
	return trimmed[:end+1], strings.TrimPrefix(trimmed[end+4:], "\n")
}

// TestGraphFinder_SourceParity is the load-path unification proof: the same
// corpus, loaded once through the filesystem source (raw canonical bytes) and
// once through a structured document source (decoded frontmatter + body),
// yields the same entries, the same per-entry warnings, and the same Health.
// Both flow through the one buildGraph semantic gate; only the byte encoding
// differs.
func TestGraphFinder_SourceParity(t *testing.T) {
	dir := t.TempDir()
	for _, e := range parityCorpus {
		rel, err := model.IDToRelPath(e.id)
		if err != nil {
			t.Fatalf("IDToRelPath(%s): %v", e.id, err)
		}
		path := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(e.markdown), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	fsDocs, err := filesystemSource{dir: dir}.GraphDocuments()
	if err != nil {
		t.Fatalf("filesystem source: %v", err)
	}
	fsGraph, _, err := buildGraph(fsDocs)
	if err != nil {
		t.Fatalf("buildGraph(filesystem): %v", err)
	}

	structDocs, err := structuredCorpusSource{entries: parityCorpus}.GraphDocuments()
	if err != nil {
		t.Fatalf("structured source: %v", err)
	}
	structGraph, _, err := buildGraph(structDocs)
	if err != nil {
		t.Fatalf("buildGraph(structured): %v", err)
	}

	// Same entry IDs.
	fsIDs := entryIDSet(fsGraph)
	structIDs := entryIDSet(structGraph)
	if len(fsIDs) != len(structIDs) {
		t.Fatalf("entry count: filesystem %d, structured %d", len(fsIDs), len(structIDs))
	}
	for id := range fsIDs {
		if !structIDs[id] {
			t.Errorf("entry %s present in filesystem graph but not structured", id)
		}
	}

	// Same per-entry warnings (field + message), keyed by entry ID.
	for id := range fsIDs {
		fsW := warningStrings(fsGraph.ByID[id])
		structW := warningStrings(structGraph.ByID[id])
		if strings.Join(fsW, "|") != strings.Join(structW, "|") {
			t.Errorf("warnings mismatch for %s:\n  filesystem: %v\n  structured: %v", id, fsW, structW)
		}
	}

	// Same Health summary.
	fsHealth := fsGraph.Health()
	structHealth := structGraph.Health()
	if fsHealth.Warnings != structHealth.Warnings || fsHealth.LoadErrors != structHealth.LoadErrors {
		t.Fatalf("health mismatch: filesystem %+v, structured %+v", fsHealth, structHealth)
	}
	if fsHealth.Warnings == 0 {
		t.Fatal("expected the corpus to produce warnings (dangling ref, nameless actor) — parity is only meaningful if the validation path ran")
	}
}

func entryIDSet(g *model.Graph) map[string]bool {
	out := make(map[string]bool, len(g.Entries))
	for _, e := range g.Entries {
		out[e.ID] = true
	}
	return out
}

func warningStrings(e *model.Entry) []string {
	if e == nil {
		return nil
	}
	out := make([]string, 0, len(e.Warnings))
	for _, w := range e.Warnings {
		out = append(out, w.Field+": "+w.Message)
	}
	return out
}

// TestGraphFinder_PartialRead pins that a document that fails the single parse
// gate becomes a LoadIssue rather than aborting the build: the parseable
// entries still load, and Health reports the failure.
func TestGraphFinder_PartialRead(t *testing.T) {
	docs := GraphDocuments{
		Entries: []EntryDocument{
			{
				ID: "20260101-100000-d-tac-ok",
				Raw: []byte("---\ntype: decision\nlayer: tactical\nkind: directive\n" +
					"summary: Fine.\n---\n\nBody.\n"),
			},
			{
				// Frontmatter that will not parse — an unterminated flow sequence.
				ID:  "20260101-110000-s-tac-bad",
				Raw: []byte("---\ntype: signal\ntopics: [oops\n---\n\nBody.\n"),
			},
		},
	}
	graph, _, err := buildGraph(docs)
	if err != nil {
		t.Fatalf("buildGraph must not abort on a malformed entry: %v", err)
	}
	if graph.ByID["20260101-100000-d-tac-ok"] == nil {
		t.Error("parseable entry should still load alongside a malformed one")
	}
	if _, ok := graph.ByID["20260101-110000-s-tac-bad"]; ok {
		t.Error("malformed entry must not enter the graph as a valid entry")
	}
	health := graph.Health()
	if health.LoadErrors != 1 {
		t.Fatalf("expected 1 load error, got %d (%+v)", health.LoadErrors, health.Issues)
	}
}
