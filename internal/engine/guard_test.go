package engine

import (
	"fmt"
	"strings"
	"testing"
)

func TestParseGuard(t *testing.T) {
	preds := map[string]bool{
		"hasBody": true, "hasRefs": false, "confirmed": true, "blocked": false,
	}
	eval := func(name string) (bool, error) {
		if name == "boom" {
			return false, fmt.Errorf("predicate exploded")
		}
		return preds[name], nil
	}

	tests := []struct {
		src     string
		want    bool
		wantErr string
	}{
		{src: "hasBody", want: true},
		{src: "hasRefs", want: false},
		{src: "hasBody and hasRefs", want: false},
		{src: "hasBody or hasRefs", want: true},
		{src: "not hasRefs", want: true},
		{src: "not hasBody", want: false},
		{src: "hasBody and not hasRefs", want: true},
		// Precedence: not > and > or.
		{src: "hasRefs and hasBody or confirmed", want: true},
		{src: "hasRefs or hasBody and confirmed", want: true},
		{src: "hasRefs and (hasBody or confirmed)", want: false},
		{src: "not (hasBody and hasRefs)", want: true},
		{src: "hasBody and\n        confirmed", want: true}, // folded YAML scalar
		{src: "", wantErr: "empty guard"},
		{src: "hasBody and", wantErr: "expected predicate name"},
		{src: "and hasBody", wantErr: "expected predicate name"},
		{src: "(hasBody", wantErr: "missing closing parenthesis"},
		{src: "hasBody hasRefs", wantErr: "unexpected token"},
		{src: "count > 3", wantErr: "unexpected character"},
		{src: "x == y", wantErr: "unexpected character"},
		{src: `state.body`, wantErr: "unexpected character"},
		{src: "not", wantErr: "expected predicate name"},
	}
	for _, tt := range tests {
		t.Run(tt.src, func(t *testing.T) {
			g, err := ParseGuard(tt.src)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("ParseGuard(%q) err = %v, want containing %q", tt.src, err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseGuard(%q): %v", tt.src, err)
			}
			got, err := g.Eval(eval)
			if err != nil {
				t.Fatalf("Eval: %v", err)
			}
			if got != tt.want {
				t.Errorf("Eval(%q) = %v, want %v", tt.src, got, tt.want)
			}
		})
	}
}

func TestGuardEvalErrorPropagates(t *testing.T) {
	g, err := ParseGuard("hasBody and boom")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := g.Eval(func(name string) (bool, error) {
		if name == "boom" {
			return false, fmt.Errorf("predicate exploded")
		}
		return true, nil
	}); err == nil || !strings.Contains(err.Error(), "exploded") {
		t.Fatalf("Eval must propagate predicate errors, got %v", err)
	}
}

func TestGuardPredicates(t *testing.T) {
	g, err := ParseGuard("hasBody and (hasRefs or not hasBody) and hasRefs")
	if err != nil {
		t.Fatal(err)
	}
	got := g.Predicates()
	want := []string{"hasBody", "hasRefs"}
	if len(got) != len(want) {
		t.Fatalf("Predicates() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("Predicates() = %v, want %v", got, want)
		}
	}
}
