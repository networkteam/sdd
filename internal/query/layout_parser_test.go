package query_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/networkteam/sdd/internal/model"
	"github.com/networkteam/sdd/internal/query"
)

func TestParseLayout_Valid(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  model.Layout
	}{
		// --- Bare-name forms (slice 1 surface) ---
		{
			name:  "single function",
			input: "as-list",
			want: model.Layout{Sections: []model.Section{
				{Functions: []model.Function{{Name: "as-list"}}},
			}},
		},
		{
			name:  "two functions chained with colon",
			input: "active:as-list",
			want: model.Layout{Sections: []model.Section{
				{Functions: []model.Function{{Name: "active"}, {Name: "as-list"}}},
			}},
		},
		{
			name:  "two sections via comma",
			input: "active:as-list,active:as-list",
			want: model.Layout{Sections: []model.Section{
				{Functions: []model.Function{{Name: "active"}, {Name: "as-list"}}},
				{Functions: []model.Function{{Name: "active"}, {Name: "as-list"}}},
			}},
		},
		{
			name:  "permissive on unknown names",
			input: "futurefn:anotherone:as-list",
			want: model.Layout{Sections: []model.Section{
				{Functions: []model.Function{{Name: "futurefn"}, {Name: "anotherone"}, {Name: "as-list"}}},
			}},
		},

		// --- Slice 2: argful forms ---
		{
			name:  "single integer arg",
			input: "n(10)",
			want: model.Layout{Sections: []model.Section{
				{Functions: []model.Function{
					{Name: "n", Args: []model.FunctionArg{
						{Kind: model.ArgKindNumber, Number: 10},
					}},
				}},
			}},
		},
		{
			name:  "single identifier arg",
			input: "kind(plan)",
			want: model.Layout{Sections: []model.Section{
				{Functions: []model.Function{
					{Name: "kind", Args: []model.FunctionArg{
						{Kind: model.ArgKindIdent, String: "plan"},
					}},
				}},
			}},
		},
		{
			name:  "multi identifier args (disjunction within filter)",
			input: "kind(plan,directive,activity)",
			want: model.Layout{Sections: []model.Section{
				{Functions: []model.Function{
					{Name: "kind", Args: []model.FunctionArg{
						{Kind: model.ArgKindIdent, String: "plan"},
						{Kind: model.ArgKindIdent, String: "directive"},
						{Kind: model.ArgKindIdent, String: "activity"},
					}},
				}},
			}},
		},
		{
			name:  "double-quoted string arg",
			input: `topic("hello")`,
			want: model.Layout{Sections: []model.Section{
				{Functions: []model.Function{
					{Name: "topic", Args: []model.FunctionArg{
						{Kind: model.ArgKindString, String: "hello"},
					}},
				}},
			}},
		},
		{
			name:  "single-quoted string arg",
			input: `topic('hello')`,
			want: model.Layout{Sections: []model.Section{
				{Functions: []model.Function{
					{Name: "topic", Args: []model.FunctionArg{
						{Kind: model.ArgKindString, String: "hello"},
					}},
				}},
			}},
		},
		{
			name:  "string with internal whitespace",
			input: `topic("hello world")`,
			want: model.Layout{Sections: []model.Section{
				{Functions: []model.Function{
					{Name: "topic", Args: []model.FunctionArg{
						{Kind: model.ArgKindString, String: "hello world"},
					}},
				}},
			}},
		},
		{
			name:  "string with slashes (forward-looking topic paths)",
			input: `topic("infrastructure/cli")`,
			want: model.Layout{Sections: []model.Section{
				{Functions: []model.Function{
					{Name: "topic", Args: []model.FunctionArg{
						{Kind: model.ArgKindString, String: "infrastructure/cli"},
					}},
				}},
			}},
		},
		{
			name:  "decimal number",
			input: "stalled(0.5)",
			want: model.Layout{Sections: []model.Section{
				{Functions: []model.Function{
					{Name: "stalled", Args: []model.FunctionArg{
						{Kind: model.ArgKindNumber, Number: 0.5},
					}},
				}},
			}},
		},
		{
			name:  "negative number",
			input: "score(-1)",
			want: model.Layout{Sections: []model.Section{
				{Functions: []model.Function{
					{Name: "score", Args: []model.FunctionArg{
						{Kind: model.ArgKindNumber, Number: -1},
					}},
				}},
			}},
		},
		{
			name:  "empty arg list",
			input: "default()",
			want: model.Layout{Sections: []model.Section{
				{Functions: []model.Function{
					{Name: "default", Args: nil},
				}},
			}},
		},
		{
			name:  "nested function call (forward-looking ranking)",
			input: "rank(heat(exp-14d))",
			want: model.Layout{Sections: []model.Section{
				{Functions: []model.Function{
					{Name: "rank", Args: []model.FunctionArg{
						{Kind: model.ArgKindFunc, Func: &model.Function{
							Name: "heat",
							Args: []model.FunctionArg{
								{Kind: model.ArgKindIdent, String: "exp-14d"},
							},
						}},
					}},
				}},
			}},
		},
		{
			name:  "two-level nested call",
			input: "outer(middle(inner(deep)))",
			want: model.Layout{Sections: []model.Section{
				{Functions: []model.Function{
					{Name: "outer", Args: []model.FunctionArg{
						{Kind: model.ArgKindFunc, Func: &model.Function{
							Name: "middle",
							Args: []model.FunctionArg{
								{Kind: model.ArgKindFunc, Func: &model.Function{
									Name: "inner",
									Args: []model.FunctionArg{
										{Kind: model.ArgKindIdent, String: "deep"},
									},
								}},
							},
						}},
					}},
				}},
			}},
		},
		{
			name:  "comma at depth 0 separates sections, not args",
			input: "n(10),n(20)",
			want: model.Layout{Sections: []model.Section{
				{Functions: []model.Function{
					{Name: "n", Args: []model.FunctionArg{{Kind: model.ArgKindNumber, Number: 10}}},
				}},
				{Functions: []model.Function{
					{Name: "n", Args: []model.FunctionArg{{Kind: model.ArgKindNumber, Number: 20}}},
				}},
			}},
		},
		{
			name:  "argful in chained section",
			input: "active:kind(plan):n(10):as-list",
			want: model.Layout{Sections: []model.Section{
				{Functions: []model.Function{
					{Name: "active"},
					{Name: "kind", Args: []model.FunctionArg{{Kind: model.ArgKindIdent, String: "plan"}}},
					{Name: "n", Args: []model.FunctionArg{{Kind: model.ArgKindNumber, Number: 10}}},
					{Name: "as-list"},
				}},
			}},
		},
		{
			name:  "mixed args (number then identifier)",
			input: "fn(10,plan)",
			want: model.Layout{Sections: []model.Section{
				{Functions: []model.Function{
					{Name: "fn", Args: []model.FunctionArg{
						{Kind: model.ArgKindNumber, Number: 10},
						{Kind: model.ArgKindIdent, String: "plan"},
					}},
				}},
			}},
		},
		{
			name:  "identifier with hyphens and digits (decay name shape)",
			input: "rank(exp-14d)",
			want: model.Layout{Sections: []model.Section{
				{Functions: []model.Function{
					{Name: "rank", Args: []model.FunctionArg{
						{Kind: model.ArgKindIdent, String: "exp-14d"},
					}},
				}},
			}},
		},
		{
			name:  "complex full composition (forward-looking)",
			input: "active:kind(plan,directive):n(20):rank(heat(exp-14d)):as-list",
			want: model.Layout{Sections: []model.Section{
				{Functions: []model.Function{
					{Name: "active"},
					{Name: "kind", Args: []model.FunctionArg{
						{Kind: model.ArgKindIdent, String: "plan"},
						{Kind: model.ArgKindIdent, String: "directive"},
					}},
					{Name: "n", Args: []model.FunctionArg{{Kind: model.ArgKindNumber, Number: 20}}},
					{Name: "rank", Args: []model.FunctionArg{
						{Kind: model.ArgKindFunc, Func: &model.Function{
							Name: "heat",
							Args: []model.FunctionArg{{Kind: model.ArgKindIdent, String: "exp-14d"}},
						}},
					}},
					{Name: "as-list"},
				}},
			}},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := query.ParseLayout(tc.input)
			if err != nil {
				t.Fatalf("ParseLayout(%q): unexpected error: %v", tc.input, err)
			}
			// This table asserts function structure; the captured raw source
			// has its own test (TestSectionSourceCapturedAndPreservedThroughMacros).
			for i := range got.Sections {
				got.Sections[i].Source = ""
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("ParseLayout(%q):\n  got:  %#v\n  want: %#v", tc.input, got, tc.want)
			}
		})
	}
}

func TestParseLayout_Invalid(t *testing.T) {
	cases := []struct {
		name        string
		input       string
		errContains []string
	}{
		// --- Slice 1 cases that still hold ---
		{name: "empty string", input: "", errContains: []string{"empty layout"}},
		{name: "leading colon", input: ":as-list", errContains: []string{"position 0"}},
		{name: "trailing colon", input: "active:", errContains: []string{"position 7", "trailing colon"}},
		{name: "double colon", input: "active::as-list", errContains: []string{"position 7"}},
		{name: "leading comma", input: ",as-list", errContains: []string{"position 0"}},
		{name: "trailing comma", input: "as-list,", errContains: []string{"position 8", "trailing comma"}},
		{name: "double comma at depth 0", input: "as-list,,", errContains: []string{"position 8"}},
		{name: "name starts with digit", input: "1abc", errContains: []string{"position 0"}},
		{name: "name starts with hyphen", input: "-abc", errContains: []string{"position 0"}},
		{name: "leading whitespace", input: " active", errContains: []string{"position 0"}},
		{name: "trailing whitespace", input: "active:as-list ", errContains: []string{"position 14"}},
		{name: "whitespace around colon", input: "active : as-list", errContains: []string{"position 6"}},

		// --- Slice 2: argful error cases ---
		{name: "unmatched open paren", input: "n(10", errContains: []string{"position 4", "expected"}},
		{name: "unmatched close paren", input: "n10)", errContains: []string{"position 3"}},
		{name: "empty arg between commas", input: "n(10,,20)", errContains: []string{"position 5"}},
		{name: "leading comma in args", input: "n(,10)", errContains: []string{"position 2"}},
		{name: "trailing comma in args", input: "n(10,)", errContains: []string{"position 5"}},
		{name: "unterminated double-quoted string", input: `topic("hello`, errContains: []string{"unterminated"}},
		{name: "unterminated single-quoted string", input: `topic('hello`, errContains: []string{"unterminated"}},
		{name: "string crosses paren boundary", input: `topic("hello)`, errContains: []string{"unterminated"}},
		{name: "two args without comma", input: "n(10 20)", errContains: []string{"position 4"}},
		{name: "missing close paren before colon", input: "n(10:as-list", errContains: []string{"position 4"}},
		{name: "missing close paren before comma sep", input: "n(10,as-list", errContains: []string{"position", "expected"}},
		{name: "double dot in number", input: "n(1.2.3)", errContains: []string{"position 5"}},
		{name: "lone minus is not a number", input: "n(-)", errContains: []string{"position"}},
		{name: "whitespace inside args (after comma)", input: "kind(plan, directive)", errContains: []string{"position 10"}},
		{name: "whitespace before close paren", input: "n(10 )", errContains: []string{"position 4"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := query.ParseLayout(tc.input)
			if err == nil {
				t.Fatalf("ParseLayout(%q): expected error, got nil", tc.input)
			}
			msg := err.Error()
			for _, want := range tc.errContains {
				if !strings.Contains(msg, want) {
					t.Errorf("ParseLayout(%q): error %q does not contain %q", tc.input, msg, want)
				}
			}
		})
	}
}

// TestParseLayout_QuotingHint covers the actionable hint on the arguments that
// must be quoted: a bare multi-word name, ISO date, or duration breaks the
// parser, and the hint points at the fix. Punctuation that breaks at the same
// site gets no hint, so genuine syntax errors stay uncluttered.
func TestParseLayout_QuotingHint(t *testing.T) {
	const hint = "quote multi-word names"

	present := []string{
		`participant(Jonathan Philipp)`, // space — multi-word name
		`since(7d)`,                     // trailing letter — duration
		`since(2026-07-17)`,             // hyphen — ISO date
	}
	for _, input := range present {
		_, err := query.ParseLayout(input)
		if err == nil {
			t.Fatalf("ParseLayout(%q): expected error, got nil", input)
		}
		if !strings.Contains(err.Error(), hint) {
			t.Errorf("ParseLayout(%q): error %q missing quoting hint", input, err.Error())
		}
	}

	absent := []string{
		`n(10;)`, // punctuation at the expected-')' site — not a bare value
		`n(10:)`, // colon likewise
	}
	for _, input := range absent {
		_, err := query.ParseLayout(input)
		if err == nil {
			t.Fatalf("ParseLayout(%q): expected error, got nil", input)
		}
		if strings.Contains(err.Error(), hint) {
			t.Errorf("ParseLayout(%q): error %q should not carry the quoting hint", input, err.Error())
		}
	}
}
