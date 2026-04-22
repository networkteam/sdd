package model

import (
	"crypto/rand"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type EntryType string

const (
	TypeSignal   EntryType = "signal"
	TypeDecision EntryType = "decision"
)

type Layer string

const (
	LayerStrategic   Layer = "strategic"
	LayerConceptual  Layer = "conceptual"
	LayerTactical    Layer = "tactical"
	LayerOperational Layer = "operational"
	LayerProcess     Layer = "process"
)

// TypeAbbrev maps full type names to abbreviations used in IDs.
var TypeAbbrev = map[EntryType]string{
	TypeSignal:   "s",
	TypeDecision: "d",
}

// TypeFromAbbrev maps abbreviations to full type names.
var TypeFromAbbrev = map[string]EntryType{
	"s": TypeSignal,
	"d": TypeDecision,
}

// LayerAbbrev maps full layer names to abbreviations used in IDs.
var LayerAbbrev = map[Layer]string{
	LayerStrategic:   "stg",
	LayerConceptual:  "cpt",
	LayerTactical:    "tac",
	LayerOperational: "ops",
	LayerProcess:     "prc",
}

// LayerFromAbbrev maps abbreviations to full layer names.
var LayerFromAbbrev = map[string]Layer{
	"stg": LayerStrategic,
	"cpt": LayerConceptual,
	"tac": LayerTactical,
	"ops": LayerOperational,
	"prc": LayerProcess,
}

// Kind is a sub-type classifier carried on signals and decisions. Its allowed
// values depend on the entry's Type:
//
//   - Signal kinds: gap (default), fact, question, insight, done
//   - Decision kinds: directive (default), activity, plan, contract, aspiration
//
// Empty Kind on a new entry is replaced by the type's default during capture.
type Kind string

const (
	// Signal kinds.
	KindGap      Kind = "gap"
	KindFact     Kind = "fact"
	KindQuestion Kind = "question"
	KindInsight  Kind = "insight"
	KindDone     Kind = "done"

	// Decision kinds.
	KindDirective  Kind = "directive"
	KindActivity   Kind = "activity"
	KindPlan       Kind = "plan"
	KindContract   Kind = "contract"
	KindAspiration Kind = "aspiration"
)

// signalKinds is the set of kinds valid on type: signal entries.
var signalKinds = map[Kind]bool{
	KindGap:      true,
	KindFact:     true,
	KindQuestion: true,
	KindInsight:  true,
	KindDone:     true,
}

// decisionKinds is the set of kinds valid on type: decision entries.
var decisionKinds = map[Kind]bool{
	KindDirective:  true,
	KindActivity:   true,
	KindPlan:       true,
	KindContract:   true,
	KindAspiration: true,
}

// IsValidKindForType reports whether k is an allowed kind for the given type.
// Empty kind is allowed at this layer — defaults are applied separately during
// entry construction (see DefaultKindForType).
func IsValidKindForType(t EntryType, k Kind) bool {
	if k == "" {
		return true
	}
	switch t {
	case TypeSignal:
		return signalKinds[k]
	case TypeDecision:
		return decisionKinds[k]
	default:
		return false
	}
}

// DefaultKindForType returns the kind applied when a new entry of type t is
// captured without an explicit --kind. Signals default to gap; decisions to
// directive. Other types have no default (empty).
func DefaultKindForType(t EntryType) Kind {
	switch t {
	case TypeSignal:
		return KindGap
	case TypeDecision:
		return KindDirective
	default:
		return ""
	}
}

// Warning represents a validation issue found on a graph entry.
type Warning struct {
	Field   string // "refs", "closes", "supersedes"
	Value   string // the offending ID or value
	Message string // human-readable description
}

type Entry struct {
	ID           string
	Type         EntryType
	Layer        Layer
	Kind         Kind // only meaningful for decisions; empty = directive (default)
	Refs         []string
	Supersedes   []string
	Closes       []string
	Participants []string
	Confidence   string
	Content      string
	Time         time.Time
	Preflight    string    // "skipped" or "error" annotation from pre-flight validation
	Attachments  []string  // filenames discovered from the co-located attachment directory
	Summary      string    // LLM-generated summary: this entry + direct relationships
	SummaryHash  string    // hex-encoded hash of the rendered summary prompt inputs
	Warnings     []Warning // validation issues found during graph construction
}

// IsContract returns true if this decision is a standing constraint.
func (e *Entry) IsContract() bool {
	return e.Kind == KindContract
}

// IsPlan returns true if this decision is an implementation plan.
func (e *Entry) IsPlan() bool {
	return e.Kind == KindPlan
}

// IsAspiration returns true if this decision is a perpetual direction.
func (e *Entry) IsAspiration() bool {
	return e.Kind == KindAspiration
}

// frontmatter is the YAML structure in the file header.
type frontmatter struct {
	Type         string   `yaml:"type"`
	Layer        string   `yaml:"layer"`
	Kind         string   `yaml:"kind,omitempty"`
	Refs         []string `yaml:"refs,omitempty"`
	Supersedes   []string `yaml:"supersedes,omitempty"`
	Closes       []string `yaml:"closes,omitempty"`
	Participants []string `yaml:"participants,omitempty"`
	Confidence   string   `yaml:"confidence,omitempty"`
	Preflight    string   `yaml:"preflight,omitempty"`
	Summary      string   `yaml:"summary,omitempty"`
	SummaryHash  string   `yaml:"summary_hash,omitempty"`
}

// ParseEntry parses a graph entry from its filename and file content.
func ParseEntry(filename, content string) (*Entry, error) {
	id := strings.TrimSuffix(filename, ".md")

	idParts, err := ParseID(id)
	if err != nil {
		return nil, fmt.Errorf("parsing ID %q: %w", id, err)
	}

	fm, body, err := parseFrontmatter(content)
	if err != nil {
		return nil, fmt.Errorf("parsing frontmatter in %q: %w", filename, err)
	}

	entryType, err := parseEntryType(fm.Type)
	if err != nil {
		return nil, fmt.Errorf("parsing frontmatter in %q: %w", filename, err)
	}

	layer, ok := LayerFromAbbrev[fm.Layer]
	if !ok {
		layer = Layer(fm.Layer)
	}

	return &Entry{
		ID:           id,
		Type:         entryType,
		Layer:        layer,
		Kind:         Kind(fm.Kind),
		Refs:         fm.Refs,
		Supersedes:   fm.Supersedes,
		Closes:       fm.Closes,
		Participants: fm.Participants,
		Confidence:   fm.Confidence,
		Preflight:    fm.Preflight,
		Summary:      fm.Summary,
		SummaryHash:  fm.SummaryHash,
		Content:      strings.TrimSpace(body),
		Time:         idParts.Time,
	}, nil
}

// IDParts holds the parsed components of a document ID.
type IDParts struct {
	Timestamp string
	Time      time.Time
	TypeCode  string // abbreviation: "s" or "d"
	LayerCode string // abbreviation: "stg", "cpt", "tac", "ops", "prc"
	Suffix    string
}

// ParseID parses a document ID into its components.
// ID format: {YYYYMMDD}-{HHmmss}-{type}-{layer}-{suffix}
func ParseID(id string) (IDParts, error) {
	dashes := []int{}
	for i, c := range id {
		if c == '-' {
			dashes = append(dashes, i)
		}
	}
	if len(dashes) < 4 {
		return IDParts{}, fmt.Errorf("invalid ID format: %q (need at least 4 dashes)", id)
	}

	timestamp := id[:dashes[1]]
	t, err := time.Parse("20060102-150405", timestamp)
	if err != nil {
		return IDParts{}, fmt.Errorf("parsing time from %q: %w", id, err)
	}

	return IDParts{
		Timestamp: timestamp,
		Time:      t,
		TypeCode:  id[dashes[1]+1 : dashes[2]],
		LayerCode: id[dashes[2]+1 : dashes[3]],
		Suffix:    id[dashes[3]+1:],
	}, nil
}

// IDToRelPath converts an entry ID to its relative file path in the hierarchical layout.
// ID format: YYYYMMDD-HHmmss-type-layer-suffix
// Path format: YYYY/MM/DD-HHmmss-type-layer-suffix.md
func IDToRelPath(id string) (string, error) {
	if len(id) < 8 {
		return "", fmt.Errorf("ID too short: %q", id)
	}
	yyyy := id[0:4]
	mm := id[4:6]
	shortName := id[6:] // DD-HHmmss-type-layer-suffix
	return filepath.Join(yyyy, mm, shortName+".md"), nil
}

// RelPathToID converts a relative path (YYYY/MM/DD-HHmmss-type-layer-suffix.md) to a full entry ID.
func RelPathToID(rel string) (string, error) {
	parts := strings.Split(filepath.ToSlash(rel), "/")
	if len(parts) != 3 {
		return "", fmt.Errorf("expected YYYY/MM/filename.md, got %q", rel)
	}
	yyyy := parts[0]
	mm := parts[1]
	filename := strings.TrimSuffix(parts[2], ".md")
	return yyyy + mm + filename, nil
}

// AttachDirRelPath returns the relative path to the attachment directory for an entry ID.
// This is the entry's file path without the .md extension.
func AttachDirRelPath(id string) (string, error) {
	rel, err := IDToRelPath(id)
	if err != nil {
		return "", err
	}
	return strings.TrimSuffix(rel, ".md"), nil
}

// ResolveAttachmentLinks replaces {{attachments}} placeholders in content with the
// actual relative directory path for markdown links.
func ResolveAttachmentLinks(content, id string) string {
	if len(id) < 8 {
		return content
	}
	// The short filename (without YYYYMM prefix and .md) serves as the directory name
	// relative to the entry file in the same directory.
	shortName := id[6:] // DD-HHmmss-type-layer-suffix
	return strings.ReplaceAll(content, "{{attachments}}", "./"+shortName)
}

// parseEntryType resolves a frontmatter type string to a canonical EntryType.
// Accepts both abbreviated ("s", "d") and full ("signal", "decision") forms.
// Unknown values return an error — the loader is the last line of defence
// against stale or malformed graph entries.
func parseEntryType(s string) (EntryType, error) {
	switch s {
	case "s", "signal":
		return TypeSignal, nil
	case "d", "decision":
		return TypeDecision, nil
	case "":
		return "", fmt.Errorf("missing type field")
	default:
		return "", fmt.Errorf("unknown type %q (expected signal or decision)", s)
	}
}

// parseFrontmatter splits content into YAML frontmatter and body.
func parseFrontmatter(content string) (*frontmatter, string, error) {
	content = strings.TrimSpace(content)
	if !strings.HasPrefix(content, "---") {
		return nil, content, fmt.Errorf("missing frontmatter delimiter")
	}

	rest := content[3:]
	idx := strings.Index(rest, "\n---")
	if idx < 0 {
		return nil, content, fmt.Errorf("missing closing frontmatter delimiter")
	}

	yamlContent := rest[:idx]
	body := rest[idx+4:] // skip \n---

	var fm frontmatter
	if err := yaml.Unmarshal([]byte(yamlContent), &fm); err != nil {
		return nil, "", fmt.Errorf("parsing YAML: %w", err)
	}

	return &fm, body, nil
}

// FormatFrontmatter creates the YAML frontmatter string for an entry.
func FormatFrontmatter(e *Entry) string {
	fm := frontmatter{
		Type:         string(e.Type),
		Layer:        string(e.Layer),
		Kind:         string(e.Kind),
		Refs:         e.Refs,
		Supersedes:   e.Supersedes,
		Closes:       e.Closes,
		Participants: e.Participants,
		Confidence:   e.Confidence,
		Preflight:    e.Preflight,
		Summary:      e.Summary,
		SummaryHash:  e.SummaryHash,
	}

	data, _ := yaml.Marshal(&fm)
	return "---\n" + string(data) + "---\n"
}

// GenerateID creates a new document ID with the current timestamp and a random suffix.
func GenerateID(typ EntryType, layer Layer, suffix string) string {
	return GenerateIDAt(typ, layer, suffix, time.Now())
}

// RewriteID returns the id with its type abbreviation replaced by newType's
// abbreviation. Timestamp, layer, and suffix are preserved. Used by the
// sdd rewrite command for mechanical type changes.
func RewriteID(id string, newType EntryType) (string, error) {
	parts, err := ParseID(id)
	if err != nil {
		return "", err
	}
	newAbbrev, ok := TypeAbbrev[newType]
	if !ok {
		return "", fmt.Errorf("unknown type: %s", newType)
	}
	return fmt.Sprintf("%s-%s-%s-%s", parts.Timestamp, newAbbrev, parts.LayerCode, parts.Suffix), nil
}

// GenerateIDAt creates a new document ID with the given timestamp and a random suffix.
// Accepts the time explicitly so callers can inject a clock for testability.
func GenerateIDAt(typ EntryType, layer Layer, suffix string, t time.Time) string {
	ta := TypeAbbrev[typ]
	la := LayerAbbrev[layer]
	return fmt.Sprintf("%s-%s-%s-%s", t.Format("20060102-150405"), ta, la, suffix)
}

// RandomSuffix returns an n-character lowercase alphanumeric string suitable
// for use as the trailing random portion of a document ID.
func RandomSuffix(n int) (string, error) {
	const charset = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	for i := range b {
		b[i] = charset[b[i]%byte(len(charset))]
	}
	return string(b), nil
}

// TypeLabel returns a display label for the entry type.
func (e *Entry) TypeLabel() string {
	switch e.Type {
	case TypeSignal:
		return "signal"
	case TypeDecision:
		return "decision"
	default:
		return string(e.Type)
	}
}

// LayerLabel returns a display label for the layer.
func (e *Entry) LayerLabel() string {
	return string(e.Layer)
}

// ShortContent returns content truncated to maxLen, preferring sentence boundaries.
// Accumulates complete sentences up to the limit. If no sentence fits, accumulates words.
func (e *Entry) ShortContent(maxLen int) string {
	line := e.Content
	if idx := strings.Index(line, "\n"); idx >= 0 {
		line = line[:idx]
	}
	if len(line) <= maxLen {
		return line
	}

	// Try to accumulate sentences
	sentences := splitSentences(line)
	if len(sentences) > 1 {
		result := sentences[0]
		included := 1
		for _, s := range sentences[1:] {
			candidate := result + " " + s
			if len(candidate) > maxLen {
				break
			}
			result = candidate
			included++
		}
		if included < len(sentences) && len(result)+4 <= maxLen {
			result += " ..."
		}
		if len(result) <= maxLen {
			return result
		}
	}

	// Fall back to accumulating words
	words := strings.Fields(line)
	result := words[0]
	for _, w := range words[1:] {
		candidate := result + " " + w
		if len(candidate)+4 > maxLen { // +4 for " ..."
			break
		}
		result = candidate
	}
	return result + " ..."
}

// splitSentences splits text on sentence-ending punctuation followed by a space.
func splitSentences(text string) []string {
	var sentences []string
	start := 0
	for i := 0; i < len(text)-1; i++ {
		if (text[i] == '.' || text[i] == '!' || text[i] == '?') && text[i+1] == ' ' {
			sentences = append(sentences, text[start:i+1])
			start = i + 2
		}
	}
	if start < len(text) {
		sentences = append(sentences, text[start:])
	}
	return sentences
}
