package gollm_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/networkteam/sdd/internal/model"
	"github.com/networkteam/sdd/pkg/llm"

	gollmrunner "github.com/networkteam/sdd/internal/llm/gollm"
)

// problemChars are the characters that broke the checks' JSON output before:
// typographic quotes of several languages, an ASCII quote, braces, a
// backslash path, a regex, a line break.
const problemChars = "„Code einlösen“ «zitiert» ‚einfach‘ “English” an ASCII \" quote, Options{Zebra: true}, C:\\Projekte\\de.json, ^Code\\s+[A-Z0-9]{8}$, a line\nbreak, <b> & 🎉"

// Out of alphabetical order, so a schema that went through a Go map shows.
const wireSchema = `{"type":"object","properties":{"reasoning":{"type":"string"},"axis":{"type":"string","enum":["form"]}},"required":["reasoning","axis"],"additionalProperties":false}`

// wireCase drives a real request at a local listener: what reaches the
// provider is not inferable from the adapter code, since gollm copies option
// maps into the request body verbatim.
type wireCase struct {
	name     string
	cfg      func(url string) model.LLMConfig
	path     string
	response func(text string) string
	system   func(t *testing.T, body map[string]any) string
	user     func(t *testing.T, body map[string]any) string
	schema   func(t *testing.T, body map[string]any) any
}

var wireCases = []wireCase{
	{
		name: "openai",
		// A key OpenAI itself would reject must still reach a gateway that issued it.
		cfg: func(url string) model.LLMConfig {
			return model.LLMConfig{Provider: "openai", Model: "m", Endpoint: url, APIKeys: map[string]string{"openai": "op-notarealkeyaaaaaaaaaa"}}
		},
		path: "/v1/chat/completions",
		response: func(text string) string {
			return `{"choices":[{"message":{"content":` + jsonString(text) + `}}],"usage":{"prompt_tokens":11,"completion_tokens":22}}`
		},
		system: func(t *testing.T, body map[string]any) string { return messageContent(t, body, "system", "developer") },
		user:   func(t *testing.T, body map[string]any) string { return messageContent(t, body, "user") },
		schema: func(t *testing.T, body map[string]any) any {
			js := asMap(t, asMap(t, body["response_format"])["json_schema"])
			if js["strict"] != true {
				t.Errorf("json_schema.strict = %v, want true", js["strict"])
			}
			return js["schema"]
		},
	},
	{
		name: "anthropic",
		cfg: func(url string) model.LLMConfig {
			return model.LLMConfig{Provider: "anthropic", Model: "m", Endpoint: url, APIKeys: map[string]string{"anthropic": "op-notarealkeyaaaaaaaaaa"}}
		},
		path: "/v1/messages",
		response: func(text string) string {
			return `{"content":[{"type":"text","text":` + jsonString(text) + `}],"usage":{"input_tokens":11,"output_tokens":22}}`
		},
		system: func(t *testing.T, body map[string]any) string {
			var parts []string
			for _, block := range asSlice(t, body["system"]) {
				parts = append(parts, asMap(t, block)["text"].(string))
			}
			return strings.Join(parts, "\n\n")
		},
		user: func(t *testing.T, body map[string]any) string {
			var parts []string
			for _, m := range asSlice(t, body["messages"]) {
				for _, block := range asSlice(t, asMap(t, m)["content"]) {
					parts = append(parts, asMap(t, block)["text"].(string))
				}
			}
			return strings.Join(parts, "")
		},
		schema: func(t *testing.T, body map[string]any) any {
			return asMap(t, asMap(t, body["output_config"])["format"])["schema"]
		},
	},
	{
		name: "ollama",
		cfg: func(url string) model.LLMConfig {
			return model.LLMConfig{Provider: "ollama", Model: "m", OllamaEndpoint: url, Params: map[string]string{"think": "high"}}
		},
		path: "/api/generate",
		response: func(text string) string {
			return `{"model":"m","response":` + jsonString(text) + `,"done":true,"prompt_eval_count":11,"eval_count":22}`
		},
		system: func(t *testing.T, body map[string]any) string { s, _ := body["system"].(string); return s },
		user:   func(t *testing.T, body map[string]any) string { s, _ := body["prompt"].(string); return s },
		schema: func(t *testing.T, body map[string]any) any { return body["format"] },
	},
}

// A structured request reaches each provider with the schema in its native
// slot, byte for byte, the system prompt once in its own slot and the user
// prompt alone in the user content, and the problem characters unchanged on
// the way out and back; the result reports the provider's usage.
func TestRequestWire(t *testing.T) {
	for _, c := range wireCases {
		t.Run(c.name, func(t *testing.T) {
			var mu sync.Mutex
			var raw []byte
			var path string
			answer := `{"reasoning":` + jsonString(problemChars) + `,"axis":"form"}`
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/api/tags" { // Ollama's reachability probe
					_, _ = w.Write([]byte(`{"models":[]}`))
					return
				}
				body, _ := io.ReadAll(r.Body)
				mu.Lock()
				raw, path = body, r.URL.Path
				mu.Unlock()
				_, _ = w.Write([]byte(c.response(answer)))
			}))
			defer srv.Close()

			runner, err := gollmrunner.NewRunner(c.cfg(srv.URL))
			if err != nil {
				t.Fatalf("NewRunner: %v", err)
			}
			res, err := runner.Run(context.Background(), llm.Request{
				SystemPrompt:   "SYSTEM-BLOCK-MARKER " + problemChars,
				UserPrompt:     "USER-BLOCK-MARKER " + problemChars,
				ResponseSchema: json.RawMessage(wireSchema),
			})
			if err != nil {
				t.Fatalf("Run: %v", err)
			}

			mu.Lock()
			defer mu.Unlock()
			if path != c.path {
				t.Errorf("request path = %q, want %q", path, c.path)
			}
			if !bytes.Contains(raw, []byte(wireSchema)) {
				t.Errorf("schema not sent byte for byte: %s", raw)
			}
			if n := bytes.Count(raw, []byte("SYSTEM-BLOCK-MARKER")); n != 1 {
				t.Errorf("system prompt sent %d times, want once", n)
			}
			var body map[string]any
			if err := json.Unmarshal(raw, &body); err != nil {
				t.Fatalf("request body: %v", err)
			}
			if got, _ := json.Marshal(c.schema(t, body)); !jsonEqual(t, got, []byte(wireSchema)) {
				t.Errorf("schema in the provider's slot = %s", got)
			}
			if got := c.system(t, body); got != "SYSTEM-BLOCK-MARKER "+problemChars {
				t.Errorf("system slot = %q", got)
			}
			if got := c.user(t, body); got != "USER-BLOCK-MARKER "+problemChars {
				t.Errorf("user content = %q", got)
			}
			if res.Text != answer {
				t.Errorf("text = %q, want %q", res.Text, answer)
			}
			if res.Usage.InputTokens != 11 || res.Usage.OutputTokens != 22 {
				t.Errorf("usage not parsed: %+v", res.Usage)
			}
		})
	}
}

// A request without a schema asks for free text: no provider's schema slot is
// filled. Behaviour-affecting params reach the provider and the variant.
func TestRequestWireWithoutSchema(t *testing.T) {
	for _, c := range wireCases {
		t.Run(c.name, func(t *testing.T) {
			var raw []byte
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/api/tags" {
					_, _ = w.Write([]byte(`{"models":[]}`))
					return
				}
				raw, _ = io.ReadAll(r.Body)
				_, _ = w.Write([]byte(c.response("SERVED")))
			}))
			defer srv.Close()

			cfg := c.cfg(srv.URL)
			runner, err := gollmrunner.NewRunner(cfg)
			if err != nil {
				t.Fatalf("NewRunner: %v", err)
			}
			res, err := runner.Run(context.Background(), llm.Request{SystemPrompt: "S", UserPrompt: "U"})
			if err != nil {
				t.Fatalf("Run: %v", err)
			}
			for _, key := range []string{`"response_format"`, `"output_config"`, `"format"`} {
				if bytes.Contains(raw, []byte(key)) {
					t.Errorf("free-text request carries %s: %s", key, raw)
				}
			}
			if res.Text != "SERVED" {
				t.Errorf("text = %q, want SERVED", res.Text)
			}
			if cfg.Params["think"] != "" {
				var body map[string]any
				_ = json.Unmarshal(raw, &body)
				if body["think"] != "high" {
					t.Errorf("think = %v, want high", body["think"])
				}
				if res.Identity.Variant != "think=high" {
					t.Errorf("variant = %q, want think=high", res.Identity.Variant)
				}
			}
		})
	}
}

func jsonString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

func jsonEqual(t *testing.T, a, b []byte) bool {
	t.Helper()
	var x, y any
	if err := json.Unmarshal(a, &x); err != nil {
		t.Fatalf("decoding %s: %v", a, err)
	}
	if err := json.Unmarshal(b, &y); err != nil {
		t.Fatalf("decoding %s: %v", b, err)
	}
	ax, _ := json.Marshal(x)
	by, _ := json.Marshal(y)
	return bytes.Equal(ax, by)
}

func messageContent(t *testing.T, body map[string]any, roles ...string) string {
	t.Helper()
	var parts []string
	for _, m := range asSlice(t, body["messages"]) {
		msg := asMap(t, m)
		for _, role := range roles {
			if msg["role"] == role {
				parts = append(parts, msg["content"].(string))
			}
		}
	}
	return strings.Join(parts, "\n\n")
}

func asSlice(t *testing.T, v any) []any {
	t.Helper()
	s, ok := v.([]any)
	if !ok {
		t.Fatalf("want a JSON array, got %T", v)
	}
	return s
}

func asMap(t *testing.T, v any) map[string]any {
	t.Helper()
	m, ok := v.(map[string]any)
	if !ok {
		t.Fatalf("want a JSON object, got %T", v)
	}
	return m
}
