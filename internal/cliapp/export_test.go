package cliapp_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"strings"
	"testing"
)

type exportDoc struct {
	Experimental bool `json:"experimental"`
	Repos        []struct {
		RepoID   string `json:"repo_id"`
		Local    bool   `json:"local"`
		Revision string `json:"revision"`
		Entries  []struct {
			ID       string   `json:"id"`
			ShortID  string   `json:"short_id"`
			Time     string   `json:"time"`
			LandedAt string   `json:"landed_at"`
			Status   string   `json:"status"`
			StatusBy string   `json:"status_by"`
			ClosedBy []string `json:"closed_by"`
			Closes   []string `json:"closes"`
			Summary  string   `json:"summary"`
			Heat     float64  `json:"heat"`
			InDegree int      `json:"in_degree"`
			Embedded bool     `json:"embedded"`
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
		} `json:"entries"`
		WIP []struct {
			Entry       string `json:"entry"`
			Participant string `json:"participant"`
		} `json:"wip"`
	} `json:"repos"`
}

// TestCLIExportJSON pins the export's derived attributes against a repo whose
// closing decision arrived on main through a merge commit: its landed_at is the
// merge's committer time, not its capture or branch-commit time.
func TestCLIExportJSON(t *testing.T) {
	f := newAppFixture(t)
	const (
		closerID  = "20260914-030000-d-tac-mrg"
		pendingID = "20260914-040000-s-tac-new"
		mergedAt  = "2026-09-15T10:00:00+02:00"
	)
	git := func(date string, args ...string) string {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-c", "commit.gpgsign=false"}, args...)...)
		cmd.Dir = f.root
		cmd.Env = os.Environ()
		if date != "" {
			cmd.Env = append(cmd.Env, "GIT_AUTHOR_DATE="+date, "GIT_COMMITTER_DATE="+date)
		}
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	git("", "init", "-q", "-b", "main")
	git("", "config", "user.email", "test@example.com")
	git("", "config", "user.name", "Test")
	git("2026-09-14T02:00:10+02:00", "add", "decisions")
	git("2026-09-14T02:00:10+02:00", "commit", "-q", "-m", "capture signal")
	git("", "checkout", "-q", "-b", "feature")
	f.write(t, "decisions/2026/09/14-030000-d-tac-mrg.md", "---\ntype: decision\nkind: directive\nintent: pending\nlayer: tactical\nconfidence: high\nparticipants: [Local Author]\nrefs:\n  - id: "+appEntryID+"\n    kind: addresses\n    desc: answers the gap\ncloses: ["+appEntryID+"]\n---\n\nThe branch decision.\n")
	f.write(t, "decisions/2026/09/14-030000-d-tac-mrg/notes.md", strings.Repeat("A note line.\n", 2000))
	git("2026-09-14T03:00:30+02:00", "add", "decisions")
	git("2026-09-14T03:00:30+02:00", "commit", "-q", "-m", "capture decision")
	git("", "checkout", "-q", "main")
	git(mergedAt, "merge", "-q", "--no-ff", "-m", "merge feature", "feature")
	f.write(t, "decisions/2026/09/14-040000-s-tac-new.md", "---\ntype: signal\nkind: gap\nlayer: tactical\nconfidence: low\nparticipants: [Local Author]\n---\n\nNot committed yet.\n")
	f.write(t, "decisions/wip/20260914-050000-local-author.md", "---\nentry: "+closerID+"\nparticipant: Local Author\n---\n\nFollowing up.\n")
	head := git("", "rev-parse", "HEAD")

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
	repo := doc.Repos[0]
	if repo.RepoID != "example.test/cli" || !repo.Local || repo.Revision != head {
		t.Errorf("repo = %s local=%v revision=%s; want example.test/cli local=true revision=%s", repo.RepoID, repo.Local, repo.Revision, head)
	}
	if len(repo.WIP) != 1 || repo.WIP[0].Entry != closerID || repo.WIP[0].Participant != "Local Author" {
		t.Errorf("wip = %+v, want one marker on %s", repo.WIP, closerID)
	}

	byID := map[string]int{}
	embedded := 0
	for i, e := range repo.Entries {
		byID[e.ID] = i
		if e.Embedded {
			embedded++
		}
	}
	if embedded == 0 {
		t.Error("export omitted the embedded base entries")
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
	if signal.LandedAt != "2026-09-14T02:00:10+02:00" {
		t.Errorf("signal landed_at = %q, want its own commit time", signal.LandedAt)
	}

	closer := repo.Entries[byID[closerID]]
	if closer.LandedAt != mergedAt {
		t.Errorf("merged decision landed_at = %q, want merge commit time %s", closer.LandedAt, mergedAt)
	}
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

	if pending := repo.Entries[byID[pendingID]]; pending.LandedAt != "" {
		t.Errorf("uncommitted entry landed_at = %q, want none", pending.LandedAt)
	}

	if bad := f.run(t, "export", "--format", "text"); bad.err == nil || bad.stdout != "" {
		t.Errorf("export --format text = %v with output %q, want an error and no result", bad.err, bad.stdout)
	}
}
