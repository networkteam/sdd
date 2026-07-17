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
// SnapshotData.
type Snapshot struct {
	project  ProjectID
	revision string
	data     SnapshotData
	graph    *model.Graph
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

// BuildSnapshot is the single in-memory graph construction path. It validates
// canonical documents, joins embedded base procedures, and constructs the
// private indexed model without requiring a filesystem.
func BuildSnapshot(_ context.Context, data SnapshotData) (*Snapshot, error) {
	if data.Project == "" {
		return nil, fmt.Errorf("sdd: snapshot project is required")
	}
	if data.Revision == "" {
		return nil, fmt.Errorf("sdd: snapshot revision is required")
	}
	entries := make([]*model.Entry, 0, len(data.Entries))
	onDisk := make(map[string]bool, len(data.Entries))
	for _, document := range data.Entries {
		id, err := model.RelPathToID(document.LogicalPath)
		if err != nil {
			return nil, fmt.Errorf("sdd: entry document %q: %w", document.LogicalPath, err)
		}
		frontmatter, err := yaml.Marshal(document.Frontmatter)
		if err != nil {
			return nil, fmt.Errorf("sdd: encoding frontmatter for %s: %w", id, err)
		}
		content := "---\n" + string(frontmatter) + "---\n\n" + document.Body
		entry, err := model.ParseEntry(id+".md", content)
		if err != nil {
			return nil, fmt.Errorf("sdd: parsing entry document %q: %w", document.LogicalPath, err)
		}
		entry.Attachments = append([]string(nil), document.Attachments...)
		entries = append(entries, entry)
		onDisk[entry.ID] = true
	}
	base, err := finders.BaseEntries()
	if err != nil {
		return nil, fmt.Errorf("sdd: %w", err)
	}
	entries = model.MergeEmbedded(entries, onDisk, base)
	markers := make([]*model.WIPMarker, 0, len(data.WIP))
	for _, document := range data.WIP {
		if !strings.HasPrefix(document.LogicalPath, "wip/") {
			return nil, fmt.Errorf("sdd: WIP document %q is outside wip/", document.LogicalPath)
		}
		marker, err := model.ParseWIPMarker(strings.TrimSuffix(strings.TrimPrefix(document.LogicalPath, "wip/"), ".md")+".md", document.Content)
		if err != nil {
			return nil, fmt.Errorf("sdd: parsing WIP document %q: %w", document.LogicalPath, err)
		}
		markers = append(markers, marker)
	}
	return &Snapshot{
		project:  data.Project,
		revision: data.Revision,
		data:     cloneSnapshotData(data),
		graph:    model.NewGraph(entries),
		wip:      markers,
	}, nil
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
			return fmt.Errorf("sdd: parsing %s: %w", filename, err)
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
