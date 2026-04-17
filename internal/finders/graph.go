package finders

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/networkteam/sdd/internal/meta"
	"github.com/networkteam/sdd/internal/model"
)

// LoadGraph reads all .md files from dir (hierarchical YYYY/MM/ layout) and builds the graph.
func (f *Finder) LoadGraph(dir string) (*model.Graph, error) {
	var entries []*model.Entry

	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if model.IsWIPDir(d) {
			return fs.SkipDir
		}
		if meta.IsSDDMetaDir(d) {
			return fs.SkipDir
		}
		if d.IsDir() || !strings.HasSuffix(d.Name(), ".md") {
			return nil
		}

		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return fmt.Errorf("getting relative path for %s: %w", path, err)
		}

		id, err := model.RelPathToID(rel)
		if err != nil {
			// Skip files that don't match the expected layout
			return nil
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("reading %s: %w", path, err)
		}

		entry, err := model.ParseEntry(id+".md", string(data))
		if err != nil {
			return fmt.Errorf("parsing %s: %w", path, err)
		}

		entries = append(entries, entry)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walking graph dir: %w", err)
	}

	// Scan for attachment directories
	for _, e := range entries {
		attachRel, err := model.AttachDirRelPath(e.ID)
		if err != nil {
			continue
		}
		attachDir := filepath.Join(dir, attachRel)
		files, err := os.ReadDir(attachDir)
		if err != nil {
			continue // no attachment directory
		}
		for _, file := range files {
			if !file.IsDir() {
				e.Attachments = append(e.Attachments, filepath.Join(attachRel, file.Name()))
			}
		}
	}

	g := model.NewGraph(entries)
	g.SetGraphDir(dir)
	return g, nil
}
