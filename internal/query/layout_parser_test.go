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
			name:  "three functions chained",
			input: "active:active:as-list",
			want: model.Layout{Sections: []model.Section{
				{Functions: []model.Function{{Name: "active"}, {Name: "active"}, {Name: "as-list"}}},
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
			name:  "single function, multiple sections",
			input: "as-list,as-list,as-list",
			want: model.Layout{Sections: []model.Section{
				{Functions: []model.Function{{Name: "as-list"}}},
				{Functions: []model.Function{{Name: "as-list"}}},
				{Functions: []model.Function{{Name: "as-list"}}},
			}},
		},
		{
			name:  "underscores in name",
			input: "as_list",
			want: model.Layout{Sections: []model.Section{
				{Functions: []model.Function{{Name: "as_list"}}},
			}},
		},
		{
			name:  "digits in name body",
			input: "top25:as-list",
			want: model.Layout{Sections: []model.Section{
				{Functions: []model.Function{{Name: "top25"}, {Name: "as-list"}}},
			}},
		},
		{
			name:  "uppercase letters allowed",
			input: "Active:AsList",
			want: model.Layout{Sections: []model.Section{
				{Functions: []model.Function{{Name: "Active"}, {Name: "AsList"}}},
			}},
		},
		// Forward-looking: parser is permissive on names. Unknown names parse
		// fine — the executor rejects them at runtime with the listed-valid-set
		// error. This keeps the parser stable as the vocabulary grows.
		{
			name:  "permissive on unknown names",
			input: "futurefn:anotherone:as-list",
			want: model.Layout{Sections: []model.Section{
				{Functions: []model.Function{{Name: "futurefn"}, {Name: "anotherone"}, {Name: "as-list"}}},
			}},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := query.ParseLayout(tc.input)
			if err != nil {
				t.Fatalf("ParseLayout(%q): unexpected error: %v", tc.input, err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("ParseLayout(%q):\n  got:  %+v\n  want: %+v", tc.input, got, tc.want)
			}
		})
	}
}

func TestParseLayout_Invalid(t *testing.T) {
	cases := []struct {
		name        string
		input       string
		errContains []string // all substrings must appear in err.Error()
	}{
		{
			name:        "empty string",
			input:       "",
			errContains: []string{"empty layout"},
		},
		{
			name:        "leading colon",
			input:       ":as-list",
			errContains: []string{"position 0"},
		},
		{
			name:        "trailing colon",
			input:       "active:",
			errContains: []string{"position 7", "trailing colon"},
		},
		{
			name:        "double colon",
			input:       "active::as-list",
			errContains: []string{"position 7"},
		},
		{
			name:        "leading comma",
			input:       ",as-list",
			errContains: []string{"position 0"},
		},
		{
			name:        "trailing comma",
			input:       "as-list,",
			errContains: []string{"position 8", "trailing comma"},
		},
		{
			name:        "double comma",
			input:       "as-list,,",
			errContains: []string{"position 8"},
		},
		{
			name:        "name starts with digit",
			input:       "1abc",
			errContains: []string{"position 0"},
		},
		{
			name:        "name starts with hyphen",
			input:       "-abc",
			errContains: []string{"position 0"},
		},
		{
			name:        "name starts with underscore",
			input:       "_abc",
			errContains: []string{"position 0"},
		},
		{
			name:        "parens not yet supported",
			input:       "top(20)",
			errContains: []string{"position 3"},
		},
		{
			name:        "exclamation invalid",
			input:       "as-list!",
			errContains: []string{"position 7"},
		},
		{
			name:        "leading whitespace",
			input:       " active",
			errContains: []string{"position 0"},
		},
		{
			name:        "trailing whitespace",
			input:       "active:as-list ",
			errContains: []string{"position 14"},
		},
		{
			name:        "whitespace around colon",
			input:       "active : as-list",
			errContains: []string{"position 6"},
		},
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
