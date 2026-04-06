package sdd

import (
	"fmt"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type EntryType string

const (
	TypeSignal   EntryType = "signal"
	TypeDecision EntryType = "decision"
	TypeAction   EntryType = "action"
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
	TypeAction:   "a",
}

// TypeFromAbbrev maps abbreviations to full type names.
var TypeFromAbbrev = map[string]EntryType{
	"s": TypeSignal,
	"d": TypeDecision,
	"a": TypeAction,
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

type Entry struct {
	ID           string
	Type         EntryType
	Layer        Layer
	Refs         []string
	Participants []string
	Confidence   string
	Content      string
	Time         time.Time
}

// frontmatter is the YAML structure in the file header.
type frontmatter struct {
	Type         string   `yaml:"type"`
	Layer        string   `yaml:"layer"`
	Refs         []string `yaml:"refs,omitempty"`
	Participants []string `yaml:"participants,omitempty"`
	Confidence   string   `yaml:"confidence,omitempty"`
}

// ParseEntry parses a graph entry from its filename and file content.
func ParseEntry(filename, content string) (*Entry, error) {
	id := strings.TrimSuffix(filename, ".md")

	t, err := ParseIDTime(id)
	if err != nil {
		return nil, fmt.Errorf("parsing time from %q: %w", id, err)
	}

	fm, body, err := parseFrontmatter(content)
	if err != nil {
		return nil, fmt.Errorf("parsing frontmatter in %q: %w", filename, err)
	}

	entryType, ok := TypeFromAbbrev[fm.Type]
	if !ok {
		entryType = EntryType(fm.Type)
	}

	layer, ok := LayerFromAbbrev[fm.Layer]
	if !ok {
		layer = Layer(fm.Layer)
	}

	return &Entry{
		ID:           id,
		Type:         entryType,
		Layer:        layer,
		Refs:         fm.Refs,
		Participants: fm.Participants,
		Confidence:   fm.Confidence,
		Content:      strings.TrimSpace(body),
		Time:         t,
	}, nil
}

// ParseIDTime extracts the timestamp from a document ID.
// ID format: {YYYYMMDD}-{HHmmss}-{type}-{layer}-{suffix}
func ParseIDTime(id string) (time.Time, error) {
	parts := strings.SplitN(id, "-", 3)
	if len(parts) < 2 {
		return time.Time{}, fmt.Errorf("invalid ID format: %q", id)
	}
	return time.Parse("20060102-150405", parts[0]+"-"+parts[1])
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
		Refs:         e.Refs,
		Participants: e.Participants,
		Confidence:   e.Confidence,
	}

	data, _ := yaml.Marshal(&fm)
	return "---\n" + string(data) + "---\n"
}

// GenerateID creates a new document ID with the current timestamp and a random suffix.
func GenerateID(typ EntryType, layer Layer, suffix string) string {
	now := time.Now()
	ta := TypeAbbrev[typ]
	la := LayerAbbrev[layer]
	return fmt.Sprintf("%s-%s-%s-%s", now.Format("20060102-150405"), ta, la, suffix)
}

// TypeLabel returns a display label for the entry type.
func (e *Entry) TypeLabel() string {
	switch e.Type {
	case TypeSignal:
		return "signal"
	case TypeDecision:
		return "decision"
	case TypeAction:
		return "action"
	default:
		return string(e.Type)
	}
}

// LayerLabel returns a display label for the layer.
func (e *Entry) LayerLabel() string {
	return string(e.Layer)
}

// ShortContent returns the first line of content, truncated to maxLen.
func (e *Entry) ShortContent(maxLen int) string {
	line := e.Content
	if idx := strings.Index(line, "\n"); idx >= 0 {
		line = line[:idx]
	}
	if len(line) > maxLen {
		line = line[:maxLen-3] + "..."
	}
	return line
}
