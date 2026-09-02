package embed

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/networkteam/sdd/internal/model"
	pkgembed "github.com/networkteam/sdd/pkg/llm/embed"
)

func TestApplyTemplate(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		tmpl  string
		input []string
		want  []string
	}{
		{
			name:  "empty template passes through unchanged",
			tmpl:  "",
			input: []string{"a", "b"},
			want:  []string{"a", "b"},
		},
		{
			name:  "qwen3-style instruction prefix",
			tmpl:  "Instruct: Given a query phrase, retrieve related entries from a knowledge graph\nQuery:{text}",
			input: []string{"how do augmenting directives work?"},
			want:  []string{"Instruct: Given a query phrase, retrieve related entries from a knowledge graph\nQuery:how do augmenting directives work?"},
		},
		{
			name:  "e5-style passage prefix",
			tmpl:  "passage: {text}",
			input: []string{"first", "second"},
			want:  []string{"passage: first", "passage: second"},
		},
		{
			name:  "placeholder repeated is replaced everywhere",
			tmpl:  "{text} | search this: {text}",
			input: []string{"x"},
			want:  []string{"x | search this: x"},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			got := applyTemplate(c.tmpl, c.input)
			if !reflect.DeepEqual(got, c.want) {
				t.Errorf("got %#v, want %#v", got, c.want)
			}
		})
	}
}

func TestAppendDocTemplateHash(t *testing.T) {
	t.Parallel()

	if got := appendDocTemplateHash("openai/text-embedding-3-small/1536", ""); got != "openai/text-embedding-3-small/1536" {
		t.Errorf("empty template should leave fingerprint unchanged; got %q", got)
	}

	a := appendDocTemplateHash("base", "passage: {text}")
	b := appendDocTemplateHash("base", "search_document: {text}")
	if a == b {
		t.Errorf("different templates must produce different fingerprints; got %q == %q", a, b)
	}
	if !strings.HasPrefix(a, "base/d:") {
		t.Errorf("fingerprint should carry /d: marker; got %q", a)
	}

	// Same template → same fingerprint (hash stability).
	a2 := appendDocTemplateHash("base", "passage: {text}")
	if a != a2 {
		t.Errorf("same template should produce stable fingerprint; got %q vs %q", a, a2)
	}
}

// TestEmbedder_DocumentTemplateAppliedAtTransport verifies that the
// document template lands on every input embedded for documents —
// the HTTP server captures the post-template payload and we assert on it.
// Mirror coverage lives below for queries.
func TestEmbedder_DocumentTemplateAppliedAtTransport(t *testing.T) {
	t.Parallel()

	var captured []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body openaiEmbedRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		captured = append(captured, body.Input...)
		w.Header().Set("Content-Type", "application/json")
		out := make([]map[string]any, len(body.Input))
		for i := range body.Input {
			out[i] = map[string]any{"embedding": []float32{0, 0}}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"data": out})
	}))
	defer srv.Close()

	emb, err := New(model.EmbeddingConfig{
		Provider:         "openai",
		Model:            "test-model",
		Endpoint:         srv.URL,
		APIKeys:          map[string]string{"openai": "k"},
		RateLimitRPS:     1000,
		QueryTemplate:    "QUERY: {text}",
		DocumentTemplate: "DOC: {text}",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := emb.Embed(context.Background(), pkgembed.Request{Purpose: pkgembed.PurposeDocument, Texts: []string{"alpha", "beta"}}); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(captured, []string{"DOC: alpha", "DOC: beta"}) {
		t.Errorf("expected DOC template applied; captured: %v", captured)
	}

	captured = nil
	if _, err := emb.Embed(context.Background(), pkgembed.Request{Purpose: pkgembed.PurposeQuery, Texts: []string{"gamma"}}); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(captured, []string{"QUERY: gamma"}) {
		t.Errorf("expected QUERY template applied; captured: %v", captured)
	}
}

// TestEmbedder_QueryTemplateNotInFingerprint pins the asymmetry: query
// template tweaks must NOT shift the fingerprint (free-tweak knob),
// while document template tweaks MUST (index invalidation).
func TestEmbedder_QueryTemplateNotInFingerprint(t *testing.T) {
	t.Parallel()

	cfgWithBoth := model.EmbeddingConfig{
		Provider:         "openai",
		Model:            "test-model",
		APIKeys:          map[string]string{"openai": "k"},
		Dimensions:       128,
		RateLimitRPS:     1000,
		QueryTemplate:    "QUERY-A: {text}",
		DocumentTemplate: "DOC: {text}",
	}
	cfgQueryChanged := cfgWithBoth
	cfgQueryChanged.QueryTemplate = "QUERY-B (totally different): {text}"

	cfgDocChanged := cfgWithBoth
	cfgDocChanged.DocumentTemplate = "OTHER-DOC: {text}"

	a, _ := New(cfgWithBoth)
	b, _ := New(cfgQueryChanged)
	c, _ := New(cfgDocChanged)

	if a.Fingerprint() != b.Fingerprint() {
		t.Errorf("query-template change must not shift fingerprint:\n  a=%q\n  b=%q",
			a.Fingerprint(), b.Fingerprint())
	}
	if a.Fingerprint() == c.Fingerprint() {
		t.Errorf("doc-template change must shift fingerprint; both produced %q", a.Fingerprint())
	}
}

// TestEmbedder_OllamaTemplateApplied mirrors the OpenAI document/query
// coverage on the Ollama transport — the wire shape differs but the
// template contract is the same.
func TestEmbedder_OllamaTemplateApplied(t *testing.T) {
	t.Parallel()

	var captured []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body ollamaEmbedRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		captured = append(captured, body.Input...)
		out := make([][]float32, len(body.Input))
		for i := range body.Input {
			out[i] = []float32{0, 0}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"embeddings": out})
	}))
	defer srv.Close()

	emb, err := New(model.EmbeddingConfig{
		Provider:         "ollama",
		Model:            "nomic-embed-text",
		OllamaEndpoint:   srv.URL,
		QueryTemplate:    "search_query: {text}",
		DocumentTemplate: "search_document: {text}",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := emb.Embed(context.Background(), pkgembed.Request{Purpose: pkgembed.PurposeDocument, Texts: []string{"x"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := emb.Embed(context.Background(), pkgembed.Request{Purpose: pkgembed.PurposeQuery, Texts: []string{"y"}}); err != nil {
		t.Fatal(err)
	}
	want := []string{"search_document: x", "search_query: y"}
	if !reflect.DeepEqual(captured, want) {
		t.Errorf("got %v, want %v", captured, want)
	}
}
