package engine

import (
	"strings"
	"testing"
)

func TestParseVarType(t *testing.T) {
	tests := []struct {
		src     string
		want    string
		wantErr bool
	}{
		{src: "text", want: "text"},
		{src: "bool", want: "bool"},
		{src: "entry-id", want: "entry-id"},
		{src: "list<ref>", want: "list<ref>"},
		{src: "list<entry-id>", want: "list<entry-id>"},
		{src: " list<label> ", want: "list<label>"},
		{src: "list<nope>", wantErr: true},
		{src: "list<ref", wantErr: true},
		{src: "integer", wantErr: true},
		{src: "", wantErr: true},
		{src: "list<list<text>>", wantErr: true}, // no nesting
	}
	for _, tt := range tests {
		got, err := ParseVarType(tt.src)
		if tt.wantErr {
			if err == nil {
				t.Errorf("ParseVarType(%q) = %v, want error", tt.src, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("ParseVarType(%q): %v", tt.src, err)
			continue
		}
		if got.String() != tt.want {
			t.Errorf("ParseVarType(%q) = %q, want %q", tt.src, got, tt.want)
		}
	}
}

func TestValidateValue(t *testing.T) {
	const goodID = "20260702-120000-d-tac-abc"
	tests := []struct {
		name    string
		typ     string
		value   any
		wantErr string
	}{
		{name: "text ok", typ: "text", value: "hello"},
		{name: "text not string", typ: "text", value: 42, wantErr: "expected text"},
		{name: "bool ok", typ: "bool", value: true},
		{name: "bool not bool", typ: "bool", value: "true", wantErr: "expected bool"},
		{name: "entry-id ok", typ: "entry-id", value: goodID},
		{name: "entry-id short form rejected", typ: "entry-id", value: "d-tac-abc", wantErr: "not a full entry ID"},
		{name: "ref ok", typ: "ref", value: map[string]any{"id": goodID, "kind": "addresses"}},
		{name: "ref bad kind", typ: "ref", value: map[string]any{"id": goodID, "kind": "relates"}, wantErr: "closed ref-kind set"},
		{name: "ref legacy kind rejected", typ: "ref", value: map[string]any{"id": goodID, "kind": "grounds"}, wantErr: "closed ref-kind set"},
		{name: "ref missing id", typ: "ref", value: map[string]any{"kind": "related"}, wantErr: "not a full entry ID"},
		{name: "label ok", typ: "label", value: "type-system/kinds"},
		{name: "label bad component", typ: "label", value: "a//b", wantErr: "empty"},
		{name: "participant ok", typ: "participant", value: "Christopher"},
		{name: "participant empty", typ: "participant", value: "  ", wantErr: "non-empty"},
		{name: "entry-kind ok", typ: "entry-kind", value: "directive"},
		{name: "entry-kind signal ok", typ: "entry-kind", value: "gap"},
		{name: "entry-kind unknown", typ: "entry-kind", value: "todo", wantErr: "unknown entry kind"},
		{name: "layer full", typ: "layer", value: "tactical"},
		{name: "layer abbrev normalized", typ: "layer", value: "tac"},
		{name: "layer unknown", typ: "layer", value: "meta", wantErr: "unknown layer"},
		{name: "confidence ok", typ: "confidence", value: "medium"},
		{name: "confidence unknown", typ: "confidence", value: "sure", wantErr: "high|medium|low"},
		{name: "intent ok", typ: "intent", value: "guiding"},
		{name: "intent unknown", typ: "intent", value: "someday", wantErr: "pending|guiding|settled"},
		{name: "attachment handle ok", typ: "attachment-handle", value: "att_1"},
		{name: "fact index ok", typ: "fact-index", value: map[string]any{"title": "View grammar", "topic": "cli/view"}},
		{name: "fact index partial", typ: "fact-index", value: map[string]any{"title": "View grammar"}, wantErr: "exactly title and topic"},
		{name: "fact index extra", typ: "fact-index", value: map[string]any{"title": "View grammar", "topic": "cli/view", "extra": true}, wantErr: "exactly title and topic"},
		{name: "fact index blank title", typ: "fact-index", value: map[string]any{"title": " ", "topic": "cli/view"}, wantErr: "trimmed, non-empty"},
		{name: "fact index bad topic", typ: "fact-index", value: map[string]any{"title": "View grammar", "topic": "cli view"}, wantErr: "invalid character"},
		{name: "list ok", typ: "list<label>", value: []any{"cli/ux", "type-system"}},
		{name: "list element invalid", typ: "list<label>", value: []any{"ok", ""}, wantErr: "item 1"},
		{name: "list not list", typ: "list<label>", value: "cli/ux", wantErr: "expected a list"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vt, err := ParseVarType(tt.typ)
			if err != nil {
				t.Fatal(err)
			}
			_, err = vt.ValidateValue(tt.value)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("ValidateValue = %v, want error containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("ValidateValue: %v", err)
			}
		})
	}
}

func TestValidateValueNormalizesFactIndex(t *testing.T) {
	value, err := (VarType{Base: TypeFactIndex}).ValidateValue(map[string]any{"title": "View grammar", "topic": "cli/view"})
	if err != nil {
		t.Fatal(err)
	}
	if value != (FactIndex{Title: "View grammar", Topic: "cli/view"}) {
		t.Fatalf("value = %#v", value)
	}
}

func TestValidateValueNormalizesLayer(t *testing.T) {
	vt, _ := ParseVarType("layer")
	v, err := vt.ValidateValue("tac")
	if err != nil {
		t.Fatal(err)
	}
	if v != "tactical" {
		t.Errorf("layer abbrev normalized to %q, want tactical", v)
	}
}
