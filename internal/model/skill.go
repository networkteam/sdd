package model

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// AgentTarget identifies which agent a skill bundle installs for. Claude is
// the only supported target for MVP; the type exists so the install flow can
// grow additional agents (Codex, etc.) without structural change.
type AgentTarget string

const (
	// AgentClaude installs into Claude Code's skill directories
	// (~/.claude/skills or <repo>/.claude/skills).
	AgentClaude AgentTarget = "claude"
)

// DefaultAgentTarget is the target used when nothing is specified. Only one
// agent is wired at MVP.
const DefaultAgentTarget = AgentClaude

// Scope selects where skills are installed for a given agent. User scope is
// shared across all projects for a given OS user; Project scope lives beside
// the repository.
type Scope string

const (
	// ScopeUser installs into the user-global agent directory (e.g.
	// ~/.claude/skills for Claude).
	ScopeUser Scope = "user"

	// ScopeProject installs into the repository-local agent directory (e.g.
	// <repo>/.claude/skills for Claude).
	ScopeProject Scope = "project"
)

// DefaultScope is the scope used when nothing is specified.
const DefaultScope = ScopeUser

// Stamp frontmatter keys injected into every installed skill file. Stripped
// before hashing so the stamps themselves don't pollute the content hash.
const (
	SkillStampVersion = "sdd-version"
	SkillStampHash    = "sdd-content-hash"
)

// SkillBundleEntry is one file in the embedded skill bundle — the content as
// shipped inside the sdd binary, without install-time stamps.
type SkillBundleEntry struct {
	// Skill is the top-level skill directory (e.g. "sdd", "sdd-catchup").
	Skill string

	// RelPath is the path inside the skill directory (e.g. "SKILL.md",
	// "references/framework-concepts.md"). Forward-slash separators.
	RelPath string

	// Content is the raw file bytes as embedded.
	Content []byte
}

// SkillBundle is the set of files embedded in the binary for a single agent
// target.
type SkillBundle struct {
	Target  AgentTarget
	Entries []SkillBundleEntry
}

// SkillFile is a parsed on-disk skill file, carrying the install stamps read
// from its frontmatter plus the raw content needed to re-hash.
type SkillFile struct {
	// AbsPath is the absolute path where the file lives on disk.
	AbsPath string

	// Content is the raw file bytes as read.
	Content []byte

	// StoredVersion is the value of the sdd-version frontmatter stamp, or
	// empty if absent.
	StoredVersion string

	// StoredHash is the value of the sdd-content-hash frontmatter stamp, or
	// empty if absent.
	StoredHash string
}

// SkillInstallStatus describes the install state of a single skill file
// relative to the embedded bundle.
type SkillInstallStatus string

const (
	// SkillStatusMissing means the file has no on-disk counterpart.
	SkillStatusMissing SkillInstallStatus = "missing"

	// SkillStatusCurrent means the on-disk file's stored hash matches the
	// embedded entry's hash and the user has not edited it.
	SkillStatusCurrent SkillInstallStatus = "current"

	// SkillStatusPristine means the on-disk file was produced by a previous
	// sdd init (user hasn't edited it) but its stored version differs from
	// the embedded bundle — safe to overwrite silently.
	SkillStatusPristine SkillInstallStatus = "pristine"

	// SkillStatusModified means the on-disk file's computed hash does not
	// match its stored hash — the user has edited it, overwrite requires
	// confirmation.
	SkillStatusModified SkillInstallStatus = "modified"
)

// ComputeSkillStatus classifies an installed file relative to its embedded
// counterpart. When installed is nil, the file is missing.
//
// An unstamped installed file (empty StoredHash) is treated as Pristine when
// its content byte-matches the embedded entry — first-run adoption of a
// bundled skill shouldn't reflexively prompt the user about files they
// haven't touched. Unstamped files whose content doesn't match are Modified.
func ComputeSkillStatus(embedded SkillBundleEntry, installed *SkillFile) SkillInstallStatus {
	if installed == nil {
		return SkillStatusMissing
	}

	installedComputed := ComputeSkillHash(installed.Content)
	embeddedHash := ComputeSkillHash(embedded.Content)

	if installed.StoredHash == "" {
		if installedComputed == embeddedHash {
			return SkillStatusPristine
		}
		return SkillStatusModified
	}

	if installedComputed != installed.StoredHash {
		return SkillStatusModified
	}
	if installed.StoredHash == embeddedHash {
		return SkillStatusCurrent
	}
	return SkillStatusPristine
}

// ComputeSkillHash returns the canonical content hash for a skill file. The
// hash is computed over the canonicalized frontmatter (with the sdd-version
// and sdd-content-hash keys stripped) concatenated with the body bytes. The
// result is a lowercase hex-encoded sha256 digest.
//
// Embedded entries carry no stamps, so their hash equals a freshly-written
// file's computed hash — this is the equality that lets a previously-installed
// pristine file match its embedded source across version bumps.
func ComputeSkillHash(fileContent []byte) string {
	fm, body := splitFrontmatter(fileContent)
	var canonFM []byte
	if fm != nil {
		stripped := stripStampKeys(fm)
		canonFM = CanonicalizeFrontmatter(stripped)
	}

	h := sha256.New()
	h.Write(canonFM)
	// Separator between canonical frontmatter and body. Plain newline works
	// because canonFM has no trailing newline (json.Marshal output).
	h.Write([]byte{'\n'})
	h.Write(body)
	return hex.EncodeToString(h.Sum(nil))
}

// CanonicalizeFrontmatter returns a deterministic byte representation of fm
// suitable for hashing. The encoding is JSON with sorted keys; json.Marshal
// on map[string]any sorts keys alphabetically at every depth, giving a stable
// ordering across YAML read/write cycles that don't preserve order.
//
// The output has no trailing newline. Callers embedding it in a larger hash
// stream must add their own separator.
func CanonicalizeFrontmatter(fm map[string]any) []byte {
	if len(fm) == 0 {
		return nil
	}
	normalized := normalizeForJSON(fm)
	data, err := json.Marshal(normalized)
	if err != nil {
		// json.Marshal on a normalized value is not expected to fail. Fall
		// back to an empty canonical form so a malformed structure hashes
		// deterministically rather than panicking.
		return nil
	}
	return data
}

// normalizeForJSON converts yaml.v3 decode output into types that json.Marshal
// can serialize deterministically. Specifically, it rewrites map[any]any
// (which older yaml libraries emit for nested objects) into map[string]any,
// which json.Marshal serializes with sorted keys.
func normalizeForJSON(v any) any {
	switch x := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, val := range x {
			out[k] = normalizeForJSON(val)
		}
		return out
	case map[any]any:
		out := make(map[string]any, len(x))
		for k, val := range x {
			out[fmt.Sprint(k)] = normalizeForJSON(val)
		}
		return out
	case []any:
		out := make([]any, len(x))
		for i, val := range x {
			out[i] = normalizeForJSON(val)
		}
		return out
	default:
		return x
	}
}

// stripStampKeys returns a copy of fm without the install-time stamps. Used
// to exclude sdd-version and sdd-content-hash from the hash input so the hash
// is stable under re-stamping.
func stripStampKeys(fm map[string]any) map[string]any {
	out := make(map[string]any, len(fm))
	for k, v := range fm {
		if k == SkillStampVersion || k == SkillStampHash {
			continue
		}
		out[k] = v
	}
	return out
}

// splitFrontmatter separates a YAML frontmatter block from the body of a
// skill file. Returns (nil, content) when no frontmatter is present.
//
// Frontmatter is delimited by a leading "---\n" line and a matching "---\n"
// or "---" (possibly EOF) terminator. Matches the convention used by existing
// .claude/skills/*/SKILL.md files.
func splitFrontmatter(content []byte) (map[string]any, []byte) {
	if !bytes.HasPrefix(content, []byte("---\n")) && !bytes.HasPrefix(content, []byte("---\r\n")) {
		return nil, content
	}

	// Skip the leading delimiter.
	rest := content[4:]
	if bytes.HasPrefix(content, []byte("---\r\n")) {
		rest = content[5:]
	}

	// Find the closing delimiter — a line containing only "---".
	lines := bytes.Split(rest, []byte("\n"))
	var fmLines [][]byte
	bodyStart := -1
	for i, line := range lines {
		trimmed := strings.TrimRight(string(line), "\r")
		if trimmed == "---" {
			bodyStart = i + 1
			break
		}
		fmLines = append(fmLines, line)
	}
	if bodyStart < 0 {
		// No closing delimiter — treat the whole thing as body to be safe.
		return nil, content
	}

	fmBytes := bytes.Join(fmLines, []byte("\n"))
	var fm map[string]any
	if err := yaml.Unmarshal(fmBytes, &fm); err != nil || fm == nil {
		return nil, content
	}

	var body []byte
	if bodyStart < len(lines) {
		body = bytes.Join(lines[bodyStart:], []byte("\n"))
	}
	return fm, body
}

// RenderSkillFile reassembles frontmatter and body into a skill file with the
// given install-time stamps injected. If the entry has no original
// frontmatter, a fresh block with just the stamps is written.
//
// The returned bytes are what gets hashed by ComputeSkillHash once the stamps
// are stripped — callers writing new installations should compute the hash
// from the embedded entry content (which has no stamps) to populate the
// sdd-content-hash stamp itself.
func RenderSkillFile(entry SkillBundleEntry, version, contentHash string) ([]byte, error) {
	fm, body := splitFrontmatter(entry.Content)
	if fm == nil {
		fm = map[string]any{}
	}
	fm[SkillStampVersion] = version
	fm[SkillStampHash] = contentHash

	yamlBytes, err := yaml.Marshal(fm)
	if err != nil {
		return nil, fmt.Errorf("marshalling skill frontmatter: %w", err)
	}

	var buf bytes.Buffer
	buf.WriteString("---\n")
	buf.Write(yamlBytes)
	buf.WriteString("---\n")
	buf.Write(body)
	return buf.Bytes(), nil
}

// SkillInstallDir returns the absolute directory where skills install for a
// given agent target and scope.
//
// Claude skills install at <home>/.claude/skills (user scope) or
// <repoRoot>/.claude/skills (project scope). The function errors if the
// required base path is empty for the chosen scope — callers should populate
// UserHome or RepoRoot before the query reaches the finder.
func SkillInstallDir(target AgentTarget, scope Scope, repoRoot, userHome string) (string, error) {
	switch target {
	case AgentClaude:
		switch scope {
		case ScopeUser:
			if userHome == "" {
				return "", fmt.Errorf("user home is required for user scope")
			}
			return filepath.Join(userHome, ".claude", "skills"), nil
		case ScopeProject:
			if repoRoot == "" {
				return "", fmt.Errorf("repo root is required for project scope")
			}
			return filepath.Join(repoRoot, ".claude", "skills"), nil
		default:
			return "", fmt.Errorf("unknown scope: %q", scope)
		}
	default:
		return "", fmt.Errorf("unsupported agent target: %q", target)
	}
}

// ParseSkillFile reads an on-disk skill file's raw bytes and extracts its
// install-time stamps.
func ParseSkillFile(absPath string, content []byte) *SkillFile {
	fm, _ := splitFrontmatter(content)
	sf := &SkillFile{AbsPath: absPath, Content: content}
	if fm == nil {
		return sf
	}
	if v, ok := fm[SkillStampVersion].(string); ok {
		sf.StoredVersion = v
	}
	if h, ok := fm[SkillStampHash].(string); ok {
		sf.StoredHash = h
	}
	return sf
}
