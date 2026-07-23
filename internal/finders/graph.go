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

// filesystemSource is the DocumentSource over a graph directory (hierarchical
// YYYY/MM/ layout). It hands raw canonical content — the CLI path does no
// double YAML parse, so it stays byte-identical to the direct on-disk read —
// and scans co-located attachment directories. It deliberately does not read
// wip/: the CLI resolves WIP markers lazily (GraphFinder.WIPMarkers) only when
// a layout needs them, so a plain graph load never touches or fails on them.
type filesystemSource struct {
	dir string
}

// GraphDocuments walks the graph directory and reads every entry file as raw
// canonical content. File-read and walk errors are hard failures; files that
// do not match the ID layout are skipped (not the graph's concern). A file
// whose content fails to parse is not detected here — the raw bytes flow to
// buildGraph, whose single ParseEntry gate records the failure as a LoadIssue.
func (s filesystemSource) GraphDocuments() (GraphDocuments, error) {
	var docs GraphDocuments
	walkErr := filepath.WalkDir(s.dir, func(path string, d fs.DirEntry, err error) error {
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

		rel, err := filepath.Rel(s.dir, path)
		if err != nil {
			return fmt.Errorf("getting relative path for %s: %w", path, err)
		}
		id, err := model.RelPathToID(rel)
		if err != nil {
			// Skip files that don't match the expected layout.
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("reading %s: %w", path, err)
		}
		docs.Entries = append(docs.Entries, EntryDocument{
			ID:          id,
			Raw:         data,
			Attachments: s.scanAttachments(id),
		})
		return nil
	})
	if walkErr != nil {
		return GraphDocuments{}, fmt.Errorf("walking graph dir: %w", walkErr)
	}
	return docs, nil
}

// scanAttachments lists the files in an entry's co-located attachment
// directory, returning graph-relative paths. A missing directory (the common
// case) yields none.
func (s filesystemSource) scanAttachments(id string) []string {
	attachRel, err := model.AttachDirRelPath(id)
	if err != nil {
		return nil
	}
	files, err := os.ReadDir(filepath.Join(s.dir, attachRel))
	if err != nil {
		return nil // no attachment directory
	}
	var attachments []string
	for _, file := range files {
		if !file.IsDir() {
			attachments = append(attachments, filepath.Join(attachRel, file.Name()))
		}
	}
	return attachments
}

// LoadGraph reads all .md files from dir (hierarchical YYYY/MM/ layout),
// joins the base procedure entries embedded in the binary, and builds the
// graph. Base entries are always loaded — a project graph never contains
// them on disk, but its entries may supersede them (procedure
// customization). On the unlikely ID collision, the disk entry wins: a
// project owns its graph directory.
//
// This is now a thin adapter: the filesystem DocumentSource supplies the raw
// documents and GraphFinder applies the single semantic gate (parse, base
// merge, partial-read LoadIssues). The signature is kept so CLI and handler
// callers are untouched; the graph's directory is recorded for provenance and
// lazy WIP resolution.
func (f *Finder) LoadGraph(dir string) (*model.Graph, error) {
	docs, err := filesystemSource{dir: dir}.GraphDocuments()
	if err != nil {
		return nil, err
	}
	// The CLI path needs only the graph; WIP markers are resolved lazily by
	// GraphFinder.WIPMarkers from the recorded directory when a layout asks.
	g, _, err := buildGraph(docs)
	if err != nil {
		return nil, err
	}
	g.SetGraphDir(dir)
	return g, nil
}
