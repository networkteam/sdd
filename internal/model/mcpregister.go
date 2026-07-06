package model

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
)

// MCPRegTarget is one config file `sdd init` manages to register the SDD MCP
// server for an agent, paired with the merge that brings it up to date.
type MCPRegTarget struct {
	// Path is the absolute path of the config file.
	Path string
	// Merge takes the file's current bytes (nil/empty when absent) and
	// returns the bytes to write, whether anything changed, and any parse
	// error. It adds the sdd registration when absent and leaves an existing
	// sdd entry untouched — the user owns it once it exists.
	Merge func(existing []byte) (merged []byte, changed bool, err error)
}

// MCPRegistrationTargets returns the project-scope config files that register
// the SDD workflow MCP server (and pre-allow its tools) for one agent, so
// engine mode works out of the box without manual setup (d-tac-wfl). Claude
// Code gets a project .mcp.json entry (with alwaysLoad so its tools skip the
// deferred-schema ceremony) plus an mcp__sdd__* allow rule in
// .claude/settings.json; Codex gets a .codex/config.toml entry that forwards
// SSH_AUTH_SOCK so `sdd serve` can reach the ssh-agent to sign commits
// (d-tac-ay1). User-scope registration is deliberately not handled here yet.
func MCPRegistrationTargets(target AgentTarget, repoRoot string) ([]MCPRegTarget, error) {
	if repoRoot == "" {
		return nil, fmt.Errorf("repo root is required for MCP registration")
	}
	switch target {
	case AgentClaude:
		return []MCPRegTarget{
			{Path: filepath.Join(repoRoot, ".mcp.json"), Merge: MergeClaudeMCPJSON},
			{Path: filepath.Join(repoRoot, ".claude", "settings.json"), Merge: MergeClaudeSettingsAllow},
		}, nil
	case AgentCodex:
		return []MCPRegTarget{
			{Path: filepath.Join(repoRoot, ".codex", "config.toml"), Merge: MergeCodexConfigTOML},
		}, nil
	default:
		return nil, fmt.Errorf("unsupported agent target: %q", target)
	}
}

// mcpAllowGlob is the tool-permission glob that pre-allows every SDD MCP tool.
const mcpAllowGlob = "mcp__sdd__*"

// MergeClaudeMCPJSON ensures the project .mcp.json declares the stdio `sdd
// serve` server with alwaysLoad set. An existing sdd server entry is left
// verbatim (the user may have customized it); other servers and unknown
// top-level keys round-trip. Keys re-order (Go marshals maps sorted) — a
// one-time cost of not carrying a comment-preserving JSON editor.
func MergeClaudeMCPJSON(existing []byte) ([]byte, bool, error) {
	root := map[string]any{}
	if len(bytes.TrimSpace(existing)) > 0 {
		if err := json.Unmarshal(existing, &root); err != nil {
			return nil, false, fmt.Errorf("parsing .mcp.json: %w", err)
		}
	}
	servers := map[string]any{}
	if raw, ok := root["mcpServers"]; ok {
		servers, ok = raw.(map[string]any)
		if !ok {
			return nil, false, fmt.Errorf(".mcp.json: mcpServers is not an object")
		}
	}
	if _, ok := servers["sdd"]; ok {
		return existing, false, nil
	}
	servers["sdd"] = map[string]any{
		"type":       "stdio",
		"command":    "sdd",
		"args":       []any{"serve"},
		"env":        map[string]any{},
		"alwaysLoad": true,
	}
	root["mcpServers"] = servers
	return marshalJSONFile(root)
}

// MergeClaudeSettingsAllow ensures .claude/settings.json pre-allows the SDD
// MCP tools via an mcp__sdd__* entry in permissions.allow. Existing allow
// entries keep their order (the glob appends); other settings round-trip.
func MergeClaudeSettingsAllow(existing []byte) ([]byte, bool, error) {
	root := map[string]any{}
	if len(bytes.TrimSpace(existing)) > 0 {
		if err := json.Unmarshal(existing, &root); err != nil {
			return nil, false, fmt.Errorf("parsing settings.json: %w", err)
		}
	}
	perms := map[string]any{}
	if raw, ok := root["permissions"]; ok {
		perms, ok = raw.(map[string]any)
		if !ok {
			return nil, false, fmt.Errorf("settings.json: permissions is not an object")
		}
	}
	var allow []any
	if raw, ok := perms["allow"]; ok {
		allow, ok = raw.([]any)
		if !ok {
			return nil, false, fmt.Errorf("settings.json: permissions.allow is not an array")
		}
	}
	for _, v := range allow {
		if s, _ := v.(string); s == mcpAllowGlob {
			return existing, false, nil
		}
	}
	allow = append(allow, mcpAllowGlob)
	perms["allow"] = allow
	root["permissions"] = perms
	return marshalJSONFile(root)
}

// marshalJSONFile renders a config map as 2-space-indented JSON with a
// trailing newline — matching the repo's committed .mcp.json / settings.json
// style. Always reports changed=true; callers gate on it only after deciding
// a write is warranted.
func marshalJSONFile(root map[string]any) ([]byte, bool, error) {
	out, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return nil, false, fmt.Errorf("encoding config: %w", err)
	}
	return append(out, '\n'), true, nil
}

// codexSDDBlock is the [mcp_servers.sdd] table for Codex's config.toml. cwd
// pins the server to the project; default_tools_approval_mode = "approve"
// pre-allows its tools; env_vars forwards SSH_AUTH_SOCK so `sdd serve` under
// Codex's filtered environment can reach the ssh-agent and sign commits
// without hanging (d-tac-ay1).
const codexSDDHeader = "[mcp_servers.sdd]"
const codexSDDBlock = codexSDDHeader + "\n" +
	"command = \"sdd\"\n" +
	"args = [\"serve\"]\n" +
	"cwd = \".\"\n" +
	"default_tools_approval_mode = \"approve\"\n" +
	"env_vars = [\"SSH_AUTH_SOCK\"]\n"

// MergeCodexConfigTOML ensures Codex's config.toml carries the [mcp_servers.sdd]
// table. TOML has no stdlib parser, so this stays text-only: an existing
// [mcp_servers.sdd] header means the user owns the block and it is left
// verbatim (comments and all); otherwise the canonical block is appended,
// preserving everything already in the file.
func MergeCodexConfigTOML(existing []byte) ([]byte, bool, error) {
	if len(bytes.TrimSpace(existing)) == 0 {
		return []byte(codexSDDBlock), true, nil
	}
	sc := bufio.NewScanner(bytes.NewReader(existing))
	for sc.Scan() {
		if strings.TrimSpace(sc.Text()) == codexSDDHeader {
			return existing, false, nil
		}
	}
	if err := sc.Err(); err != nil {
		return nil, false, fmt.Errorf("scanning config.toml: %w", err)
	}
	out := make([]byte, 0, len(existing)+len(codexSDDBlock)+2)
	out = append(out, existing...)
	if existing[len(existing)-1] != '\n' {
		out = append(out, '\n')
	}
	out = append(out, '\n')
	out = append(out, codexSDDBlock...)
	return out, true, nil
}
