package query

import "github.com/networkteam/sdd/internal/model"

// ReadAttachmentQuery captures intent to read an entry's attachment content
// through the CLI-owned accessor, so agents never derive storage paths
// themselves (20260606-004059-d-tac-d21). Content is paged for remote
// consumers: Offset is a byte position into the file, MaxBytes caps the
// returned slice.
type ReadAttachmentQuery struct {
	Graph    *model.Graph
	GraphDir string
	// EntryID is the full ID of the entry whose attachment is read.
	EntryID string
	// Name selects the attachment by filename. Optional when the entry has
	// exactly one attachment.
	Name string
	// Offset is the byte position to start reading from (0 = start).
	Offset int64
	// MaxBytes caps the returned content size. Zero applies the default page
	// size.
	MaxBytes int
}

// DefaultAttachmentPageBytes is the page size when MaxBytes is unset —
// large enough that most attachments arrive in one call, small enough to
// stay well inside a tool-response budget.
const DefaultAttachmentPageBytes = 64 * 1024

// ReadAttachmentResult is one page of attachment content plus the paging
// state a caller needs to continue.
type ReadAttachmentResult struct {
	EntryID string
	// Name is the resolved attachment filename.
	Name string
	// Content is the page read, as UTF-8 text.
	Content string
	// Offset echoes the byte position the page started at.
	Offset int64
	// NextOffset is the byte position to pass for the following page; only
	// meaningful when More is true.
	NextOffset int64
	// TotalBytes is the attachment's full size.
	TotalBytes int64
	// More reports whether content remains past this page.
	More bool
	// Available lists the entry's attachment filenames — the discoverable
	// surface when Name was ambiguous or the caller wants the inventory.
	Available []string
	// Path is the attachment's absolute filesystem path. The accessor always
	// resolves it; consumers decide whether to expose it (the MCP tool hands
	// it out only to local clients, which can then read the file directly
	// instead of paging).
	Path string
}
