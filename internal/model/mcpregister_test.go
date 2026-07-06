package model_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/networkteam/sdd/internal/model"
)

func TestMergeClaudeMCPJSON_FreshWritesSDDServerWithAlwaysLoad(t *testing.T) {
	out, changed, err := model.MergeClaudeMCPJSON(nil)
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatal("fresh .mcp.json should report changed")
	}
	var root struct {
		MCPServers map[string]struct {
			Type       string   `json:"type"`
			Command    string   `json:"command"`
			Args       []string `json:"args"`
			AlwaysLoad bool     `json:"alwaysLoad"`
		} `json:"mcpServers"`
	}
	if err := json.Unmarshal(out, &root); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, out)
	}
	sdd, ok := root.MCPServers["sdd"]
	if !ok {
		t.Fatalf("mcpServers.sdd missing:\n%s", out)
	}
	if sdd.Type != "stdio" || sdd.Command != "sdd" || len(sdd.Args) != 1 || sdd.Args[0] != "serve" {
		t.Errorf("unexpected sdd server shape: %+v", sdd)
	}
	if !sdd.AlwaysLoad {
		t.Error("sdd server should carry alwaysLoad:true to skip deferred-schema loading")
	}
}

func TestMergeClaudeMCPJSON_AddsToExistingPreservingOtherServers(t *testing.T) {
	existing := []byte(`{"mcpServers":{"other":{"command":"otherbin","args":["run"]}}}`)
	out, changed, err := model.MergeClaudeMCPJSON(existing)
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatal("adding sdd to a file without it should report changed")
	}
	var root struct {
		MCPServers map[string]json.RawMessage `json:"mcpServers"`
	}
	if err := json.Unmarshal(out, &root); err != nil {
		t.Fatal(err)
	}
	if _, ok := root.MCPServers["other"]; !ok {
		t.Errorf("existing 'other' server was dropped:\n%s", out)
	}
	if _, ok := root.MCPServers["sdd"]; !ok {
		t.Errorf("sdd server not added:\n%s", out)
	}
}

func TestMergeClaudeMCPJSON_ExistingSDDLeftUntouched(t *testing.T) {
	existing := []byte(`{"mcpServers":{"sdd":{"command":"custom-sdd","args":["serve","--flag"]}}}`)
	out, changed, err := model.MergeClaudeMCPJSON(existing)
	if err != nil {
		t.Fatal(err)
	}
	if changed {
		t.Fatal("an existing sdd entry must be left untouched (user owns it)")
	}
	if string(out) != string(existing) {
		t.Errorf("output should be byte-identical to input, got:\n%s", out)
	}
}

func TestMergeClaudeMCPJSON_MalformedServersErrors(t *testing.T) {
	if _, _, err := model.MergeClaudeMCPJSON([]byte(`{"mcpServers":"nope"}`)); err == nil {
		t.Fatal("expected an error when mcpServers is not an object")
	}
}

func TestMergeClaudeSettingsAllow_FreshAndAppendAndNoop(t *testing.T) {
	// Fresh file.
	out, changed, err := model.MergeClaudeSettingsAllow(nil)
	if err != nil || !changed {
		t.Fatalf("fresh settings should change: changed=%v err=%v", changed, err)
	}
	if !strings.Contains(string(out), "mcp__sdd__*") {
		t.Errorf("fresh settings missing the allow glob:\n%s", out)
	}

	// Append to existing, preserving other keys and existing allow entries.
	existing := []byte(`{"permissions":{"allow":["Bash(sdd *)"]},"worktree":{"baseRef":"head"}}`)
	out, changed, err = model.MergeClaudeSettingsAllow(existing)
	if err != nil || !changed {
		t.Fatalf("append should change: changed=%v err=%v", changed, err)
	}
	var root struct {
		Permissions struct {
			Allow []string `json:"allow"`
		} `json:"permissions"`
		Worktree map[string]any `json:"worktree"`
	}
	if err := json.Unmarshal(out, &root); err != nil {
		t.Fatal(err)
	}
	if root.Worktree["baseRef"] != "head" {
		t.Errorf("worktree key not preserved:\n%s", out)
	}
	if len(root.Permissions.Allow) != 2 || root.Permissions.Allow[0] != "Bash(sdd *)" || root.Permissions.Allow[1] != "mcp__sdd__*" {
		t.Errorf("allow list should append the glob after existing entries, got %v", root.Permissions.Allow)
	}

	// Already present → no change.
	_, changed, err = model.MergeClaudeSettingsAllow(out)
	if err != nil {
		t.Fatal(err)
	}
	if changed {
		t.Error("a settings file already carrying the glob must not change")
	}
}

func TestMergeCodexConfigTOML_FreshForwardsSSHAuthSock(t *testing.T) {
	out, changed, err := model.MergeCodexConfigTOML(nil)
	if err != nil || !changed {
		t.Fatalf("fresh config.toml should change: changed=%v err=%v", changed, err)
	}
	s := string(out)
	for _, want := range []string{"[mcp_servers.sdd]", `command = "sdd"`, `args = ["serve"]`, `env_vars = ["SSH_AUTH_SOCK"]`} {
		if !strings.Contains(s, want) {
			t.Errorf("fresh config.toml missing %q:\n%s", want, s)
		}
	}
}

func TestMergeCodexConfigTOML_AppendsPreservingExisting(t *testing.T) {
	existing := []byte("[mcp_servers.other]\ncommand = \"x\"\n")
	out, changed, err := model.MergeCodexConfigTOML(existing)
	if err != nil || !changed {
		t.Fatalf("append should change: changed=%v err=%v", changed, err)
	}
	s := string(out)
	if !strings.Contains(s, "[mcp_servers.other]") {
		t.Errorf("existing table dropped:\n%s", s)
	}
	if !strings.Contains(s, "[mcp_servers.sdd]") {
		t.Errorf("sdd table not appended:\n%s", s)
	}
	// A blank line must separate the appended block from prior content.
	if !strings.Contains(s, "command = \"x\"\n\n[mcp_servers.sdd]") {
		t.Errorf("appended block not separated by a blank line:\n%s", s)
	}
}

func TestMergeCodexConfigTOML_ExistingHeaderLeftUntouched(t *testing.T) {
	existing := []byte("# my config\n[mcp_servers.sdd]\ncommand = \"custom\"\n")
	out, changed, err := model.MergeCodexConfigTOML(existing)
	if err != nil {
		t.Fatal(err)
	}
	if changed {
		t.Fatal("an existing [mcp_servers.sdd] table must be left untouched")
	}
	if string(out) != string(existing) {
		t.Errorf("output should be byte-identical to input, got:\n%s", out)
	}
}
