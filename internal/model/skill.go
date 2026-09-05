package model

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"golang.org/x/mod/semver"
	"gopkg.in/yaml.v3"
)

// AgentTarget identifies which agent a skill bundle renders and installs for.
// Each target is a render profile over the one neutral template tree — the
// value drives {{ if eq .Agent "..." }} conditionals, the install directory,
// and (eventually) the frontmatter shape.
type AgentTarget string

const (
	// AgentClaude renders into Claude Code's skill directories
	// (~/.claude/skills or <repo>/.claude/skills).
	AgentClaude AgentTarget = "claude"

	// AgentCodex renders into the Agent Skills standard directories
	// (~/.agents/skills or <repo>/.agents/skills), consumed by Codex and
	// other agents reading the open Agent Skills format.
	AgentCodex AgentTarget = "codex"
)

// DefaultAgentTarget is the target used when nothing is specified.
const DefaultAgentTarget = AgentClaude

// KnownAgentTargets lists every target sdd can render, in the order the
// `sdd init` multi-select presents them. Adding a target here surfaces it in
// the selector and in supported_agents validation.
var KnownAgentTargets = []AgentTarget{AgentClaude, AgentCodex}

// DefaultSupportedAgents is the supported_agents value written on a fresh init
// when the operator makes no other selection — Claude alone, matching the
// pre-multi-agent default.
var DefaultSupportedAgents = []AgentTarget{AgentClaude}

// ParseAgentTarget validates s against KnownAgentTargets, returning the typed
// value or an error naming the valid set.
func ParseAgentTarget(s string) (AgentTarget, error) {
	for _, t := range KnownAgentTargets {
		if string(t) == s {
			return t, nil
		}
	}
	names := make([]string, len(KnownAgentTargets))
	for i, t := range KnownAgentTargets {
		names[i] = string(t)
	}
	return "", fmt.Errorf("unknown agent target %q (known: %s)", s, strings.Join(names, ", "))
}

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
	// Skill is the top-level skill directory (e.g. "sdd", "sdd-explore").
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
//
// Stamps are stripped wherever they live — top level (Claude) or nested under
// metadata: (Codex). A metadata map emptied by the strip is dropped entirely
// so a stamped Codex file hashes identically to its unstamped embedded source
// (which carries no metadata key).
func stripStampKeys(fm map[string]any) map[string]any {
	out := make(map[string]any, len(fm))
	for k, v := range fm {
		if k == SkillStampVersion || k == SkillStampHash {
			continue
		}
		out[k] = v
	}
	if meta, ok := asStringMap(out["metadata"]); ok {
		stripped := make(map[string]any, len(meta))
		for k, v := range meta {
			if k == SkillStampVersion || k == SkillStampHash {
				continue
			}
			stripped[k] = v
		}
		if len(stripped) == 0 {
			delete(out, "metadata")
		} else {
			out["metadata"] = stripped
		}
	}
	return out
}

// setStamps injects the install-time version and content-hash stamps into fm.
// Claude keeps them at the top level; Codex nests them under metadata: so the
// rendered SKILL.md conforms to the Agent Skills standard. Stamps merge into
// any metadata the template already authored rather than replacing it.
func setStamps(fm map[string]any, target AgentTarget, version, contentHash string) {
	if target == AgentCodex {
		meta, ok := asStringMap(fm["metadata"])
		if !ok {
			meta = map[string]any{}
		}
		meta[SkillStampVersion] = version
		meta[SkillStampHash] = contentHash
		fm["metadata"] = meta
		return
	}
	fm[SkillStampVersion] = version
	fm[SkillStampHash] = contentHash
}

// readStamps extracts the install-time stamps from fm, accepting either the
// top-level placement (Claude) or the metadata-nested placement (Codex).
func readStamps(fm map[string]any) (version, hash string) {
	if v, ok := fm[SkillStampVersion].(string); ok {
		version = v
	}
	if h, ok := fm[SkillStampHash].(string); ok {
		hash = h
	}
	if version != "" && hash != "" {
		return version, hash
	}
	if meta, ok := asStringMap(fm["metadata"]); ok {
		if version == "" {
			if v, ok := meta[SkillStampVersion].(string); ok {
				version = v
			}
		}
		if hash == "" {
			if h, ok := meta[SkillStampHash].(string); ok {
				hash = h
			}
		}
	}
	return version, hash
}

// asStringMap coerces a frontmatter value into a map[string]any, tolerating the
// map[any]any that some YAML decode paths produce for nested mappings.
func asStringMap(v any) (map[string]any, bool) {
	switch m := v.(type) {
	case map[string]any:
		return m, true
	case map[any]any:
		out := make(map[string]any, len(m))
		for k, val := range m {
			out[fmt.Sprint(k)] = val
		}
		return out, true
	default:
		return nil, false
	}
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
// Stamp placement is target-aware: Claude keeps them at the top level (its
// historical shape), while Codex nests them under metadata: so the rendered
// SKILL.md conforms to the Agent Skills standard, which rejects unknown
// top-level keys. The read (ParseSkillFile) and hash (stripStampKeys) paths
// accept either placement, so they stay target-agnostic.
//
// The returned bytes are what gets hashed by ComputeSkillHash once the stamps
// are stripped — callers writing new installations should compute the hash
// from the embedded entry content (which has no stamps) to populate the
// sdd-content-hash stamp itself.
func RenderSkillFile(entry SkillBundleEntry, target AgentTarget, version, contentHash string) ([]byte, error) {
	fm, body := splitFrontmatter(entry.Content)
	if fm == nil {
		fm = map[string]any{}
	}
	setStamps(fm, target, version, contentHash)

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

// SkillFileBody returns the body of a skill file (frontmatter stripped). Bundle
// sources carry no install stamps, so this is usually the whole content; the
// strip keeps it correct if a fragment ever grows frontmatter.
func SkillFileBody(content []byte) []byte {
	_, body := splitFrontmatter(content)
	return body
}

// SkillInstallDir returns the absolute directory where skills install for a
// given agent target and scope.
//
// The per-target subpath (".claude/skills" for Claude, ".agents/skills" for
// Codex) is joined under <home> for user scope or <repoRoot> for project
// scope. The function errors if the required base path is empty for the chosen
// scope — callers should populate UserHome or RepoRoot before the query
// reaches the finder.
func SkillInstallDir(target AgentTarget, scope Scope, repoRoot, userHome string) (string, error) {
	var sub []string
	switch target {
	case AgentClaude:
		sub = []string{".claude", "skills"}
	case AgentCodex:
		sub = []string{".agents", "skills"}
	default:
		return "", fmt.Errorf("unsupported agent target: %q", target)
	}

	switch scope {
	case ScopeUser:
		if userHome == "" {
			return "", fmt.Errorf("user home is required for user scope")
		}
		return filepath.Join(append([]string{userHome}, sub...)...), nil
	case ScopeProject:
		if repoRoot == "" {
			return "", fmt.Errorf("repo root is required for project scope")
		}
		return filepath.Join(append([]string{repoRoot}, sub...)...), nil
	default:
		return "", fmt.Errorf("unknown scope: %q", scope)
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
	sf.StoredVersion, sf.StoredHash = readStamps(fm)
	return sf
}

// SkillOrphanClass classifies an installed file the embedded bundle no longer
// carries — the state left behind when a bundle source is removed by a
// rename, a split, or a retirement.
type SkillOrphanClass string

const (
	// SkillOrphanForeign means the file carries no sdd install stamp, so sdd
	// never wrote it: a skill of the user's own sharing the install
	// directory. Never removed, never reported.
	SkillOrphanForeign SkillOrphanClass = "foreign"

	// SkillOrphanUnmodified means sdd wrote the file and its content still
	// matches the stamp from that install — safe to remove.
	SkillOrphanUnmodified SkillOrphanClass = "unmodified"

	// SkillOrphanModified means sdd wrote the file and it has been edited
	// since — preserved, and named so the user can resolve it.
	SkillOrphanModified SkillOrphanClass = "modified"
)

// ClassifySkillOrphan decides what may be done with an installed file that has
// no bundle counterpart. The stamp is the ownership marker: without one, the
// file is not sdd's to touch.
func ClassifySkillOrphan(installed *SkillFile) SkillOrphanClass {
	if installed == nil || installed.StoredHash == "" {
		return SkillOrphanForeign
	}
	if ComputeSkillHash(installed.Content) == installed.StoredHash {
		return SkillOrphanUnmodified
	}
	return SkillOrphanModified
}

// SkillStampIsAhead reports whether an installed file's version stamp names a
// release later than the running binary — the downgrade case, where an older
// sdd would otherwise prune files a newer one installed simply because its own
// bundle does not carry them. Dev builds on either side never trigger it, in
// keeping with how they bypass the other version gates.
func SkillStampIsAhead(stampVersion, binaryVersion string) bool {
	if IsDevVersion(stampVersion) || IsDevVersion(binaryVersion) {
		return false
	}
	return semver.Compare(normalizeSemver(stampVersion), normalizeSemver(binaryVersion)) > 0
}
