package bundledskills_test

import (
	"bytes"
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
	for _, want := range []string{"sdd", "sdd-bootstrap", "sdd-explore", "sdd-groom"} {
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

// TestLoadResolvesRefKindsInclude verifies the real framework-concepts.md
// include is resolved at Load: the marker is gone and the canonical vocabulary
// fragment is inlined. This is the single-source mechanism (d-tac-kxt) end to end.
func TestLoadResolvesRefKindsInclude(t *testing.T) {
	b, err := bundledskills.Load(model.AgentClaude)
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, e := range b.Entries {
		if e.Skill == "sdd" && e.RelPath == "references/framework-concepts.md" {
			found = true
			if bytes.Contains(e.Content, []byte("sdd:include")) {
				t.Errorf("include marker not resolved in framework-concepts.md")
			}
			// A distinctive phrase from references/ref-kinds.md must be inlined.
			if !bytes.Contains(e.Content, []byte("A ref is a contextual pointer")) {
				t.Errorf("ref-kinds vocabulary not inlined into framework-concepts.md")
			}
		}
	}
	if !found {
		t.Fatal("framework-concepts.md not found in bundle")
	}
}

func TestReadReference_RefKinds(t *testing.T) {
	body, err := bundledskills.ReadReference("sdd", "references/ref-kinds.md")
	if err != nil {
		t.Fatalf("ReadReference: %v", err)
	}
	if !bytes.Contains(body, []byte("grounded-in")) || !bytes.Contains(body, []byte("required-by")) {
		t.Errorf("ref-kinds reference missing expected kinds:\n%s", body)
	}
}
