package model

import (
	"fmt"
	"time"

	"gopkg.in/yaml.v3"
)

const (
	// DefaultGraphDir is the conventional graph directory relative to repo root
	// when initialized with sdd init.
	DefaultGraphDir = ".sdd/graph"

	// SDDDirName is the metadata directory name at the repository root.
	SDDDirName = ".sdd"

	// DefaultLLMProvider is the provider used when none is configured. The
	// claude CLI bridge runs via the user's logged-in Claude Code session, so
	// no API key is required for first-run usage.
	DefaultLLMProvider = "claude-cli"

	// DefaultLLMModel is the claude model used when none is configured.
	DefaultLLMModel = "claude-haiku-4-5-20251001"

	// DefaultLLMConcurrency is the default worker count for concurrent
	// LLM calls (e.g. sdd summarize --all).
	DefaultLLMConcurrency = 4

	// DefaultSyncCooldown bounds how often background sync runs git fetch
	// when last-fetch exceeds this duration. Applied when Config.Sync.Cooldown
	// is empty or unparseable.
	DefaultSyncCooldown = "15m"
)

// Config represents the contents of .sdd/config.yaml (shared, committed) or
// .sdd/config.local.yaml (gitignored, per-machine). Both files unmarshal into
// the same struct; the local file overlays the shared file via MergeConfig.
// Empty / zero-valued fields in the local file mean "inherit from shared",
// so any subset of fields can appear in either file.
type Config struct {
	GraphDir string     `yaml:"graph_dir,omitempty"`
	LLM      LLMConfig  `yaml:"llm,omitempty"`
	Sync     SyncConfig `yaml:"sync,omitempty"`
	// Participant is the canonical name used for entry authorship when
	// --participants / --participant is omitted at capture time. Lives in
	// .sdd/config.local.yaml (gitignored) because the same person may use
	// different spellings across projects.
	Participant string `yaml:"participant,omitempty"`
	// Language is a locale code (e.g. "de", "en", "de-DE") that governs the
	// graph's authored language. Captured entries are written in this
	// language; the /sdd skill renders translated vocabulary to users via
	// bundled translation references. Empty means English (default).
	Language string `yaml:"language,omitempty"`
}

// SyncConfig governs background sync awareness: the auto-fetch cooldown and
// related behavior. Stored as a string Go duration (e.g. "15m") parsed at
// use site so malformed values fall back to DefaultSyncCooldown rather than
// failing at config load.
type SyncConfig struct {
	// Cooldown is the minimum interval between background git fetches. Go
	// duration string (e.g. "15m", "1h"). Empty means DefaultSyncCooldown.
	Cooldown string `yaml:"cooldown,omitempty"`
}

// LLMConfig holds settings for LLM provider selection, model choice, and
// concurrency/rate-limit behavior. API keys and per-machine endpoints
// typically live in .sdd/config.local.yaml; defaults (provider, model,
// timeout, concurrency) are safe to commit in .sdd/config.yaml.
type LLMConfig struct {
	// Provider selects the runner implementation: "claude-cli" (default, uses
	// the logged-in Claude Code session) or a gollm-supported provider name
	// such as "anthropic", "openai", "ollama".
	Provider string `yaml:"provider,omitempty"`
	// Model is the provider-specific model identifier.
	Model string `yaml:"model,omitempty"`
	// Timeout is a Go duration string (e.g. "2m") applied per LLM call.
	Timeout string `yaml:"timeout,omitempty"`
	// Concurrency bounds the worker pool for batch operations. Zero means
	// "use DefaultLLMConcurrency".
	Concurrency int `yaml:"concurrency,omitempty"`
	// OllamaEndpoint overrides the default Ollama URL for the gollm adapter.
	OllamaEndpoint string `yaml:"ollama_endpoint,omitempty"`
	// APIKeys maps provider name to API key. Typically lives in
	// config.local.yaml so keys stay out of version control.
	APIKeys map[string]string `yaml:"api_keys,omitempty"`
	// RateLimitRPS caps remote-provider requests per second (0 = uncapped).
	// The claude-cli and ollama providers ignore this.
	RateLimitRPS float64 `yaml:"rate_limit_rps,omitempty"`
}

// ParseConfig unmarshals YAML bytes into a Config struct. Empty input is
// valid and yields a zero-valued Config.
func ParseConfig(data []byte) (*Config, error) {
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}
	return &cfg, nil
}

// MergeConfig returns a new Config with fields from overlay overriding base
// wherever the overlay value is non-empty/non-zero. APIKeys are merged
// key-by-key so overlay entries replace individual providers without
// clobbering the full map. A nil overlay returns a copy of base.
func MergeConfig(base, overlay *Config) *Config {
	if base == nil {
		base = &Config{}
	}
	out := *base
	if overlay == nil {
		return &out
	}
	if overlay.GraphDir != "" {
		out.GraphDir = overlay.GraphDir
	}
	if overlay.Participant != "" {
		out.Participant = overlay.Participant
	}
	if overlay.Language != "" {
		out.Language = overlay.Language
	}
	out.LLM = mergeLLMConfig(base.LLM, overlay.LLM)
	out.Sync = mergeSyncConfig(base.Sync, overlay.Sync)
	return &out
}

func mergeSyncConfig(base, overlay SyncConfig) SyncConfig {
	out := base
	if overlay.Cooldown != "" {
		out.Cooldown = overlay.Cooldown
	}
	return out
}

func mergeLLMConfig(base, overlay LLMConfig) LLMConfig {
	out := base
	if overlay.Provider != "" {
		out.Provider = overlay.Provider
	}
	if overlay.Model != "" {
		out.Model = overlay.Model
	}
	if overlay.Timeout != "" {
		out.Timeout = overlay.Timeout
	}
	if overlay.Concurrency != 0 {
		out.Concurrency = overlay.Concurrency
	}
	if overlay.OllamaEndpoint != "" {
		out.OllamaEndpoint = overlay.OllamaEndpoint
	}
	if overlay.RateLimitRPS != 0 {
		out.RateLimitRPS = overlay.RateLimitRPS
	}
	if len(overlay.APIKeys) > 0 {
		if out.APIKeys == nil {
			out.APIKeys = make(map[string]string, len(overlay.APIKeys))
		} else {
			// Copy-on-write so the merge doesn't mutate base.
			copied := make(map[string]string, len(out.APIKeys)+len(overlay.APIKeys))
			for k, v := range out.APIKeys {
				copied[k] = v
			}
			out.APIKeys = copied
		}
		for k, v := range overlay.APIKeys {
			out.APIKeys[k] = v
		}
	}
	return out
}

// FormatConfig returns a commented YAML config template with the given graph
// dir. If cfg.Language is set, the locale is written as an active
// `language: <code>` entry. Otherwise a commented hint is emitted instead so
// the option stays discoverable in the file.
func FormatConfig(cfg Config) string {
	graphDir := cfg.GraphDir
	if graphDir == "" {
		graphDir = DefaultGraphDir
	}
	languageBlock := "# Graph language — locale code for the language captured entries are\n" +
		"# authored in. Empty means English (default). The /sdd skill reads the\n" +
		"# matching references/vocabulary-<locale>.md when rendering to users.\n"
	if cfg.Language != "" {
		languageBlock += "language: " + cfg.Language + "\n"
	} else {
		languageBlock += "# language: de\n"
	}
	return "# SDD configuration\n" +
		"# See https://github.com/networkteam/sdd for documentation.\n" +
		"\n" +
		"# Graph directory relative to repository root.\n" +
		"graph_dir: " + graphDir + "\n" +
		"\n" +
		languageBlock +
		"\n" +
		"# LLM provider settings (defaults shown — override here or in config.local.yaml).\n" +
		"# llm:\n" +
		"#   provider: " + DefaultLLMProvider + "\n" +
		"#   model: " + DefaultLLMModel + "\n" +
		"#   timeout: 2m\n" +
		"#   concurrency: 4\n" +
		"\n" +
		"# Background sync — controls how often the CLI auto-fetches to detect graph\n" +
		"# changes from collaborators. Go duration string.\n" +
		"# sync:\n" +
		"#   cooldown: " + DefaultSyncCooldown + "\n"
}

// ResolveSyncCooldown returns the effective cooldown duration from cfg,
// falling back to DefaultSyncCooldown on empty or unparseable values.
func ResolveSyncCooldown(cfg *Config) time.Duration {
	raw := ""
	if cfg != nil {
		raw = cfg.Sync.Cooldown
	}
	if raw == "" {
		raw = DefaultSyncCooldown
	}
	if d, err := time.ParseDuration(raw); err == nil && d > 0 {
		return d
	}
	d, _ := time.ParseDuration(DefaultSyncCooldown)
	return d
}
