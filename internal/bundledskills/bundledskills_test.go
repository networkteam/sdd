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

// TestLoadRendersClaudeProfile checks that rendering the Claude profile produces
// fully-resolved output: no template actions survive, the inject helper emits
// Claude's dynamic-injection token, the agent conditional picks the Claude branch
// (not the else branch), and the escaped attachments placeholder renders back to
// its literal form. The byte-level parity guarantee (rendered == prior bundle) is
// enforced by `sdd init` leaving .claude/skills/ unchanged on a clean tree.
func TestLoadRendersClaudeProfile(t *testing.T) {
	b, err := bundledskills.Load(model.AgentClaude)
	if err != nil {
		t.Fatal(err)
	}

	byPath := map[string][]byte{}
	badMarkers := []string{"{{ inject", "{{inject", "{{ if", "{{if", "{{ template", "{{template", "{{ end", "{{ else"}
	for _, e := range b.Entries {
		byPath[e.Skill+"/"+e.RelPath] = e.Content
		for _, m := range badMarkers {
			if bytes.Contains(e.Content, []byte(m)) {
				t.Errorf("%s/%s: unrendered template action %q survived render", e.Skill, e.RelPath, m)
			}
		}
	}

	sddSkill := byPath["sdd/SKILL.md"]
	if !bytes.Contains(sddSkill, []byte("!`sdd info`")) {
		t.Error("sdd/SKILL.md: inject helper did not render the Claude !`sdd info` token")
	}
	if !bytes.Contains(sddSkill, []byte("via the Skill tool")) {
		t.Error("sdd/SKILL.md: Claude branch of the catch-up conditional missing")
	}
	if bytes.Contains(sddSkill, []byte("Then run the `sdd-catchup` skill")) {
		t.Error("sdd/SKILL.md: non-Claude else branch leaked into the Claude render")
	}

	if cliRef := byPath["sdd/references/cli-reference.md"]; !bytes.Contains(cliRef, []byte("{{attachments}}/filename")) {
		t.Error("cli-reference.md: escaped {{attachments}} did not render to the literal token")
	}
}
