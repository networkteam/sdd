package finders

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/networkteam/resonance/framework/sdd/model"
)

// LoadWIPMarkers reads all marker files from the wip/ subdirectory of graphDir.
func (f *Finder) LoadWIPMarkers(graphDir string) ([]*model.WIPMarker, error) {
	wipDir := filepath.Join(graphDir, "wip")

	entries, err := os.ReadDir(wipDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // no wip directory = no markers
		}
		return nil, fmt.Errorf("reading wip directory: %w", err)
	}

	var markers []*model.WIPMarker
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}

		data, err := os.ReadFile(filepath.Join(wipDir, entry.Name()))
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", entry.Name(), err)
		}

		marker, err := model.ParseWIPMarker(entry.Name(), string(data))
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
