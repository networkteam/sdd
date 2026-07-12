package sdd

type InfoRequest struct{}

type InfoResult struct {
	Project     ProjectRef
	Participant string
	Language    string
	Search      string
}

type ViewRequest struct {
	Layout   string
	Repos    []string
	AllRepos bool
}

type ViewResult struct {
	Project  ProjectRef
	Sections string
}

type ShowRequest struct {
	IDs       []string
	UpDepth   int
	DownDepth int
}

type ShowResult struct {
	Project ProjectRef
	Entries string
}

type SearchRequest struct {
	Terms             []string
	Phrase            string
	Type              string
	Layer             string
	Kind              string
	IncludeSuperseded bool
	Limit             int
	MaxCitations      int
	Repos             []string
	AllRepos          bool
}

type SearchResult struct {
	Project ProjectRef
	Results string
}

type ReadAttachmentRequest struct {
	EntryID  string
	Filename string
	Offset   int64
	MaxBytes int
}

type ReadAttachmentResult struct {
	Project ProjectRef
	Page    AttachmentPage
}

type ProcedureListRequest struct{}

type ProcedureListResult struct {
	Project    ProjectRef
	Procedures string
}
