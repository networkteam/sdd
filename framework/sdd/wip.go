package sdd

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// WIPMarker represents an in-flight work marker.
type WIPMarker struct {
	ID          string // filename without .md: {YYYYMMDD}-{HHmmss}-{participant}
	Entry       string // graph entry ID being worked on
	Participant string
	Exclusive   bool
	Content     string // free-text description
	Time        time.Time
}

// wipFrontmatter is the YAML structure in a WIP marker file.
type wipFrontmatter struct {
	Entry       string `yaml:"entry"`
	Participant string `yaml:"participant"`
	Exclusive   bool   `yaml:"exclusive,omitempty"`
}

// GenerateWIPMarkerID creates a marker ID from the current timestamp and participant name.
func GenerateWIPMarkerID(participant string) string {
	now := time.Now()
	// Lowercase and replace spaces with hyphens for filesystem safety
	safe := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(participant), " ", "-"))
	return fmt.Sprintf("%s-%s", now.Format("20060102-150405"), safe)
}

// WIPMarkerPath returns the path to a marker file relative to the graph directory.
func WIPMarkerPath(markerID string) string {
	return filepath.Join("wip", markerID+".md")
}

// ParseWIPMarker parses a WIP marker from its filename and file content.
func ParseWIPMarker(filename, content string) (*WIPMarker, error) {
	id := strings.TrimSuffix(filename, ".md")

	// Parse timestamp from the ID (first 15 chars: YYYYMMDD-HHmmss)
	if len(id) < 16 { // at least timestamp + dash + one char participant
		return nil, fmt.Errorf("marker ID too short: %q", id)
	}
	t, err := time.Parse("20060102-150405", id[:15])
	if err != nil {
		return nil, fmt.Errorf("parsing time from marker %q: %w", id, err)
	}

	fm, body, err := parseWIPFrontmatter(content)
	if err != nil {
		return nil, fmt.Errorf("parsing frontmatter in %q: %w", filename, err)
	}

	return &WIPMarker{
		ID:          id,
		Entry:       fm.Entry,
		Participant: fm.Participant,
		Exclusive:   fm.Exclusive,
		Content:     strings.TrimSpace(body),
		Time:        t,
	}, nil
}

// FormatWIPMarker creates the file content for a WIP marker.
func FormatWIPMarker(m *WIPMarker) string {
	fm := wipFrontmatter{
		Entry:       m.Entry,
		Participant: m.Participant,
		Exclusive:   m.Exclusive,
	}

	data, _ := yaml.Marshal(&fm)
	content := "---\n" + string(data) + "---\n"
	if m.Content != "" {
		content += "\n" + m.Content + "\n"
	}
	return content
}

// LoadWIPMarkers reads all marker files from the wip/ subdirectory of graphDir.
func LoadWIPMarkers(graphDir string) ([]*WIPMarker, error) {
	wipDir := filepath.Join(graphDir, "wip")

	entries, err := os.ReadDir(wipDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // no wip directory = no markers
		}
		return nil, fmt.Errorf("reading wip directory: %w", err)
	}

	var markers []*WIPMarker
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}

		data, err := os.ReadFile(filepath.Join(wipDir, entry.Name()))
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", entry.Name(), err)
		}

		marker, err := ParseWIPMarker(entry.Name(), string(data))
		if err != nil {
			return nil, fmt.Errorf("parsing %s: %w", entry.Name(), err)
		}

		markers = append(markers, marker)
	}

	sort.Slice(markers, func(i, j int) bool {
		return markers[i].Time.Before(markers[j].Time)
	})

	return markers, nil
}

// ShortContent returns the first line of the marker content, truncated to maxLen.
func (m *WIPMarker) ShortContent(maxLen int) string {
	line := m.Content
	if idx := strings.Index(line, "\n"); idx >= 0 {
		line = line[:idx]
	}
	if len(line) <= maxLen {
		return line
	}
	if maxLen > 4 {
		return line[:maxLen-4] + " ..."
	}
	return line[:maxLen]
}

// parseWIPFrontmatter splits content into YAML frontmatter and body.
func parseWIPFrontmatter(content string) (*wipFrontmatter, string, error) {
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

	var fm wipFrontmatter
	if err := yaml.Unmarshal([]byte(yamlContent), &fm); err != nil {
		return nil, "", fmt.Errorf("parsing YAML: %w", err)
	}

	// Validate required fields
	if fm.Entry == "" {
		return nil, "", fmt.Errorf("missing required field: entry")
	}
	if fm.Participant == "" {
		return nil, "", fmt.Errorf("missing required field: participant")
	}

	// Validate that entry looks like a valid ID
	if _, err := ParseID(fm.Entry); err != nil {
		return nil, "", fmt.Errorf("invalid entry ID %q: %w", fm.Entry, err)
	}

	return &fm, body, nil
}

// MarkersForEntry returns all markers that reference the given graph entry ID.
func MarkersForEntry(markers []*WIPMarker, entryID string) []*WIPMarker {
	var result []*WIPMarker
	for _, m := range markers {
		if m.Entry == entryID {
			result = append(result, m)
		}
	}
	return result
}

// HasExclusiveMarker returns true if any marker for the given entry is exclusive.
func HasExclusiveMarker(markers []*WIPMarker, entryID string) (*WIPMarker, bool) {
	for _, m := range markers {
		if m.Entry == entryID && m.Exclusive {
			return m, true
		}
	}
	return nil, false
}

// WIPDir returns the path to the wip/ directory within a graph directory.
func WIPDir(graphDir string) string {
	return filepath.Join(graphDir, "wip")
}

// IsWIPDir returns true if the given directory entry represents the wip/ directory.
func IsWIPDir(d fs.DirEntry) bool {
	return d.IsDir() && d.Name() == "wip"
}
