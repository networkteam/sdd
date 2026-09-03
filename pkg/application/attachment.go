package application

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"path"
	"strings"
)

// PageAttachment reads one page of an entry's attachment from a graph
// directory. An empty filename selects the entry's only attachment and fails
// when there are several. A caller's mistake — bad range, bad name, no
// attachment to infer — is an ApplicationError with ErrorInvalidArgument; a
// missing entry directory or file surfaces as the filesystem's fs.ErrNotExist.
// Paths never leave graphDir: the filename must be a bare name and the
// resulting path must be valid for fs.FS. Locking and recovery around the
// read belong to the store that owns the directory.
func PageAttachment(fsys fs.FS, graphDir, entryID, filename string, offset int64, maxBytes int) (AttachmentPage, error) {
	if offset < 0 || maxBytes <= 0 {
		return AttachmentPage{}, invalidAttachmentArgument("invalid attachment page range")
	}
	attachmentDir, err := AttachmentDirRelPath(entryID)
	if err != nil {
		return AttachmentPage{}, err
	}
	if graphDir == "" {
		graphDir = "."
	}
	dir := path.Join(graphDir, attachmentDir)
	if filename == "" {
		entries, readErr := fs.ReadDir(fsys, dir)
		if readErr != nil {
			return AttachmentPage{}, readErr
		}
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			if filename != "" {
				return AttachmentPage{}, invalidAttachmentArgument(fmt.Sprintf("attachment name is required when entry %s has more than one attachment", entryID))
			}
			filename = entry.Name()
		}
		if filename == "" {
			return AttachmentPage{}, invalidAttachmentArgument(fmt.Sprintf("entry %s has no attachments", entryID))
		}
	}
	if filename == "." || filename == ".." || path.Base(filename) != filename || strings.ContainsAny(filename, `/\`) {
		return AttachmentPage{}, invalidAttachmentArgument(fmt.Sprintf("invalid attachment filename %q", filename))
	}
	data, err := fs.ReadFile(fsys, path.Join(dir, filename))
	if err != nil {
		return AttachmentPage{}, err
	}
	if offset > int64(len(data)) {
		offset = int64(len(data))
	}
	end := min(offset+int64(maxBytes), int64(len(data)))
	sum := sha256.Sum256(data)
	return AttachmentPage{
		Filename: filename, Content: append([]byte(nil), data[offset:end]...), Offset: offset,
		NextOffset: end, TotalSize: int64(len(data)), More: end < int64(len(data)),
		Digest: BlobDigest{Algorithm: "sha256", Value: hex.EncodeToString(sum[:])},
	}, nil
}

func invalidAttachmentArgument(message string) error {
	return &ApplicationError{Code: ErrorInvalidArgument, Message: "sdd: " + message}
}
