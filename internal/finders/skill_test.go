package finders_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/networkteam/sdd/internal/finders"
	"github.com/networkteam/sdd/internal/model"
	"github.com/networkteam/sdd/internal/query"
)

func TestSkillStatus_MissingWhenNothingInstalled(t *testing.T) {
	tmp := t.TempDir()
	f := finders.New(nil)
	res, err := f.SkillStatus(context.Background(), query.SkillStatusQuery{
		Target:   model.AgentClaude,
		Scope:    model.ScopeProject,
		RepoRoot: tmp,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.InstallDir != filepath.Join(tmp, ".claude", "skills") {
		t.Errorf("install dir: got %s", res.InstallDir)
	}
	if len(res.Entries) == 0 {
		t.Fatal("expected some entries from the embedded bundle")
	}
	for _, e := range res.Entries {
		if e.Status != model.SkillStatusMissing {
			t.Errorf("entry %s/%s: got %s, want missing", e.Skill, e.RelPath, e.Status)
		}
	}
}

func TestSkillStatus_CurrentWhenInstalledFromSameBundle(t *testing.T) {
	tmp := t.TempDir()

	// First pass: collect entries.
	f := finders.New(nil)
	res, err := f.SkillStatus(context.Background(), query.SkillStatusQuery{
		Target:   model.AgentClaude,
		Scope:    model.ScopeProject,
		RepoRoot: tmp,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Render + write each file exactly as SkillHandler.Install will.
	for _, e := range res.Entries {
		hash := model.ComputeSkillHash(e.Embedded.Content)
		out, err := model.RenderSkillFile(e.Embedded, "v0.2.0", hash)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Dir(e.AbsPath), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(e.AbsPath, out, 0o644); err != nil {
			t.Fatal(err)
		}
	}

	// Second pass: every entry must read as Current.
	res2, err := f.SkillStatus(context.Background(), query.SkillStatusQuery{
		Target:   model.AgentClaude,
		Scope:    model.ScopeProject,
		RepoRoot: tmp,
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range res2.Entries {
		if e.Status != model.SkillStatusCurrent {
			t.Errorf("entry %s/%s: got %s, want current", e.Skill, e.RelPath, e.Status)
		}
	}
}

func TestSkillStatus_ModifiedWhenBodyEdited(t *testing.T) {
	tmp := t.TempDir()
	f := finders.New(nil)

	first, err := f.SkillStatus(context.Background(), query.SkillStatusQuery{
		Target:   model.AgentClaude,
		Scope:    model.ScopeProject,
		RepoRoot: tmp,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Entries) == 0 {
		t.Fatal("expected bundle entries")
	}
	// Install every file, then tamper with exactly one.
	var tampered string
	for i, e := range first.Entries {
		hash := model.ComputeSkillHash(e.Embedded.Content)
		out, err := model.RenderSkillFile(e.Embedded, "v0.2.0", hash)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Dir(e.AbsPath), 0o755); err != nil {
			t.Fatal(err)
		}
		if i == 0 {
			out = append(out, []byte("\nlocal edit\n")...)
			tampered = e.AbsPath
		}
		if err := os.WriteFile(e.AbsPath, out, 0o644); err != nil {
			t.Fatal(err)
		}
	}

	second, err := f.SkillStatus(context.Background(), query.SkillStatusQuery{
		Target:   model.AgentClaude,
		Scope:    model.ScopeProject,
		RepoRoot: tmp,
	})
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, e := range second.Entries {
		if e.AbsPath == tampered {
			found = true
			if e.Status != model.SkillStatusModified {
				t.Errorf("tampered file: got %s, want modified", e.Status)
			}
		} else if e.Status != model.SkillStatusCurrent {
			t.Errorf("%s: got %s, want current", e.AbsPath, e.Status)
		}
	}
	if !found {
		t.Error("did not find tampered file in second pass")
	}
}
