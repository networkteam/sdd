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

// writeStampedOrphan plants a file that a previous sdd init could have written
// — carrying valid install stamps — at a path the current bundle does not
// contain. Stamp keys are stripped before hashing, so the digest computed over
// the placeholder text is the digest of the finished file.
func writeStampedOrphan(t *testing.T, path, body string) {
	t.Helper()
	writeStampedOrphanAt(t, path, "v0.1.0", body)
}

func writeStampedOrphanAt(t *testing.T, path, version, body string) {
	t.Helper()
	text := "---\nname: " + filepath.Base(filepath.Dir(path)) + "\nsdd-version: " + version + "\nsdd-content-hash: placeholder\n---\n\n" + body
	stamped := strings.Replace(text, "placeholder", model.ComputeSkillHash([]byte(text)), 1)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir for %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(stamped), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// TestInit_PrunesUnmodifiedOrphan covers the upgrade path of s-tac-zaz: a file
// a previous install wrote, whose bundle source is gone, is removed on the next
// init even though no agent was dropped — and the directory it emptied goes too.
func TestInit_PrunesUnmodifiedOrphan(t *testing.T) {
	tmp := t.TempDir()
	h := initExistingWithAgents(t, tmp, model.AgentClaude)

	orphan := filepath.Join(tmp, ".claude/skills/sdd-retired/SKILL.md")
	writeStampedOrphan(t, orphan, "A skill the bundle no longer ships.\n")
	orphanRef := filepath.Join(tmp, ".claude/skills/sdd/references/gone.md")
	writeStampedOrphan(t, orphanRef, "A reference the bundle no longer ships.\n")

	var pruned []command.AgentPruneResult
	if err := h.Init(context.Background(), &command.InitCmd{
		RepoRoot:             tmp,
		BinaryVersion:        "v0.2.0",
		Scope:                model.ScopeProject,
		OnSkillOrphansPruned: func(r command.AgentPruneResult) { pruned = append(pruned, r) },
	}); err != nil {
		t.Fatal(err)
	}

	for _, p := range []string{orphan, orphanRef} {
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Errorf("unmodified orphan %s should be removed, stat err = %v", p, err)
		}
	}
	// The skill dir emptied by the removal goes with it; the one still holding
	// bundle files stays.
	if _, err := os.Stat(filepath.Join(tmp, ".claude/skills/sdd-retired")); !os.IsNotExist(err) {
		t.Errorf("emptied skill dir should be pruned, stat err = %v", err)
	}
	if _, err := os.Stat(filepath.Join(tmp, ".claude/skills/sdd/SKILL.md")); err != nil {
		t.Errorf("bundle files must survive the orphan sweep: %v", err)
	}
	if len(pruned) != 1 {
		t.Fatalf("expected one orphan-prune callback, got %+v", pruned)
	}
	if !slices.Contains(pruned[0].Removed, orphan) || !slices.Contains(pruned[0].Removed, orphanRef) {
		t.Errorf("Removed should name both orphans, got %v", pruned[0].Removed)
	}
}

// TestInit_PreservesModifiedOrphan holds the safety half of the rule: an orphan
// the user has edited is never silently discarded — it stays, and the run names
// it. Under --force it goes like any other sdd-owned file.
func TestInit_PreservesModifiedOrphan(t *testing.T) {
	tmp := t.TempDir()
	h := initExistingWithAgents(t, tmp, model.AgentClaude)

	orphan := filepath.Join(tmp, ".claude/skills/sdd-retired/SKILL.md")
	writeStampedOrphan(t, orphan, "A skill the bundle no longer ships.\n")
	appendToFile(t, orphan, "\n<!-- local edit, must survive -->\n")

	var pruned []command.AgentPruneResult
	if err := h.Init(context.Background(), &command.InitCmd{
		RepoRoot:             tmp,
		BinaryVersion:        "v0.2.0",
		Scope:                model.ScopeProject,
		OnSkillOrphansPruned: func(r command.AgentPruneResult) { pruned = append(pruned, r) },
	}); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(orphan); err != nil {
		t.Errorf("modified orphan must be preserved without --force: %v", err)
	}
	if len(pruned) != 1 || !slices.Contains(pruned[0].KeptModified, orphan) {
		t.Fatalf("KeptModified should name %s, got %+v", orphan, pruned)
	}

	pruned = nil
	if err := h.Init(context.Background(), &command.InitCmd{
		RepoRoot:             tmp,
		BinaryVersion:        "v0.2.0",
		Scope:                model.ScopeProject,
		Force:                true,
		OnSkillOrphansPruned: func(r command.AgentPruneResult) { pruned = append(pruned, r) },
	}); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(orphan); !os.IsNotExist(err) {
		t.Errorf("--force should remove the modified orphan, stat err = %v", err)
	}
	if len(pruned) != 1 || !slices.Contains(pruned[0].Removed, orphan) {
		t.Errorf("under --force the modified orphan should be Removed, got %+v", pruned)
	}
}

// TestInit_LeavesAheadStampedOrphan covers the downgrade case: an older binary
// finds files a newer sdd installed missing from its own bundle, and must not
// delete the future on the strength of that.
func TestInit_LeavesAheadStampedOrphan(t *testing.T) {
	tmp := t.TempDir()
	h := handlers.New(handlers.Options{Reader: finders.New(finders.Options{})})
	seed := &command.InitCmd{
		RepoRoot:      tmp,
		BinaryVersion: "v0.17.0",
		Targets:       []model.AgentTarget{model.AgentClaude},
		Scope:         model.ScopeProject,
	}
	if err := h.Init(context.Background(), seed); err != nil {
		t.Fatalf("seed init: %v", err)
	}

	ahead := filepath.Join(tmp, ".claude/skills/sdd-future/SKILL.md")
	writeStampedOrphanAt(t, ahead, "v0.19.0", "Shipped by a later sdd than the one running.\n")
	behind := filepath.Join(tmp, ".claude/skills/sdd-retired/SKILL.md")
	writeStampedOrphanAt(t, behind, "v0.16.0", "Shipped by an earlier sdd.\n")

	if err := h.Init(context.Background(), &command.InitCmd{
		RepoRoot:      tmp,
		BinaryVersion: "v0.17.0",
		Scope:         model.ScopeProject,
	}); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(ahead); err != nil {
		t.Errorf("a file stamped by a later sdd must survive: %v", err)
	}
	if _, err := os.Stat(behind); !os.IsNotExist(err) {
		t.Errorf("a file stamped by an earlier sdd is an ordinary orphan, stat err = %v", err)
	}
}

// TestInit_LeavesForeignSkillUntouched guards the ownership rule: the install
// directory also holds skills sdd never wrote. Carrying no stamp, they are not
// orphans — not removed, and not reported as anything the user must resolve.
func TestInit_LeavesForeignSkillUntouched(t *testing.T) {
	tmp := t.TempDir()
	h := initExistingWithAgents(t, tmp, model.AgentClaude)

	foreign := filepath.Join(tmp, ".claude/skills/my-own/SKILL.md")
	if err := os.MkdirAll(filepath.Dir(foreign), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(foreign, []byte("---\nname: my-own\n---\n\nMine, not sdd's.\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var pruned []command.AgentPruneResult
	if err := h.Init(context.Background(), &command.InitCmd{
		RepoRoot:             tmp,
		BinaryVersion:        "v0.2.0",
		Scope:                model.ScopeProject,
		Force:                true,
		OnSkillOrphansPruned: func(r command.AgentPruneResult) { pruned = append(pruned, r) },
	}); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(foreign); err != nil {
		t.Errorf("a skill sdd never wrote must survive even --force: %v", err)
	}
	if len(pruned) != 0 {
		t.Errorf("an unstamped file is not an orphan and must not be reported, got %+v", pruned)
	}
}
