package model

import (
	"reflect"
	"slices"
	"testing"
	"time"
)

func TestMergeConfig_EmptyOverlayPreservesBase(t *testing.T) {
	base := &PerRepoConfig{
		BaseConfig: BaseConfig{LLM: LLMConfig{
			Provider:    "claude-cli",
			Model:       "claude-haiku-4-5-20251001",
			Concurrency: 4,
		}},
		GraphDir: ".sdd/graph",
	}
	got := MergeConfig(base, &PerRepoConfig{})
	if got.GraphDir != ".sdd/graph" || got.LLM.Provider != "claude-cli" || got.LLM.Concurrency != 4 {
		t.Errorf("empty overlay should preserve base, got %+v", got)
	}
}

func TestMergeConfig_NonEmptyOverlayOverrides(t *testing.T) {
	base := &PerRepoConfig{
		BaseConfig: BaseConfig{LLM: LLMConfig{Provider: "claude-cli", Model: "claude-haiku-4-5-20251001", Concurrency: 4}},
	}
	overlay := &PerRepoConfig{
		BaseConfig: BaseConfig{LLM: LLMConfig{Provider: "ollama", Model: "llama3.1:70b"}},
	}
	got := MergeConfig(base, overlay)
	if got.LLM.Provider != "ollama" {
		t.Errorf("Provider = %q, want ollama", got.LLM.Provider)
	}
	if got.LLM.Model != "llama3.1:70b" {
		t.Errorf("Model = %q, want llama3.1:70b", got.LLM.Model)
	}
	if got.LLM.Concurrency != 4 {
		t.Errorf("Concurrency inherited from base = %d, want 4", got.LLM.Concurrency)
	}
}

func TestMergeConfig_APIKeysMerge(t *testing.T) {
	base := &PerRepoConfig{
		BaseConfig: BaseConfig{LLM: LLMConfig{APIKeys: map[string]string{"anthropic": "base-key", "openai": "base-openai"}}},
	}
	overlay := &PerRepoConfig{
		BaseConfig: BaseConfig{LLM: LLMConfig{APIKeys: map[string]string{"anthropic": "local-key", "ollama": "local-ollama"}}},
	}
	got := MergeConfig(base, overlay)
	if got.LLM.APIKeys["anthropic"] != "local-key" {
		t.Errorf("overlay should override anthropic key, got %q", got.LLM.APIKeys["anthropic"])
	}
	if got.LLM.APIKeys["openai"] != "base-openai" {
		t.Errorf("base openai key should be preserved, got %q", got.LLM.APIKeys["openai"])
	}
	if got.LLM.APIKeys["ollama"] != "local-ollama" {
		t.Errorf("overlay should add ollama key, got %q", got.LLM.APIKeys["ollama"])
	}
	// Base must not be mutated.
	if len(base.LLM.APIKeys) != 2 || base.LLM.APIKeys["anthropic"] != "base-key" {
		t.Errorf("MergeConfig mutated base APIKeys: %+v", base.LLM.APIKeys)
	}
}

func TestMergeConfig_NilOverlay(t *testing.T) {
	base := &PerRepoConfig{BaseConfig: BaseConfig{LLM: LLMConfig{Provider: "claude-cli"}}}
	got := MergeConfig(base, nil)
	if got.LLM.Provider != "claude-cli" {
		t.Errorf("nil overlay should return copy of base, got %+v", got)
	}
	// Ensure it's a copy, not the same pointer.
	got.LLM.Provider = "mutated"
	if base.LLM.Provider != "claude-cli" {
		t.Error("MergeConfig returned a pointer to base; must return a copy")
	}
}

func TestMergeConfig_NilBase(t *testing.T) {
	got := MergeConfig(nil, &PerRepoConfig{BaseConfig: BaseConfig{LLM: LLMConfig{Provider: "ollama"}}})
	if got == nil || got.LLM.Provider != "ollama" {
		t.Errorf("nil base with overlay should produce overlay values, got %+v", got)
	}
}

func TestMergeConfig_ParticipantOverlay(t *testing.T) {
	base := &PerRepoConfig{}
	overlay := &PerRepoConfig{BaseConfig: BaseConfig{Participant: "Christopher"}}
	got := MergeConfig(base, overlay)
	if got.Participant != "Christopher" {
		t.Errorf("Participant = %q, want Christopher", got.Participant)
	}
}

func TestMergeConfig_SyncOverlay(t *testing.T) {
	base := &PerRepoConfig{BaseConfig: BaseConfig{Sync: SyncConfig{Cooldown: "5m"}}}
	overlay := &PerRepoConfig{BaseConfig: BaseConfig{Sync: SyncConfig{Cooldown: "1h"}}}
	got := MergeConfig(base, overlay)
	if got.Sync.Cooldown != "1h" {
		t.Errorf("Sync.Cooldown = %q, want 1h", got.Sync.Cooldown)
	}

	// Empty overlay preserves base.
	got = MergeConfig(base, &PerRepoConfig{})
	if got.Sync.Cooldown != "5m" {
		t.Errorf("empty overlay should preserve base Sync.Cooldown = %q", got.Sync.Cooldown)
	}
}

func TestMergeConfig_DependenciesOverlay(t *testing.T) {
	base := &PerRepoConfig{Dependencies: []string{"github.com/org/one"}}
	got := MergeConfig(base, &PerRepoConfig{})
	if len(got.Dependencies) != 1 || got.Dependencies[0] != "github.com/org/one" {
		t.Errorf("empty overlay should preserve dependencies, got %v", got.Dependencies)
	}
	got = MergeConfig(base, &PerRepoConfig{Dependencies: []string{"github.com/org/two"}})
	if len(got.Dependencies) != 1 || got.Dependencies[0] != "github.com/org/two" {
		t.Errorf("overlay should replace dependencies, got %v", got.Dependencies)
	}
}

func TestMergeBaseConfig_LayersCompose(t *testing.T) {
	global := BaseConfig{
		Participant: "Christopher",
		LLM:         LLMConfig{Provider: "claude-cli", Model: "global-model"},
		Embedding:   EmbeddingConfig{Provider: "ollama", Model: "nomic-embed-text"},
	}
	repo := BaseConfig{LLM: LLMConfig{Model: "repo-model"}}
	got := MergeBaseConfig(global, repo)
	if got.Participant != "Christopher" {
		t.Errorf("Participant should survive from global, got %q", got.Participant)
	}
	if got.LLM.Provider != "claude-cli" || got.LLM.Model != "repo-model" {
		t.Errorf("repo overlay should override only what it sets, got %+v", got.LLM)
	}
	if got.Embedding.Provider != "ollama" {
		t.Errorf("Embedding should survive from global, got %+v", got.Embedding)
	}
}

// ParseConfig must accept base settings inline at the top level of a per-repo
// file — the same keys the user-global config carries — next to the
// repo-owned fields.
func TestParseConfig_InlineBaseFields(t *testing.T) {
	cfg, err := ParseConfig([]byte("participant: Christopher\ngraph_dir: .sdd/graph\nllm:\n  provider: ollama\ndependencies:\n  - github.com/org/dep\n"))
	if err != nil {
		t.Fatalf("ParseConfig: %v", err)
	}
	if cfg.Participant != "Christopher" || cfg.GraphDir != ".sdd/graph" || cfg.LLM.Provider != "ollama" {
		t.Errorf("inline base fields not decoded: %+v", cfg)
	}
	if len(cfg.Dependencies) != 1 || cfg.Dependencies[0] != "github.com/org/dep" {
		t.Errorf("dependencies not decoded: %v", cfg.Dependencies)
	}
}

func TestParseConfig_EmptyInput(t *testing.T) {
	cfg, err := ParseConfig(nil)
	if err != nil {
		t.Fatalf("empty input should be valid, got %v", err)
	}
	if cfg == nil {
		t.Fatal("empty input should yield zero config, got nil")
	}
}

// A key this binary does not know is read past rather than rejected, and
// reported by name — top-level and nested alike.
func TestParseConfig_TolerantAndReportsUnknownKeys(t *testing.T) {
	cases := []struct {
		name string
		yaml string
		key  string
	}{
		{"top-level unknown", "participant: x\nbogus_key: 1\n", "bogus_key"},
		{"nested unknown", "llm:\n  provider: ollama\n  bogus_nested: 1\n", "llm.bogus_nested"},
		{"global-only key misplaced", "repos:\n  - repo_id: a/b\n", "repos"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := ParseConfig([]byte(tc.yaml)); err != nil {
				t.Fatalf("unknown key %q must not fail the load: %v", tc.key, err)
			}
			unknown, err := UnknownYAMLKeys([]byte(tc.yaml), reflect.TypeFor[PerRepoConfig]())
			if err != nil {
				t.Fatal(err)
			}
			if !slices.Contains(unknown, tc.key) {
				t.Errorf("unknown keys %v should name %q", unknown, tc.key)
			}
		})
	}
}

// API-key maps keep accepting arbitrary provider names.
func TestParseConfig_MapKeysStayOpen(t *testing.T) {
	cfg, err := ParseConfig([]byte("llm:\n  api_keys:\n    anthropic: k1\n    custom-proxy: k2\n"))
	if err != nil {
		t.Fatalf("ParseConfig: %v", err)
	}
	if cfg.LLM.APIKeys["custom-proxy"] != "k2" {
		t.Errorf("map keys should stay open, got %+v", cfg.LLM.APIKeys)
	}
}

func TestRetryConfig_Resolved(t *testing.T) {
	// Empty config yields the baked defaults.
	maxAttempts, baseD, maxD, err := RetryConfig{}.Resolved()
	if err != nil {
		t.Fatalf("default resolve errored: %v", err)
	}
	if maxAttempts != DefaultRetryMaxAttempts {
		t.Errorf("MaxAttempts = %d, want %d", maxAttempts, DefaultRetryMaxAttempts)
	}
	if baseD != 2*time.Second || maxD != 60*time.Second {
		t.Errorf("default delays = (%v, %v), want (2s, 60s)", baseD, maxD)
	}

	// Explicit values are honored.
	maxAttempts, baseD, maxD, err = RetryConfig{MaxAttempts: 3, BaseDelay: "500ms", MaxDelay: "10s"}.Resolved()
	if err != nil {
		t.Fatalf("explicit resolve errored: %v", err)
	}
	if maxAttempts != 3 || baseD != 500*time.Millisecond || maxD != 10*time.Second {
		t.Errorf("explicit resolve = (%d, %v, %v), want (3, 500ms, 10s)", maxAttempts, baseD, maxD)
	}

	// A non-positive MaxAttempts falls back to the default.
	if got, _, _, _ := (RetryConfig{MaxAttempts: -1}).Resolved(); got != DefaultRetryMaxAttempts {
		t.Errorf("negative MaxAttempts = %d, want default %d", got, DefaultRetryMaxAttempts)
	}

	// Invalid durations surface an error rather than degrading silently.
	if _, _, _, err := (RetryConfig{BaseDelay: "soon"}).Resolved(); err == nil {
		t.Error("invalid base_delay should error")
	}
	if _, _, _, err := (RetryConfig{MaxDelay: "later"}).Resolved(); err == nil {
		t.Error("invalid max_delay should error")
	}
}

func TestMergeConfig_RetryOverlay(t *testing.T) {
	base := &PerRepoConfig{BaseConfig: BaseConfig{LLM: LLMConfig{Retry: RetryConfig{MaxAttempts: 5, BaseDelay: "2s", MaxDelay: "60s"}}}}

	// Overlay overrides only the fields it sets.
	overlay := &PerRepoConfig{BaseConfig: BaseConfig{LLM: LLMConfig{Retry: RetryConfig{MaxAttempts: 3}}}}
	got := MergeConfig(base, overlay)
	if got.LLM.Retry.MaxAttempts != 3 {
		t.Errorf("MaxAttempts = %d, want 3 (overridden)", got.LLM.Retry.MaxAttempts)
	}
	if got.LLM.Retry.BaseDelay != "2s" || got.LLM.Retry.MaxDelay != "60s" {
		t.Errorf("unset overlay fields should inherit base, got %+v", got.LLM.Retry)
	}

	// Empty overlay preserves base.
	got = MergeConfig(base, &PerRepoConfig{})
	if got.LLM.Retry.MaxAttempts != 5 || got.LLM.Retry.BaseDelay != "2s" {
		t.Errorf("empty overlay should preserve base retry, got %+v", got.LLM.Retry)
	}
}

func TestResolveSessionRetention(t *testing.T) {
	retention := func(raw string) *PerRepoConfig {
		return &PerRepoConfig{BaseConfig: BaseConfig{Sessions: SessionsConfig{Retention: raw}}}
	}
	cases := []struct {
		name    string
		cfg     *PerRepoConfig
		want    time.Duration
		wantErr bool
	}{
		{"nil config uses default", nil, 14 * 24 * time.Hour, false},
		{"empty value uses default", &PerRepoConfig{}, 14 * 24 * time.Hour, false},
		{"days", retention("1d"), 24 * time.Hour, false},
		{"go duration", retention("336h"), 336 * time.Hour, false},
		{"garbage is a config error", retention("not-a-duration"), 0, true},
		{"fractional days are a config error", retention("1.5d"), 0, true},
		{"zero is a config error", retention("0d"), 0, true},
		{"negative is a config error", retention("-1d"), 0, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ResolveSessionRetention(tc.cfg)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("want error, got %v", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestResolveSyncCooldown(t *testing.T) {
	cases := []struct {
		name   string
		cfg    *PerRepoConfig
		wantMS int64
	}{
		{"nil config uses default", nil, 15 * 60 * 1000},
		{"empty value uses default", &PerRepoConfig{}, 15 * 60 * 1000},
		{"valid override", &PerRepoConfig{BaseConfig: BaseConfig{Sync: SyncConfig{Cooldown: "2m"}}}, 2 * 60 * 1000},
		{"garbage falls back to default", &PerRepoConfig{BaseConfig: BaseConfig{Sync: SyncConfig{Cooldown: "not-a-duration"}}}, 15 * 60 * 1000},
		{"zero falls back to default", &PerRepoConfig{BaseConfig: BaseConfig{Sync: SyncConfig{Cooldown: "0s"}}}, 15 * 60 * 1000},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ResolveSyncCooldown(tc.cfg).Milliseconds()
			if got != tc.wantMS {
				t.Errorf("got %d ms, want %d ms", got, tc.wantMS)
			}
		})
	}
}
