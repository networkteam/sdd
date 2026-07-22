package application

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"sort"
	"strings"

	"github.com/networkteam/sdd/internal/finders"
	"github.com/networkteam/sdd/internal/meta"
	"github.com/networkteam/sdd/internal/model"
	"gopkg.in/yaml.v3"
)

// Snapshot is an immutable, validated SDD graph snapshot. Its indexed model
// remains private; structured and filesystem stores both enter through
// SnapshotData. The finder is the shared read authority over the snapshot's
// graph; the private graph field mirrors it so in-package seams (write-side
// resolution, the engine graph provider) keep reading a *model.Graph directly.
type Snapshot struct {
	project  ProjectID
	revision string
	data     SnapshotData
	graph    *model.Graph
	finder   *finders.GraphFinder
	wip      []*model.WIPMarker
}

func (s *Snapshot) Project() ProjectID {
	if s == nil {
		return ""
	}
	return s.project
}

func (s *Snapshot) Revision() string {
	if s == nil {
		return ""
	}
	return s.revision
}

// SnapshotData contains canonical stored document facts, never derived graph
// indexes, status, or traversal state.
type SnapshotData struct {
	Project  ProjectID
	Revision string
	Config   ProjectConfigDocument
	Entries  []EntryDocument
	WIP      []WIPDocument
	// Unreadable records documents a store could not decode into structured
	// form — a file whose YAML frontmatter would not parse, for example. They
	// are carried as data rather than aborting the load: BuildSnapshot turns
	// each into a graph load issue surfaced through Snapshot.Health, so one
	// malformed file no longer makes an entire graph (and every session over
	// it) unopenable.
	Unreadable []DocumentIssue
}

// DocumentIssue names one document a store could not decode: its logical path
// and the decode error message.
type DocumentIssue struct {
	LogicalPath string
	Message     string
}

type ProjectConfigDocument struct {
	LogicalPath string
	Fields      map[string]any
}

// EntryDocument is the storage-neutral form of an entry. Frontmatter carries
// the canonical graph schema as structured values; SDD validates and
// normalizes it before constructing a Snapshot.
type EntryDocument struct {
	LogicalPath string
	Frontmatter map[string]any
	Body        string
	Attachments []string
}

type WIPDocument struct {
	LogicalPath string
	Content     string
}

// AttachmentDirRelPath returns the graph-relative attachment directory for
// an entry ID.
func AttachmentDirRelPath(entryID string) (string, error) {
	return model.AttachDirRelPath(entryID)
}

// BuildSnapshot is the single in-memory graph construction path. It adapts the
// canonical documents into a storage-neutral source and hands them to the
// shared GraphFinder, which applies the one semantic gate (parse, embedded-base
// merge, partial-read load issues) and holds the resulting graph. A malformed
// entry document no longer aborts the build — it surfaces as a load issue on
// the snapshot's graph (Snapshot.Health). Structural failures stay hard:
// missing project/revision, a malformed logical path, a WIP document outside
// wip/, and base-entry assembly.
func BuildSnapshot(_ context.Context, data SnapshotData) (*Snapshot, error) {
	if data.Project == "" {
		return nil, fmt.Errorf("sdd: snapshot project is required")
	}
	if data.Revision == "" {
		return nil, fmt.Errorf("sdd: snapshot revision is required")
	}
	source, err := newSnapshotDocumentSource(data)
	if err != nil {
		return nil, err
	}
	gf, err := finders.NewGraphFinder(source, finders.Options{})
	if err != nil {
		return nil, fmt.Errorf("sdd: %w", err)
	}
	markers, err := gf.WIPMarkers()
	if err != nil {
		return nil, fmt.Errorf("sdd: %w", err)
	}
	return &Snapshot{
		project:  data.Project,
		revision: data.Revision,
		data:     cloneSnapshotData(data),
		graph:    gf.Graph(),
		finder:   gf,
		wip:      markers,
	}, nil
}

// Health reports the snapshot graph's integrity problems — parse-failed
// (unreadable) documents and per-entry validation warnings — so external hosts
// can render graph health without reaching into the private model. A clean
// graph reports zero of each.
func (s *Snapshot) Health() model.GraphHealth {
	if s == nil || s.finder == nil {
		return model.GraphHealth{}
	}
	return s.finder.Health()
}

// snapshotDocumentSource adapts SnapshotData to the storage-neutral
// finders.DocumentSource: it hands structured entry documents (the GraphFinder
// renders their frontmatter to canonical YAML before the single ParseEntry
// gate), WIP marker documents, and the store's undecodable-document issues.
type snapshotDocumentSource struct {
	docs finders.GraphDocuments
}

// newSnapshotDocumentSource converts SnapshotData into the storage-neutral
// document set once, surfacing the structural failures that stay hard (a
// malformed entry logical path, a WIP document outside wip/).
func newSnapshotDocumentSource(data SnapshotData) (snapshotDocumentSource, error) {
	var docs finders.GraphDocuments
	for _, document := range data.Entries {
		id, err := model.RelPathToID(document.LogicalPath)
		if err != nil {
			return snapshotDocumentSource{}, fmt.Errorf("sdd: entry document %q: %w", document.LogicalPath, err)
		}
		docs.Entries = append(docs.Entries, finders.EntryDocument{
			ID:          id,
			Frontmatter: document.Frontmatter,
			Body:        document.Body,
			Attachments: append([]string(nil), document.Attachments...),
		})
	}
	for _, document := range data.WIP {
		if !strings.HasPrefix(document.LogicalPath, "wip/") {
			return snapshotDocumentSource{}, fmt.Errorf("sdd: WIP document %q is outside wip/", document.LogicalPath)
		}
		name := strings.TrimSuffix(strings.TrimPrefix(document.LogicalPath, "wip/"), ".md") + ".md"
		docs.WIP = append(docs.WIP, finders.WIPDocument{Name: name, Content: document.Content})
	}
	for _, issue := range data.Unreadable {
		docs.Unreadable = append(docs.Unreadable, finders.DocumentIssue{Ref: issue.LogicalPath, Message: issue.Message})
	}
	return snapshotDocumentSource{docs: docs}, nil
}

func (s snapshotDocumentSource) GraphDocuments() (finders.GraphDocuments, error) {
	return s.docs, nil
}

// LoadSnapshotFS parses canonical filesystem documents into SnapshotData and
// delegates all validation and indexing to BuildSnapshot.
func LoadSnapshotFS(ctx context.Context, project ProjectID, revision string, fsys fs.FS, graphDir string) (*Snapshot, error) {
	if graphDir == "" {
		graphDir = "."
	}
	data := SnapshotData{Project: project, Revision: revision}
	err := fs.WalkDir(fsys, graphDir, func(filename string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if filename != graphDir && meta.IsSDDMetaDir(entry) {
				return fs.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(entry.Name(), ".md") {
			return nil
		}
		rel := strings.TrimPrefix(filename, strings.TrimSuffix(graphDir, "/")+"/")
		if graphDir == "." {
			rel = strings.TrimPrefix(filename, "./")
		}
		raw, err := fs.ReadFile(fsys, filename)
		if err != nil {
			return err
		}
		if strings.HasPrefix(rel, "wip/") {
			data.WIP = append(data.WIP, WIPDocument{LogicalPath: rel, Content: string(raw)})
			return nil
		}
		if _, err := model.RelPathToID(rel); err != nil {
			return nil
		}
		document, err := parseEntryDocument(rel, raw)
		if err != nil {
			// A file whose frontmatter cannot even be decoded no longer aborts
			// the walk: it is recorded as an unreadable document that
			// BuildSnapshot turns into a graph load issue. File-read and walk
			// errors above stay hard failures.
			data.Unreadable = append(data.Unreadable, DocumentIssue{LogicalPath: rel, Message: err.Error()})
			return nil
		}
		data.Entries = append(data.Entries, document)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("sdd: walking graph documents: %w", err)
	}
	for index := range data.Entries {
		document := &data.Entries[index]
		attachmentDir := strings.TrimSuffix(document.LogicalPath, ".md")
		readDir := attachmentDir
		if graphDir != "." {
			readDir = strings.TrimSuffix(graphDir, "/") + "/" + attachmentDir
		}
		attachments, readErr := fs.ReadDir(fsys, readDir)
		if readErr != nil {
			if errors.Is(readErr, fs.ErrNotExist) {
				continue
			}
			return nil, fmt.Errorf("sdd: reading attachments for %s: %w", document.LogicalPath, readErr)
		}
		for _, attachment := range attachments {
			if !attachment.IsDir() {
				document.Attachments = append(document.Attachments, attachmentDir+"/"+attachment.Name())
			}
		}
		sort.Strings(document.Attachments)
	}
	return BuildSnapshot(ctx, data)
}

func parseEntryDocument(logicalPath string, raw []byte) (EntryDocument, error) {
	text := string(raw)
	if !strings.HasPrefix(text, "---\n") {
		return EntryDocument{}, fmt.Errorf("missing YAML frontmatter")
	}
	end := strings.Index(text[4:], "\n---")
	if end < 0 {
		return EntryDocument{}, fmt.Errorf("unterminated YAML frontmatter")
	}
	end += 4
	var frontmatter map[string]any
	if err := yaml.Unmarshal([]byte(text[4:end]), &frontmatter); err != nil {
		return EntryDocument{}, err
	}
	body := strings.TrimPrefix(text[end+4:], "\n")
	return EntryDocument{LogicalPath: logicalPath, Frontmatter: frontmatter, Body: strings.TrimSpace(body)}, nil
}

func cloneSnapshotData(data SnapshotData) SnapshotData {
	clone := data
	clone.Config.Fields = cloneMap(data.Config.Fields)
	clone.Entries = make([]EntryDocument, len(data.Entries))
	for i, document := range data.Entries {
		clone.Entries[i] = document
		clone.Entries[i].Frontmatter = cloneMap(document.Frontmatter)
		clone.Entries[i].Attachments = append([]string(nil), document.Attachments...)
	}
	clone.WIP = append([]WIPDocument(nil), data.WIP...)
	clone.Unreadable = append([]DocumentIssue(nil), data.Unreadable...)
	return clone
}

func cloneMap(input map[string]any) map[string]any {
	if input == nil {
		return nil
	}
	output := make(map[string]any, len(input))
	for key, value := range input {
		output[key] = value
	}
	return output
}
