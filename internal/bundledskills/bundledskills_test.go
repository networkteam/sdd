package bundledskills_test

import (
	"testing"

	"github.com/networkteam/sdd/internal/bundledskills"
	"github.com/networkteam/sdd/internal/model"
)

func TestLoadClaude(t *testing.T) {
	b, err := bundledskills.Load(model.AgentClaude)
	if err != nil {
		t.Fatal(err)
	}
	if len(b.Entries) == 0 {
		t.Fatal("expected bundle entries")
	}

	skills := map[string]int{}
	for _, e := range b.Entries {
		skills[e.Skill]++
	}
	for _, want := range []string{"sdd", "sdd-catchup", "sdd-explore", "sdd-groom"} {
		if skills[want] == 0 {
			t.Errorf("expected skill %q in bundle", want)
		}
	}

	// Spot-check the main sdd skill carries its SKILL.md and references.
	for _, e := range b.Entries {
		if e.Skill == "sdd" && e.RelPath == "SKILL.md" {
			if len(e.Content) == 0 {
				t.Error("sdd/SKILL.md is empty")
			}
			return
		}
	}
	t.Error("sdd/SKILL.md not found in bundle")
}
