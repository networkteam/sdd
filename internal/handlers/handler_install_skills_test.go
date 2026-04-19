package handlers_test

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/networkteam/sdd/internal/command"
	"github.com/networkteam/sdd/internal/finders"
	"github.com/networkteam/sdd/internal/handlers"
	"github.com/networkteam/sdd/internal/model"
)

// TestInstallSkills_FreshInstall verifies that every embedded entry ends up
// in the "installed" category when the target dir starts empty.
func TestInstallSkills_FreshInstall(t *testing.T) {
	tmp := t.TempDir()

	h := handlers.New(handlers.Options{
		Reader: finders.New(nil),
	})

	var got command.SkillInstallResult
	err := h.InstallSkills(context.Background(), &command.InstallSkillsCmd{
		Target:        model.AgentClaude,
		Scope:         model.ScopeProject,
		RepoRoot:      tmp,
		BinaryVersion: "v0.2.0",
		OnInstalled: func(r command.SkillInstallResult) {
			got = r
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Installed) == 0 {
		t.Fatal("expected files to be installed")
	}
	if len(got.Refreshed)+len(got.Overwritten)+len(got.SkippedModified)+len(got.Current) != 0 {
		t.Errorf("unexpected non-installed categories: %+v", got)
	}

	// Spot-check one installed file was actually written with stamps.
	for _, p := range got.Installed {
		if !strings.HasPrefix(p, filepath.Join(tmp, ".claude", "skills")+string(os.PathSeparator)) {
			t.Errorf("installed path not under expected dir: %s", p)
		}
		data, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		sf := model.ParseSkillFile(p, data)
		if sf.StoredVersion != "v0.2.0" {
			t.Errorf("%s: StoredVersion = %q, want v0.2.0", p, sf.StoredVersion)
		}
		if sf.StoredHash == "" {
			t.Errorf("%s: StoredHash empty", p)
		}
	}
}

// TestInstallSkills_RepeatIsNoOp verifies that a second install run classifies
// everything as Current and writes nothing.
func TestInstallSkills_RepeatIsNoOp(t *testing.T) {
	tmp := t.TempDir()
	h := handlers.New(handlers.Options{Reader: finders.New(nil)})

	run := func() command.SkillInstallResult {
		var got command.SkillInstallResult
		err := h.InstallSkills(context.Background(), &command.InstallSkillsCmd{
			Target:        model.AgentClaude,
			Scope:         model.ScopeProject,
			RepoRoot:      tmp,
			BinaryVersion: "v0.2.0",
			OnInstalled:   func(r command.SkillInstallResult) { got = r },
		})
		if err != nil {
			t.Fatal(err)
		}
		return got
	}

	_ = run() // fresh install
	second := run()
	if len(second.Installed)+len(second.Refreshed)+len(second.Overwritten)+len(second.SkippedModified) != 0 {
		t.Errorf("second run should be a no-op, got: %+v", second)
	}
	if len(second.Current) == 0 {
		t.Errorf("second run should classify everything as current")
	}
}

// TestInstallSkills_PromptModified verifies the prompt callback is consulted
// when a file has been user-edited, and routes outcomes to Overwritten vs
// SkippedModified based on its return value.
func TestInstallSkills_PromptModified(t *testing.T) {
	tmp := t.TempDir()
	h := handlers.New(handlers.Options{Reader: finders.New(nil)})

	// Fresh install so files exist.
	err := h.InstallSkills(context.Background(), &command.InstallSkillsCmd{
		Target:        model.AgentClaude,
		Scope:         model.ScopeProject,
		RepoRoot:      tmp,
		BinaryVersion: "v0.2.0",
	})
	if err != nil {
		t.Fatal(err)
	}

	// Tamper with two files — one the user will approve, one they'll reject.
	installDir := filepath.Join(tmp, ".claude", "skills")
	paths := []string{
		filepath.Join(installDir, "sdd", "SKILL.md"),
		filepath.Join(installDir, "sdd-catchup", "SKILL.md"),
	}
	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		data = append(data, []byte("\nuser edit\n")...)
		if err := os.WriteFile(p, data, 0o644); err != nil {
			t.Fatal(err)
		}
	}

	approvePath := paths[0]
	rejectPath := paths[1]

	var got command.SkillInstallResult
	err = h.InstallSkills(context.Background(), &command.InstallSkillsCmd{
		Target:        model.AgentClaude,
		Scope:         model.ScopeProject,
		RepoRoot:      tmp,
		BinaryVersion: "v0.2.0",
		PromptOverwrite: func(p string) (bool, error) {
			return p == approvePath, nil
		},
		OnInstalled: func(r command.SkillInstallResult) { got = r },
	})
	if err != nil {
		t.Fatal(err)
	}

	if !slices.Contains(got.Overwritten, approvePath) {
		t.Errorf("expected %s in Overwritten, got: %+v", approvePath, got.Overwritten)
	}
	if !slices.Contains(got.SkippedModified, rejectPath) {
		t.Errorf("expected %s in SkippedModified, got: %+v", rejectPath, got.SkippedModified)
	}
}
