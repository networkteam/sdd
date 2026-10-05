package cliapp_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

type exportEntry struct {
	ID       string   `json:"id"`
	FullID   string   `json:"full_id"`
	ShortID  string   `json:"short_id"`
	Time     string   `json:"time"`
	Status   string   `json:"status"`
	StatusBy string   `json:"status_by"`
	ClosedBy []string `json:"closed_by"`
	Closes   []string `json:"closes"`
	Summary  string   `json:"summary"`
	Body     string   `json:"body"`
	Heat     float64  `json:"heat"`
	InDegree int      `json:"in_degree"`
	Refs     []struct {
		ID, Kind, Desc string
	} `json:"refs"`
	Attachments []struct {
		Name        string `json:"name"`
		Size        int64  `json:"size"`
		ContentType string `json:"content_type"`
		Content     string `json:"content"`
		Truncated   bool   `json:"truncated"`
	} `json:"attachments"`
}

type exportDoc struct {
	Experimental bool          `json:"experimental"`
	Embedded     []exportEntry `json:"embedded"`
	Repos        []struct {
		RepoID    string `json:"repo_id"`
		Local     bool   `json:"local"`
		Revision  string `json:"revision"`
		Selection *struct {
			Mode         string `json:"mode"`
			Hops         *int   `json:"hops"`
			TotalEntries int    `json:"total_entries"`
		} `json:"selection"`
		Entries []exportEntry `json:"entries"`
		WIP     []struct {
			Entry       string `json:"entry"`
			Participant string `json:"participant"`
		} `json:"wip"`
	} `json:"repos"`
}

// runGit runs git in dir.
func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-c", "commit.gpgsign=false"}, args...)...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

// TestCLIExportJSON pins the export's derived attributes against a git repo:
// identity, status, refs, heat, attachments, WIP, and the HEAD revision, which
// stays empty before the first commit.
func TestCLIExportJSON(t *testing.T) {
	f := newAppFixture(t)
	const closerID = "20260914-030000-d-tac-dec"
	git := func(args ...string) string {
		t.Helper()
		return runGit(t, f.root, args...)
	}
	git("init", "-q", "-b", "main")
	git("config", "user.email", "test@example.com")
	git("config", "user.name", "Test")
	f.write(t, "decisions/2026/09/14-030000-d-tac-dec.md", "---\ntype: decision\nkind: directive\nintent: pending\nlayer: tactical\nconfidence: high\nparticipants: [Local Author]\nrefs:\n  - id: "+appEntryID+"\n    kind: addresses\n    desc: answers the gap\ncloses: ["+appEntryID+"]\n---\n\nThe decision.\n")
	f.write(t, "decisions/2026/09/14-030000-d-tac-dec/notes.md", strings.Repeat("A note line.\n", 2000))
	f.write(t, "decisions/wip/20260914-050000-local-author.md", "---\nentry: "+closerID+"\nparticipant: Local Author\n---\n\nFollowing up.\n")

	export := func() exportDoc {
		t.Helper()
		result := f.run(t, "export", "--format", "json")
		if result.err != nil {
			t.Fatalf("export: %v\n%s", result.err, result.stderr)
		}
		var doc exportDoc
		if err := json.Unmarshal([]byte(result.stdout), &doc); err != nil {
			t.Fatalf("export emitted invalid JSON: %v\n%s", err, result.stdout)
		}
		if !doc.Experimental || len(doc.Repos) != 1 {
			t.Fatalf("experimental = %v, repos = %d; want true, 1", doc.Experimental, len(doc.Repos))
		}
		return doc
	}

	if rev := export().Repos[0].Revision; rev != "" {
		t.Errorf("revision before the first commit = %q, want none", rev)
	}
	git("add", "decisions")
	git("commit", "-q", "-m", "capture")
	head := git("rev-parse", "HEAD")

	doc := export()
	repo := doc.Repos[0]
	if repo.RepoID != "example.test/cli" || !repo.Local || repo.Revision != head {
		t.Errorf("repo = %s local=%v revision=%s; want example.test/cli local=true revision=%s", repo.RepoID, repo.Local, repo.Revision, head)
	}
	if len(repo.WIP) != 1 || repo.WIP[0].Entry != closerID || repo.WIP[0].Participant != "Local Author" {
		t.Errorf("wip = %+v, want one marker on %s", repo.WIP, closerID)
	}

	if len(doc.Embedded) == 0 {
		t.Error("export omitted the embedded base entries")
	}
	byID := map[string]int{}
	for i, e := range repo.Entries {
		byID[e.ID] = i
	}
	for _, e := range doc.Embedded {
		if _, ok := byID[e.ID]; ok {
			t.Errorf("embedded entry %s also listed under the local repo", e.ID)
		}
	}

	signal := repo.Entries[byID[appEntryID]]
	if signal.ShortID != "s-tac-cli" || signal.Time != "2026-09-14T02:00:00" || signal.Summary != appSummary {
		t.Errorf("signal identity = %s %s %q", signal.ShortID, signal.Time, signal.Summary)
	}
	if signal.Status != "closed-by" || signal.StatusBy != closerID || len(signal.ClosedBy) != 1 || signal.ClosedBy[0] != closerID {
		t.Errorf("signal status = %s by %s closed_by %v; want closed-by %s", signal.Status, signal.StatusBy, signal.ClosedBy, closerID)
	}
	if signal.InDegree != 1 || signal.Heat <= 0 {
		t.Errorf("signal in_degree = %d heat = %v; want 1 and positive heat", signal.InDegree, signal.Heat)
	}

	closer := repo.Entries[byID[closerID]]
	if closer.Status != "active" || len(closer.Closes) != 1 || len(closer.Refs) != 1 ||
		closer.Refs[0].ID != appEntryID || closer.Refs[0].Kind != "addresses" || closer.Refs[0].Desc != "answers the gap" {
		t.Errorf("decision = status %s closes %v refs %+v", closer.Status, closer.Closes, closer.Refs)
	}
	if len(closer.Attachments) != 1 {
		t.Fatalf("attachments = %+v, want notes.md", closer.Attachments)
	}
	a := closer.Attachments[0]
	if a.Name != "notes.md" || a.Size != 26000 || !strings.HasPrefix(a.ContentType, "text/markdown") ||
		!a.Truncated || len(a.Content) == 0 || len(a.Content) > 16*1024 {
		t.Errorf("attachment = %s size %d type %s truncated %v content %d bytes", a.Name, a.Size, a.ContentType, a.Truncated, len(a.Content))
	}

	if bad := f.run(t, "export", "--format", "text"); bad.err == nil || bad.stdout != "" {
		t.Errorf("export --format text = %v with output %q, want an error and no result", bad.err, bad.stdout)
	}
}

// newDependencyRepo commits a graph declaring repoID (and deps) in a fresh
// repository and connects it, so the export's cache freshening clones it.
func newDependencyRepo(t *testing.T, f *appFixture, repoID string, deps []string, entries map[string]string) {
	t.Helper()
	dir := t.TempDir()
	cfg := "repo_id: " + repoID + "\n"
	if len(deps) > 0 {
		cfg += "dependencies: [" + strings.Join(deps, ", ") + "]\n"
	}
	files := map[string]string{".sdd/config.yaml": cfg}
	for rel, content := range entries {
		files[".sdd/graph/"+rel] = content
	}
	for rel, content := range files {
		path := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	runGit(t, dir, "init", "-q", "-b", "main")
	runGit(t, dir, "config", "user.email", "test@example.com")
	runGit(t, dir, "config", "user.name", "Test")
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-q", "-m", "graph")

	config, err := os.ReadFile(filepath.Join(f.root, "config/sdd/config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(config), "repos:") {
		config = append(config, "repos:\n"...)
	}
	config = append(config, "  - repo_id: "+repoID+"\n    clone_url: "+dir+"\n"...)
	f.write(t, "config/sdd/config.yaml", string(config))
}

func testEntry(frontmatter string) string {
	return "---\n" + frontmatter + "layer: tactical\nconfidence: medium\nparticipants: [Local Author]\n---\n\nBody.\n"
}

// TestCLIExportDependencySelection walks the declared closure local → dep →
// deep: referenced mode starts from what the local graph cites and follows
// refs and supersedes upstream, crossing into deep only within the hop budget.
// The local graph overrides a base entry the dependency still cites, so the
// original must stay listed among the embedded entries.
func TestCLIExportDependencySelection(t *testing.T) {
	f := newAppFixture(t)
	const (
		cited      = "20260901-100000-d-tac-aaa"
		grounding  = "20260901-090000-s-tac-bbb"
		superseded = "20260901-080000-s-tac-ccc"
		uncited    = "20260901-070000-s-tac-ddd"
		deepEntry  = "20260801-100000-s-tac-xxx"
		baseEntry  = "20260703-094500-d-prc-cap"
		overridden = "20260812-180000-s-prc-typ"
	)
	newDependencyRepo(t, f, "example.test/deep", nil, map[string]string{
		"2026/08/01-100000-s-tac-xxx.md": testEntry("type: signal\nkind: fact\n"),
	})
	newDependencyRepo(t, f, "example.test/dep", []string{"example.test/deep"}, map[string]string{
		"2026/09/01-100000-d-tac-aaa.md": testEntry("type: decision\nkind: directive\nintent: pending\nrefs:\n  - {id: " + grounding + ", kind: grounded-in}\n  - {id: " + baseEntry + ", kind: related}\n  - {id: " + overridden + ", kind: related}\nsupersedes: [" + superseded + "]\n"),
		"2026/09/01-090000-s-tac-bbb.md": testEntry("type: signal\nkind: gap\nrefs:\n  - {id: 'example.test/deep:" + deepEntry + "', kind: grounded-in}\n"),
		"2026/09/01-080000-s-tac-ccc.md": testEntry("type: signal\nkind: gap\n"),
		"2026/09/01-070000-s-tac-ddd.md": testEntry("type: signal\nkind: gap\n"),
	})
	f.write(t, ".sdd/config.yaml", "repo_id: example.test/cli\ngraph_dir: decisions\ndefault_branch: main\ndependencies: [example.test/dep]\n")
	f.write(t, "decisions/2026/08/12-180000-s-prc-typ.md", "---\ntype: signal\nkind: fact\nlayer: process\nconfidence: high\nparticipants: [Local Author]\n---\n\nLocal override.\n")
	f.write(t, "decisions/2026/09/14-030000-d-tac-loc.md", testEntry("type: decision\nkind: directive\nintent: pending\nrefs:\n  - {id: 'example.test/dep:"+cited+"', kind: grounded-in}\n"))

	export := func(args ...string) exportDoc {
		t.Helper()
		result := f.run(t, append([]string{"export"}, args...)...)
		if result.err != nil {
			t.Fatalf("export %v: %v\n%s", args, result.err, result.stderr)
		}
		var doc exportDoc
		if err := json.Unmarshal([]byte(result.stdout), &doc); err != nil {
			t.Fatalf("export %v emitted invalid JSON: %v", args, err)
		}
		return doc
	}
	type repoWant struct {
		repoID  string
		entries []string
	}
	for _, tt := range []struct {
		name  string
		args  []string
		mode  string
		hops  int
		repos []repoWant
	}{
		{name: "none", args: []string{"--dependencies", "none"}},
		{name: "referenced cited only", args: []string{"--hops", "0"}, mode: "referenced", hops: 0, repos: []repoWant{
			{"example.test/dep", []string{cited}}, {"example.test/deep", nil},
		}},
		{name: "referenced default one hop", mode: "referenced", hops: 1, repos: []repoWant{
			{"example.test/dep", []string{superseded, grounding, cited}}, {"example.test/deep", nil},
		}},
		{name: "referenced crosses into deep", args: []string{"--hops", "2"}, mode: "referenced", hops: 2, repos: []repoWant{
			{"example.test/dep", []string{superseded, grounding, cited}}, {"example.test/deep", []string{deepEntry}},
		}},
		{name: "all", args: []string{"--dependencies", "all"}, mode: "all", repos: []repoWant{
			{"example.test/dep", []string{uncited, superseded, grounding, cited}}, {"example.test/deep", []string{deepEntry}},
		}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			doc := export(tt.args...)
			if len(doc.Repos) != 1+len(tt.repos) || doc.Repos[0].RepoID != "example.test/cli" || doc.Repos[0].Selection != nil {
				t.Fatalf("repos = %d, first %s selection %v; want local plus %d", len(doc.Repos), doc.Repos[0].RepoID, doc.Repos[0].Selection, len(tt.repos))
			}
			for i, want := range tt.repos {
				repo := doc.Repos[i+1]
				var got []string
				for _, e := range repo.Entries {
					got = append(got, e.ID)
					if e.FullID != want.repoID+":"+e.ID {
						t.Errorf("%s entry full_id = %q", want.repoID, e.FullID)
					}
				}
				if repo.RepoID != want.repoID || strings.Join(got, ",") != strings.Join(want.entries, ",") {
					t.Errorf("repo %d = %s %v, want %s %v", i+1, repo.RepoID, got, want.repoID, want.entries)
				}
				sel := repo.Selection
				wantTotal := map[string]int{"example.test/dep": 4, "example.test/deep": 1}[want.repoID]
				if sel == nil || sel.Mode != tt.mode || sel.TotalEntries != wantTotal || (sel.Hops == nil) != (tt.mode != "referenced") ||
					(sel.Hops != nil && *sel.Hops != tt.hops) {
					t.Errorf("%s selection = %+v, want mode %s hops %d total %d", want.repoID, sel, tt.mode, tt.hops, wantTotal)
				}
			}
			found := false
			var original *exportEntry
			for i, e := range doc.Embedded {
				found = found || e.ID == baseEntry
				if e.ID == overridden {
					original = &doc.Embedded[i]
				}
			}
			if !found {
				t.Errorf("base entry %s cited from a dependency is missing from the embedded list", baseEntry)
			}
			if tt.mode != "" && (original == nil || original.Body == "" || original.Body == "Local override.") {
				t.Errorf("base entry %s overridden locally but cited from a dependency: embedded = %+v, want the original", overridden, original)
			}
		})
	}

	out := filepath.Join(f.root, "export.json")
	if err := os.WriteFile(out, []byte("stale"), 0o644); err != nil {
		t.Fatal(err)
	}
	if result := f.run(t, "export", "--out", out); result.err != nil || result.stdout != "" {
		t.Fatalf("export --out = %v with stdout %q", result.err, result.stdout)
	}
	if info, err := os.Stat(out); err != nil || info.Mode().Perm()&0o077 != 0 {
		t.Errorf("--out file mode = %v (%v), want owner-only", info.Mode().Perm(), err)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	var doc exportDoc
	if err := json.Unmarshal(data, &doc); err != nil || len(doc.Repos) != 3 {
		t.Errorf("--out file holds %d repos (%v), want 3", len(doc.Repos), err)
	}
	if bad := f.run(t, "export", "--dependencies", "all", "--hops", "2"); bad.err == nil {
		t.Error("--hops with --dependencies all was accepted")
	}
}
