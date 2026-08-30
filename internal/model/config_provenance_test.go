package model

import (
	"testing"
)

func findValue(t *testing.T, values []ConfigValue, key string) ConfigValue {
	t.Helper()
	for _, v := range values {
		if v.Key == key {
			return v
		}
	}
	t.Fatalf("key %q not in effective values: %+v", key, values)
	return ConfigValue{}
}

func TestEffectiveConfigValues_PrecedenceAndDefaults(t *testing.T) {
	global := BaseConfig{
		Participant: "FromGlobal",
		LLM:         LLMConfig{Model: "global-model", Provider: "ollama"},
	}
	project := &PerRepoConfig{
		BaseConfig: BaseConfig{Participant: "FromProject"},
		GraphDir:   ".sdd/graph",
		Language:   "de",
	}
	local := &PerRepoConfig{
		BaseConfig: BaseConfig{LLM: LLMConfig{Model: "local-model"}},
	}

	values := EffectiveConfigValues(global, project, local)

	if v := findValue(t, values, "participant"); v.Value != "FromProject" || v.Source != ConfigSourceProject {
		t.Errorf("participant = %+v, want FromProject/project", v)
	}
	if v := findValue(t, values, "llm.model"); v.Value != "local-model" || v.Source != ConfigSourceLocal {
		t.Errorf("llm.model = %+v, want local-model/local", v)
	}
	if v := findValue(t, values, "llm.provider"); v.Value != "ollama" || v.Source != ConfigSourceGlobal {
		t.Errorf("llm.provider = %+v, want ollama/global", v)
	}
	if v := findValue(t, values, "language"); v.Value != "de" || v.Source != ConfigSourceProject {
		t.Errorf("language = %+v, want de/project", v)
	}
	// No layer sets sync.cooldown — the baked default surfaces.
	if v := findValue(t, values, "sync.cooldown"); v.Value != DefaultSyncCooldown || v.Source != ConfigSourceDefault {
		t.Errorf("sync.cooldown = %+v, want %s/default", v, DefaultSyncCooldown)
	}
	// llm.concurrency default surfaces even though llm block is partially set.
	if v := findValue(t, values, "llm.concurrency"); v.Source != ConfigSourceDefault {
		t.Errorf("llm.concurrency = %+v, want default source", v)
	}
}

func TestEffectiveConfigValues_APIKeysMaskedPerKey(t *testing.T) {
	global := BaseConfig{LLM: LLMConfig{APIKeys: map[string]string{"anthropic": "global-secret"}}}
	local := &PerRepoConfig{
		BaseConfig: BaseConfig{LLM: LLMConfig{APIKeys: map[string]string{"openai": "local-secret"}}},
	}
	values := EffectiveConfigValues(global, nil, local)

	anthropic := findValue(t, values, "llm.api_keys.anthropic")
	if !anthropic.Secret || anthropic.Value == "global-secret" {
		t.Errorf("anthropic key must be masked: %+v", anthropic)
	}
	if anthropic.Source != ConfigSourceGlobal {
		t.Errorf("anthropic source = %s, want global", anthropic.Source)
	}
	openai := findValue(t, values, "llm.api_keys.openai")
	if openai.Source != ConfigSourceLocal || !openai.Secret {
		t.Errorf("openai = %+v, want local/secret", openai)
	}
}

// Both api_keys and params are maps; only the one the field marks secret is
// masked. Inferring secrecy from the map shape hid a behaviour-affecting
// setting the operator needs to read back.
func TestEffectiveConfigValues_NonSecretMapRendersItsValues(t *testing.T) {
	global := BaseConfig{LLM: LLMConfig{
		APIKeys: map[string]string{"ollama": "global-secret"},
		Params:  map[string]string{"think": "high"},
	}}
	values := EffectiveConfigValues(global, nil, nil)

	think := findValue(t, values, "llm.params.think")
	if think.Secret {
		t.Errorf("params must not be marked secret: %+v", think)
	}
	if think.Value != "high" {
		t.Errorf("params value = %q, want high", think.Value)
	}
	key := findValue(t, values, "llm.api_keys.ollama")
	if !key.Secret || key.Value == "global-secret" {
		t.Errorf("api key must stay masked: %+v", key)
	}
}

func TestEffectiveConfigValues_SlicesRender(t *testing.T) {
	project := &PerRepoConfig{
		SupportedAgents: []AgentTarget{"claude", "codex"},
		Dependencies:    []string{"github.com/org/dep"},
	}
	values := EffectiveConfigValues(BaseConfig{}, project, nil)
	if v := findValue(t, values, "supported_agents"); v.Value != "claude, codex" {
		t.Errorf("supported_agents = %q", v.Value)
	}
	if v := findValue(t, values, "dependencies"); v.Value != "github.com/org/dep" {
		t.Errorf("dependencies = %q", v.Value)
	}
}

func TestParseConfigScalar_Types(t *testing.T) {
	if v, ok := ParseConfigScalar("8").(int64); !ok || v != 8 {
		t.Errorf("ParseConfigScalar(8) = %v", ParseConfigScalar("8"))
	}
	if v, ok := ParseConfigScalar("true").(bool); !ok || !v {
		t.Errorf("ParseConfigScalar(true) = %v", ParseConfigScalar("true"))
	}
	if v, ok := ParseConfigScalar("2m").(string); !ok || v != "2m" {
		t.Errorf("ParseConfigScalar(2m) = %v", ParseConfigScalar("2m"))
	}
	if v, ok := ParseConfigScalar("1.5").(float64); !ok || v != 1.5 {
		t.Errorf("ParseConfigScalar(1.5) = %v", ParseConfigScalar("1.5"))
	}
}
