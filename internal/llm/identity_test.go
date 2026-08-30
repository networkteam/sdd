package llm_test

import (
	"testing"

	"github.com/networkteam/sdd/internal/llm"
)

func TestIdentityString(t *testing.T) {
	if got := (llm.Identity{Model: "gpt-5.6"}).String(); got != "gpt-5.6" {
		t.Errorf("bare model = %q", got)
	}
	if got := (llm.Identity{Model: "gpt-5.6", Variant: "reasoning_effort=high"}).String(); got != "gpt-5.6 (reasoning_effort=high)" {
		t.Errorf("variant model = %q", got)
	}
}
