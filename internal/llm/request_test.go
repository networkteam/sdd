package llm

import "testing"

func TestRequestCombined(t *testing.T) {
	tests := []struct {
		name string
		req  Request
		want string
	}{
		{
			name: "both present",
			req:  Request{SystemPrompt: "sys", UserPrompt: "user"},
			want: "sys\n\nuser",
		},
		{
			name: "system only",
			req:  Request{SystemPrompt: "sys"},
			want: "sys",
		},
		{
			name: "user only",
			req:  Request{UserPrompt: "user"},
			want: "user",
		},
		{
			name: "both empty",
			req:  Request{},
			want: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.req.Combined(); got != tt.want {
				t.Errorf("Combined() = %q, want %q", got, tt.want)
			}
		})
	}
}
