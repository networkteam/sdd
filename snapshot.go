package sdd

// Snapshot is an immutable, validated SDD graph snapshot. Its indexed model
// remains private; structured and filesystem stores both enter through
// SnapshotData in the implementation slice.
type Snapshot struct {
	project  ProjectID
	revision string
	data     SnapshotData
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
}
