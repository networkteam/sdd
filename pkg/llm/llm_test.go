package llm_test

import (
	"testing"

	"github.com/networkteam/sdd/pkg/llm"
)

func TestIdentityString(t *testing.T) {
	if got := (llm.Identity{Model: "gpt-5.6"}).String(); got != "gpt-5.6" {
		t.Errorf("bare model = %q", got)
	}
	if got := (llm.Identity{Model: "gpt-5.6", Variant: "reasoning_effort=high"}).String(); got != "gpt-5.6 (reasoning_effort=high)" {
		t.Errorf("variant model = %q", got)
	}
}

func TestRequestCombined(t *testing.T) {
	tests := []struct {
		name string
		req  llm.Request
		want string
	}{
		{name: "both present", req: llm.Request{SystemPrompt: "sys", UserPrompt: "user"}, want: "sys\n\nuser"},
		{name: "system only", req: llm.Request{SystemPrompt: "sys"}, want: "sys"},
		{name: "user only", req: llm.Request{UserPrompt: "user"}, want: "user"},
		{name: "both empty", req: llm.Request{}, want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.req.Combined(); got != tt.want {
				t.Errorf("Combined() = %q, want %q", got, tt.want)
			}
		})
	}
}
