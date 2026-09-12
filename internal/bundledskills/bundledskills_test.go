package bundledskills_test

import (
	"bytes"
	"testing"

	"github.com/networkteam/sdd/internal/bundledskills"
	"github.com/networkteam/sdd/internal/model"
)

// TestLoadShipsOnlyTheEngineSkill pins the v0.18.0 bundle shape (d-tac-ip1):
// one skill, `sdd`, opening the engine flow, with the locale vocabulary as its
// only reference until that moves into the graph (d-cpt-xob).
func TestLoadShipsOnlyTheEngineSkill(t *testing.T) {
	for _, target := range []model.AgentTarget{model.AgentClaude, model.AgentCodex} {
		b, err := bundledskills.Load(target)
		if err != nil {
			t.Fatalf("%s: %v", target, err)
		}
		var paths []string
		for _, e := range b.Entries {
			if e.Skill != "sdd" {
				t.Errorf("%s: legacy skill %q still in the bundle", target, e.Skill)
			}
			paths = append(paths, e.RelPath)
		}
		want := []string{"SKILL.md", "references/vocabulary-de.md"}
		if len(paths) != len(want) {
			t.Fatalf("%s: bundle paths = %v, want %v", target, paths, want)
		}
		for i := range want {
			if paths[i] != want[i] {
				t.Errorf("%s: bundle paths = %v, want %v", target, paths, want)
			}
		}
	}
}

func TestReadReference_VocabularyDE(t *testing.T) {
	body, err := bundledskills.ReadReference("sdd", "references/vocabulary-de.md")
	if err != nil {
		t.Fatalf("ReadReference: %v", err)
	}
	if bytes.HasPrefix(body, []byte("---")) {
		t.Errorf("frontmatter not stripped:\n%s", body)
	}
	if !bytes.Contains(body, []byte("Vokabular")) || !bytes.Contains(body, []byte("Entscheidung")) {
		t.Errorf("German vocabulary missing expected terms:\n%s", body)
	}
}

func TestReadReference_Missing(t *testing.T) {
	if _, err := bundledskills.ReadReference("sdd", "references/ref-kinds.md"); err == nil {
		t.Fatal("expected an error for a reference the bundle no longer ships")
	}
}

// TestLoadRendersProfiles checks that both render profiles resolve every
// template action, that the skill opens the engine flow, and that each
// profile's frontmatter matches its host: Claude carries no Agent-Skills
// compatibility field, Codex does.
func TestLoadRendersProfiles(t *testing.T) {
	badMarkers := []string{"{{ inject", "{{inject", "{{ if", "{{if", "{{ template", "{{template", "{{ end", "{{ else"}
	for _, target := range []model.AgentTarget{model.AgentClaude, model.AgentCodex} {
		b, err := bundledskills.Load(target)
		if err != nil {
			t.Fatalf("%s: %v", target, err)
		}
		var skill []byte
		for _, e := range b.Entries {
			for _, m := range badMarkers {
				if bytes.Contains(e.Content, []byte(m)) {
					t.Errorf("%s: %s/%s: unrendered template action %q survived render", target, e.Skill, e.RelPath, m)
				}
			}
			if e.RelPath == "SKILL.md" {
				skill = e.Content
			}
		}
		if !bytes.HasPrefix(skill, []byte("---\nname: sdd\n")) {
			t.Errorf("%s: SKILL.md must be named sdd:\n%.80s", target, skill)
		}
		if !bytes.Contains(skill, []byte("`start_session`")) {
			t.Errorf("%s: SKILL.md does not open the engine flow", target)
		}
		for _, stale := range []string{"sdd-engine", "sdd-catchup", "sdd-bootstrap", "deprecated", "Deprecated"} {
			if bytes.Contains(skill, []byte(stale)) {
				t.Errorf("%s: SKILL.md still names %q", target, stale)
			}
		}
		hasCompat := bytes.Contains(skill, []byte("compatibility: Designed for OpenAI Codex"))
		if target == model.AgentCodex && !hasCompat {
			t.Errorf("codex: SKILL.md missing the compatibility field")
		}
		if target == model.AgentClaude && hasCompat {
			t.Errorf("claude: SKILL.md must not carry a compatibility field")
		}
	}
}
