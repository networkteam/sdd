package model

import (
	"crypto/sha256"
	"fmt"
	"io/fs"
	"regexp"
	"slices"
	"strings"
)

var fullEntryIDShape = regexp.MustCompile(`^[0-9]{8}-[0-9]{6}-[a-z]-[a-z]{3}-[a-z0-9]+$`)

func ValidateEntryID(id string) error {
	parts, err := ParseID(id)
	if err != nil || !fullEntryIDShape.MatchString(id) {
		return fmt.Errorf("invalid full entry ID %q", id)
	}
	if _, ok := TypeFromAbbrev[parts.TypeCode]; !ok {
		return fmt.Errorf("invalid entry type in %q", id)
	}
	if _, ok := LayerFromAbbrev[parts.LayerCode]; !ok {
		return fmt.Errorf("invalid entry layer in %q", id)
	}
	return nil
}

// NormalizeEntrySelection preserves nil as whole-graph scope.
func NormalizeEntrySelection(ids []string) ([]string, string, error) {
	if ids == nil {
		return nil, "", nil
	}
	if len(ids) == 0 {
		return nil, "", fmt.Errorf("entry selection must not be empty")
	}
	normalized := slices.Clone(ids)
	for _, id := range normalized {
		if err := ValidateEntryID(id); err != nil {
			return nil, "", err
		}
	}
	slices.Sort(normalized)
	normalized = slices.Compact(normalized)
	digest := sha256.Sum256([]byte(strings.Join(normalized, "\n")))
	return normalized, fmt.Sprintf("sha256:%x", digest), nil
}

// EntryIDForArtifactPath resolves an entry document or its attachment owner.
// Non-entry artifacts have no owner; malformed graph paths fail explicitly.
func EntryIDForArtifactPath(logicalPath string) (string, error) {
	if !fs.ValidPath(logicalPath) || strings.Contains(logicalPath, `\`) {
		return "", fmt.Errorf("invalid artifact path %q", logicalPath)
	}
	parts := strings.Split(logicalPath, "/")
	if len(parts[0]) != 4 || strings.Trim(parts[0], "0123456789") != "" {
		return "", nil
	}
	if len(parts) < 3 {
		return "", fmt.Errorf("invalid entry artifact path %q", logicalPath)
	}
	name := parts[2]
	if len(parts) == 3 {
		if !strings.HasSuffix(name, ".md") {
			return "", fmt.Errorf("invalid entry document path %q", logicalPath)
		}
		name = strings.TrimSuffix(name, ".md")
	}
	id := parts[0] + parts[1] + name
	if err := ValidateEntryID(id); err != nil {
		return "", err
	}
	canonical, err := IDToRelPath(id)
	if err != nil {
		return "", err
	}
	if strings.ReplaceAll(canonical, `\`, "/") != parts[0]+"/"+parts[1]+"/"+name+".md" {
		return "", fmt.Errorf("noncanonical entry path %q", logicalPath)
	}
	return id, nil
}

func (g *Graph) SelectedEntriesAfter(ids []string, after string) []*Entry {
	if ids == nil {
		return g.EntriesAfter(after)
	}
	start, found := slices.BinarySearch(ids, after)
	if found {
		start++
	}
	entries := make([]*Entry, 0, len(ids)-start)
	for _, id := range ids[start:] {
		if entry := g.ByID[id]; entry != nil {
			entries = append(entries, entry)
		}
	}
	return entries
}

func (g *Graph) EntrySelectionLoadError(ids []string, after string) error {
	for _, issue := range g.LoadIssues {
		id := issue.Ref
		if ValidateEntryID(id) != nil {
			var err error
			id, err = EntryIDForArtifactPath(issue.Ref)
			if err != nil || id == "" {
				if ids == nil {
					return fmt.Errorf("read entry %s: %s", issue.Ref, issue.Message)
				}
				continue
			}
		}
		if id <= after || (ids != nil && !slices.Contains(ids, id)) {
			continue
		}
		return fmt.Errorf("read entry %s: %s", issue.Ref, issue.Message)
	}
	return nil
}
