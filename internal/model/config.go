package model

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strconv"
	"strings"
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
	// Sonnet-class, matching the model the pre-flight calibration eval
	// measures — pre-flight is the highest-stakes LLM call, and a blocking
	// validator should not run on a cheaper tier than it was calibrated
	// against (d-tac-b30).
	DefaultLLMModel = "claude-sonnet-4-6"

	// DefaultLLMConcurrency is the default worker count for concurrent
	// LLM calls (e.g. sdd summarize --all).
	DefaultLLMConcurrency = 4

	// DefaultRetryMaxAttempts is the total number of LLM call attempts
	// (1 = no retry) when the gollm runner hits a transient failure. Five
	// attempts ride out a connection blip or a short 429 without making an
	// interactive sdd new hang indefinitely.
	DefaultRetryMaxAttempts = 5

	// DefaultRetryBaseDelay is the exponential-backoff floor between retries.
	DefaultRetryBaseDelay = "2s"

	// DefaultRetryMaxDelay caps backoff growth between retries.
	DefaultRetryMaxDelay = "60s"

	// DefaultSyncCooldown bounds how often background sync runs git fetch
	// when last-fetch exceeds this duration. Applied when Config.Sync.Cooldown
	// is empty or unparseable.
	DefaultSyncCooldown = "15m"

	// DefaultSessionRetention is how long an ended engine session's log is
	// kept before collection removes it. Sessions are scaffolding, but a
	// dialogue that has just ended is exactly the one worth reading when
	// something went wrong inside it, so the window buys inspectability.
	DefaultSessionRetention = "14d"
)

// BaseConfig holds the shared user/machine settings every config location
// can carry: preferences that follow the user or the machine, not the
// repository. It is embedded inline by both PerRepoConfig and the user-global
// config, so the same keys are settable once globally and overridable per
// repo (d-cpt-6cq's unified overlay). Effective resolution order: global
// base, then the committed .sdd/config.yaml, then .sdd/config.local.yaml,
// then CLI flags.
type BaseConfig struct {
	LLM       LLMConfig       `yaml:"llm,omitempty"`
	Embedding EmbeddingConfig `yaml:"embedding,omitempty"`
	Sync      SyncConfig      `yaml:"sync,omitempty"`
	Sessions  SessionsConfig  `yaml:"sessions,omitempty"`
	// Participant is the canonical name used for entry authorship when
	// --participants / --participant is omitted at capture time. Typically
	// set once in the user-global config; a per-repo override covers a
	// project where the same person deliberately uses another spelling.
	Participant string `yaml:"participant,omitempty"`
}

// IsZero reports whether no base setting is set. Resolution uses it to
// distinguish "no global settings at all" from an empty overlay layer.
func (b BaseConfig) IsZero() bool {
	return reflect.DeepEqual(b, BaseConfig{})
}

// PerRepoConfig represents the contents of .sdd/config.yaml (shared,
// committed) or .sdd/config.local.yaml (gitignored, per-machine overrides).
// Both files unmarshal into the same struct; the local file overlays the
// shared file via MergeConfig. Empty / zero-valued fields in the local file
// mean "inherit", so any subset of fields can appear in either file. The
// repository's own fields exist only on this schema — never on the
// user-global config — so a misplaced repo_id is a parse error, not a
// silently inherited default.
type PerRepoConfig struct {
	BaseConfig `yaml:",inline"`

	GraphDir string `yaml:"graph_dir,omitempty"`
	// DefaultBranch is the concrete branch used by ordinary engine captures
	// when no workflow-selected branch is supplied. It is committed so a
	// long-lived server never infers mutation authority from its launch cwd.
	DefaultBranch string `yaml:"default_branch,omitempty"`
	// RepoID is the repo's canonical URL-shaped identity (host/path, e.g.
	// github.com/networkteam/sdd) used as the prefix of cross-repo
	// references into this graph. Auto-derived from the git remote by
	// `sdd init` and committed in .sdd/config.yaml — identical for every
	// user, never user-chosen. Empty for local-only repos.
	RepoID string `yaml:"repo_id,omitempty"`
	// Language is a locale code (e.g. "de", "en", "de-DE") that governs the
	// graph's authored language. Captured entries are written in this
	// language; the /sdd skill renders translated vocabulary to users via
	// bundled translation references. Empty means English (default). A
	// property of the repository (all contributors author in it), which is
	// why it is not part of the shared BaseConfig overlay.
	Language string `yaml:"language,omitempty"`
	// SkillScope records where the project's skills were installed: user
	// (~/.claude/skills/) or project (.claude/skills/). `sdd init` writes
	// it the first time scope is chosen and reads it on every subsequent
	// run so the installed location stays stable for every contributor on
	// the repo. Empty means "no recorded preference" — typical of graphs
	// initialized before the readiness-check work landed.
	SkillScope Scope `yaml:"skill_scope,omitempty"`
	// SupportedAgents lists the agent targets `sdd init` renders skills for
	// (e.g. claude, codex). A project-level committed decision in
	// .sdd/config.yaml: every contributor renders the same set, and each
	// agent reads only its own dir. Chosen via multi-select on first run.
	// Empty means the pre-multi-agent default (Claude alone).
	SupportedAgents []AgentTarget `yaml:"supported_agents,omitempty"`
	// Dependencies is the committed, directed list of repo_ids this graph
	// references — declared the way go.mod declares modules, so a fresh
	// clone carries what it needs connected. How this machine reaches each
	// dependency (clone URL, cache) is per-user and lives in the global
	// config's repos list, never here.
	Dependencies []string `yaml:"dependencies,omitempty"`
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

// SessionsConfig holds settings for engine session lifetime.
type SessionsConfig struct {
	// Retention is how long an ended session is kept before collection
	// removes it. Days ("14d") or a Go duration ("336h"). Empty means
	// DefaultSessionRetention.
	Retention string `yaml:"retention,omitempty"`
}

// EmbeddingConfig holds settings for the search index's embedding provider.
// Decoupled from LLMConfig (chat / summary) so a participant can run a
// local Ollama embedder while still using a remote chat provider, and
// vice versa. Typically set once in the user-global config: cross-repo
// search fuses per-repo indexes under one shared embedder, so a per-repo
// override leaves that shared vector space (lint flags the fingerprint
// mismatch) — possible, but a choice with consequences.
type EmbeddingConfig struct {
	// Provider names the embedder transport: "openai" (OpenAI-compatible
	// /v1/embeddings) or "ollama" (/api/embeddings). Empty disables vector
	// search — the CLI falls back to text-mode-only.
	Provider string `yaml:"provider,omitempty"`
	// Model is the provider-specific embedding model identifier
	// (e.g. "text-embedding-3-small", "nomic-embed-text").
	Model string `yaml:"model,omitempty"`
	// Endpoint overrides the OpenAI-compatible base URL. Empty defaults to
	// the OpenAI public endpoint. Useful for self-hosted proxies that
	// implement the same wire protocol.
	Endpoint string `yaml:"endpoint,omitempty"`
	// OllamaEndpoint overrides the default Ollama URL for the embedding
	// adapter. Independent of LLMConfig.OllamaEndpoint so embedding and
	// chat can target different instances.
	OllamaEndpoint string `yaml:"ollama_endpoint,omitempty"`
	// APIKeys maps provider name to API key. Defaults to LLMConfig.APIKeys
	// if empty so a single key value can serve both axes; explicit values
	// here override that.
	APIKeys map[string]string `yaml:"api_keys,omitempty"`
	// RateLimitRPS caps remote-provider requests per second. Zero means
	// "apply a conservative per-provider default safe for tier-1 limits".
	// Local providers (ollama) ignore this field.
	RateLimitRPS float64 `yaml:"rate_limit_rps,omitempty"`
	// Timeout is a Go duration string (e.g. "30s") applied per Embed call.
	Timeout string `yaml:"timeout,omitempty"`
	// Dimensions optionally overrides the embedded vector size — used for
	// OpenAI's matryoshka-style truncation. Zero means "use the model's
	// native dimension".
	Dimensions int `yaml:"dimensions,omitempty"`
	// BatchSize bounds the number of inputs sent in a single embedding
	// request. Zero means "use a provider-specific default" (100 for
	// openai, 64 for ollama). Override when working with very large or
	// very small inputs to balance throughput against per-call timeout.
	BatchSize int `yaml:"batch_size,omitempty"`
	// QueryTemplate is applied to every text passed through
	// EmbedQueries before the transport call. The literal `{text}` is
	// replaced with the input. Empty disables the transformation
	// (matches OpenAI's prefix-agnostic behavior). Used by retrieval
	// (sdd search). Changing this is a free-tweak — query template
	// changes do not invalidate indexed embeddings.
	//
	// Example for Qwen3 / instruction-tuned encoders:
	//
	//   query_template: |-
	//     Instruct: Given a query phrase, retrieve related entries from a knowledge graph
	//     Query:{text}
	QueryTemplate string `yaml:"query_template,omitempty"`
	// DocumentTemplate is applied to every text passed through
	// EmbedDocuments before the transport call. Same `{text}`
	// substitution as QueryTemplate. Empty disables. Used by indexing
	// (sdd index, sdd search lazy-fill).
	//
	// Document templates feed into the embedder Fingerprint — changing
	// this invalidates the index and triggers re-embed on the next
	// search (or eagerly via `sdd index --force`). Required for
	// dual-prefix models (E5 `passage:`, Nomic `search_document:`);
	// stays empty for query-only models (Qwen3, BGE) and untemplated
	// models (OpenAI).
	DocumentTemplate string `yaml:"document_template,omitempty"`
}

// LLMConfig holds settings for LLM provider selection, model choice, and
// concurrency/rate-limit behavior. API keys and personal defaults live in
// the uncommitted user-global config (per machine: .sdd/config.local.yaml);
// defaults (provider, model, timeout, concurrency) are safe to commit in
// .sdd/config.yaml.
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
	// APIKeys maps provider name to API key. Lives in the user-global or
	// machine-local config, never the committed project file.
	APIKeys map[string]string `yaml:"api_keys,omitempty"`
	// RateLimitRPS caps remote-provider requests per second. Zero means
	// "apply a conservative per-model default safe for Anthropic/OpenAI
	// tier 1"; set an explicit positive value (e.g. a high number like
	// 100) to effectively disable the cap on higher tiers. The claude-cli
	// and ollama providers ignore this field.
	RateLimitRPS float64 `yaml:"rate_limit_rps,omitempty"`
	// Retry tunes the provider-aware retry policy applied by the gollm
	// runner (transient-error classification, server-provided backoff,
	// exponential backoff with jitter). Ignored by the claude-cli provider,
	// whose own CLI handles retries.
	Retry RetryConfig `yaml:"retry,omitempty"`
}

// RetryConfig tunes retry/backoff behavior for the gollm-backed runner. Each
// zero-valued field falls back to the corresponding DefaultRetry* constant via
// Resolved, so an absent retry block yields the baked defaults.
type RetryConfig struct {
	// MaxAttempts is the total number of attempts per call (1 = no retry).
	// Zero means DefaultRetryMaxAttempts.
	MaxAttempts int `yaml:"max_attempts,omitempty"`
	// BaseDelay is the exponential-backoff floor as a Go duration string
	// (e.g. "2s"). Empty means DefaultRetryBaseDelay.
	BaseDelay string `yaml:"base_delay,omitempty"`
	// MaxDelay caps backoff growth as a Go duration string (e.g. "60s").
	// Empty means DefaultRetryMaxDelay.
	MaxDelay string `yaml:"max_delay,omitempty"`
}

// Resolved applies the baked defaults to any zero-valued field and parses the
// duration strings. An invalid duration returns an error so misconfiguration
// surfaces when the runner is constructed rather than silently degrading.
func (rc RetryConfig) Resolved() (maxAttempts int, baseDelay, maxDelay time.Duration, err error) {
	maxAttempts = rc.MaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = DefaultRetryMaxAttempts
	}

	baseStr := rc.BaseDelay
	if baseStr == "" {
		baseStr = DefaultRetryBaseDelay
	}
	baseDelay, err = time.ParseDuration(baseStr)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("parsing retry base_delay %q: %w", baseStr, err)
	}

	maxStr := rc.MaxDelay
	if maxStr == "" {
		maxStr = DefaultRetryMaxDelay
	}
	maxDelay, err = time.ParseDuration(maxStr)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("parsing retry max_delay %q: %w", maxStr, err)
	}

	return maxAttempts, baseDelay, maxDelay, nil
}

// ParseConfig unmarshals YAML bytes into a PerRepoConfig. Empty input is
// valid and yields a zero-valued config.
func ParseConfig(data []byte) (*PerRepoConfig, error) {
	var cfg PerRepoConfig
	if err := UnmarshalYAML(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}
	return &cfg, nil
}

// UnmarshalYAML decodes YAML tolerantly: a key this binary does not know is
// carried past rather than rejected, because a config file is shared between
// sdd versions and one must not be bricked by a key a newer one wrote
// (20260810-144515-s-tac-8ae).
func UnmarshalYAML(data []byte, out any) error {
	dec := yaml.NewDecoder(bytes.NewReader(data))
	if err := dec.Decode(out); err != nil {
		if errors.Is(err, io.EOF) {
			return nil
		}
		return err
	}
	return nil
}

// MergeBaseConfig returns base with the non-empty/non-zero fields of overlay
// applied. APIKeys merge key-by-key so overlay entries replace individual
// providers without clobbering the full map.
func MergeBaseConfig(base, overlay BaseConfig) BaseConfig {
	out := base
	if overlay.Participant != "" {
		out.Participant = overlay.Participant
	}
	out.LLM = mergeLLMConfig(base.LLM, overlay.LLM)
	out.Embedding = mergeEmbeddingConfig(base.Embedding, overlay.Embedding)
	out.Sync = mergeSyncConfig(base.Sync, overlay.Sync)
	return out
}

// MergeConfig returns a new PerRepoConfig with fields from overlay overriding
// base wherever the overlay value is non-empty/non-zero. A nil overlay
// returns a copy of base. Used for every adjacent layer pair of the overlay:
// global base under committed config, committed config under local overrides.
func MergeConfig(base, overlay *PerRepoConfig) *PerRepoConfig {
	if base == nil {
		base = &PerRepoConfig{}
	}
	out := *base
	if overlay == nil {
		return &out
	}
	out.BaseConfig = MergeBaseConfig(base.BaseConfig, overlay.BaseConfig)
	if overlay.GraphDir != "" {
		out.GraphDir = overlay.GraphDir
	}
	if overlay.DefaultBranch != "" {
		out.DefaultBranch = overlay.DefaultBranch
	}
	if overlay.RepoID != "" {
		out.RepoID = overlay.RepoID
	}
	if overlay.Language != "" {
		out.Language = overlay.Language
	}
	if overlay.SkillScope != "" {
		out.SkillScope = overlay.SkillScope
	}
	if len(overlay.SupportedAgents) > 0 {
		out.SupportedAgents = overlay.SupportedAgents
	}
	if len(overlay.Dependencies) > 0 {
		out.Dependencies = overlay.Dependencies
	}
	return &out
}

func mergeEmbeddingConfig(base, overlay EmbeddingConfig) EmbeddingConfig {
	out := base
	if overlay.Provider != "" {
		out.Provider = overlay.Provider
	}
	if overlay.Model != "" {
		out.Model = overlay.Model
	}
	if overlay.Endpoint != "" {
		out.Endpoint = overlay.Endpoint
	}
	if overlay.OllamaEndpoint != "" {
		out.OllamaEndpoint = overlay.OllamaEndpoint
	}
	if overlay.RateLimitRPS != 0 {
		out.RateLimitRPS = overlay.RateLimitRPS
	}
	if overlay.Timeout != "" {
		out.Timeout = overlay.Timeout
	}
	if overlay.Dimensions != 0 {
		out.Dimensions = overlay.Dimensions
	}
	if overlay.BatchSize != 0 {
		out.BatchSize = overlay.BatchSize
	}
	if overlay.QueryTemplate != "" {
		out.QueryTemplate = overlay.QueryTemplate
	}
	if overlay.DocumentTemplate != "" {
		out.DocumentTemplate = overlay.DocumentTemplate
	}
	if len(overlay.APIKeys) > 0 {
		copied := make(map[string]string, len(out.APIKeys)+len(overlay.APIKeys))
		for k, v := range out.APIKeys {
			copied[k] = v
		}
		for k, v := range overlay.APIKeys {
			copied[k] = v
		}
		out.APIKeys = copied
	}
	return out
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
	out.Retry = mergeRetryConfig(base.Retry, overlay.Retry)
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

func mergeRetryConfig(base, overlay RetryConfig) RetryConfig {
	out := base
	if overlay.MaxAttempts != 0 {
		out.MaxAttempts = overlay.MaxAttempts
	}
	if overlay.BaseDelay != "" {
		out.BaseDelay = overlay.BaseDelay
	}
	if overlay.MaxDelay != "" {
		out.MaxDelay = overlay.MaxDelay
	}
	return out
}

// FormatConfig returns a commented YAML config template with the given graph
// dir. If cfg.Language is set, the locale is written as an active
// `language: <code>` entry. Otherwise a commented hint is emitted instead so
// the option stays discoverable in the file.
func FormatConfig(cfg PerRepoConfig) string {
	graphDir := cfg.GraphDir
	if graphDir == "" {
		graphDir = DefaultGraphDir
	}
	repoIDBlock := "# Canonical repo identity for cross-repo references — URL-shaped\n" +
		"# (host/path), auto-derived from the git remote by `sdd init` and\n" +
		"# identical for every user. Other graphs reference entries here as\n" +
		"# <repo_id>:<entry-id>. Empty means local-only (no remote identity).\n"
	if cfg.RepoID != "" {
		repoIDBlock += "repo_id: " + cfg.RepoID + "\n"
	} else {
		repoIDBlock += "# repo_id: github.com/org/repo\n"
	}
	defaultBranchBlock := "# Concrete branch for ordinary engine captures. Implementation runs carry\n" +
		"# explicit base/work branches instead; cwd never selects mutation authority.\n"
	if cfg.DefaultBranch != "" {
		defaultBranchBlock += "default_branch: " + cfg.DefaultBranch + "\n"
	} else {
		defaultBranchBlock += "# default_branch: main\n"
	}
	languageBlock := "# Graph language — locale code for the language captured entries are\n" +
		"# authored in. Empty means English (default). The /sdd skill reads the\n" +
		"# matching references/vocabulary-<locale>.md when rendering to users.\n"
	if cfg.Language != "" {
		languageBlock += "language: " + cfg.Language + "\n"
	} else {
		languageBlock += "# language: de\n"
	}
	skillScopeBlock := "# Skill installation scope — where `sdd init` extracts the agent skill\n" +
		"# bundle. `user` installs into the user-global directory (e.g.\n" +
		"# ~/.claude/skills/); `project` installs into the repo-local\n" +
		"# .claude/skills/ tree. Recorded on first run so subsequent runs (and\n" +
		"# clones) reinstall to the same place without re-prompting.\n"
	if cfg.SkillScope != "" {
		skillScopeBlock += "skill_scope: " + string(cfg.SkillScope) + "\n"
	} else {
		skillScopeBlock += "# skill_scope: project\n"
	}
	supportedAgentsBlock := "# Agent targets `sdd init` renders skills for. Each listed agent gets its\n" +
		"# own rendered, committed skill dir (claude → .claude/skills/, codex →\n" +
		"# .agents/skills/). Chosen via multi-select on first run; every contributor\n" +
		"# on the repo renders the same set.\n"
	if len(cfg.SupportedAgents) > 0 {
		names := make([]string, len(cfg.SupportedAgents))
		for i, a := range cfg.SupportedAgents {
			names[i] = string(a)
		}
		supportedAgentsBlock += "supported_agents: [" + strings.Join(names, ", ") + "]\n"
	} else {
		supportedAgentsBlock += "# supported_agents: [claude]\n"
	}
	return "# SDD configuration\n" +
		"# See https://github.com/networkteam/sdd for documentation.\n" +
		"\n" +
		"# Graph directory relative to repository root.\n" +
		"graph_dir: " + graphDir + "\n" +
		"\n" +
		defaultBranchBlock +
		"\n" +
		repoIDBlock +
		"\n" +
		languageBlock +
		"\n" +
		skillScopeBlock +
		"\n" +
		supportedAgentsBlock +
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

// ResolveSessionRetention returns the effective session retention from cfg.
// Retention is expressed in days ("14d") or as a Go duration; empty means
// DefaultSessionRetention. A value that does not parse is a config error the
// caller must surface, never a silent default.
func ResolveSessionRetention(cfg *PerRepoConfig) (time.Duration, error) {
	raw := ""
	if cfg != nil {
		raw = strings.TrimSpace(cfg.Sessions.Retention)
	}
	if raw == "" {
		raw = DefaultSessionRetention
	}
	d, err := parseDaysOrDuration(raw)
	if err != nil || d <= 0 {
		return 0, fmt.Errorf("sessions.retention: %q is not a positive duration — use days (%q) or a Go duration (%q)", raw, "14d", "336h")
	}
	return d, nil
}

// parseDaysOrDuration reads a duration that may use a whole-day suffix,
// which time.ParseDuration does not know.
func parseDaysOrDuration(raw string) (time.Duration, error) {
	if days, ok := strings.CutSuffix(raw, "d"); ok {
		n, err := strconv.Atoi(days)
		if err != nil {
			return 0, fmt.Errorf("invalid day count %q", raw)
		}
		return time.Duration(n) * 24 * time.Hour, nil
	}
	return time.ParseDuration(raw)
}

// ResolveSyncCooldown returns the effective cooldown duration from cfg,
// falling back to DefaultSyncCooldown on empty or unparseable values.
func ResolveSyncCooldown(cfg *PerRepoConfig) time.Duration {
	raw := ""
	if cfg != nil {
		raw = cfg.Sync.Cooldown
	}
	return parsePositiveDuration(raw, DefaultSyncCooldown)
}

// parsePositiveDuration reads a configured duration, falling back to a baked
// default when it is empty, malformed or not positive.
func parsePositiveDuration(raw, fallback string) time.Duration {
	if raw != "" {
		if d, err := time.ParseDuration(raw); err == nil && d > 0 {
			return d
		}
	}
	d, _ := time.ParseDuration(fallback)
	return d
}
