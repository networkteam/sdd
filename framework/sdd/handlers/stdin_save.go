package handlers

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"

	"github.com/networkteam/resonance/framework/sdd/meta"
)

// stdinTmpDir returns the directory for persisting rejected/dry-run stdin
// attachments. Requires h.sddDir to be set (enforced at CLI level).
func (h *Handler) stdinTmpDir() string {
	return meta.TmpDir(h.sddDir)
}

// saveStdinAttachment writes stdin content to <tmpDir>/<hash>-<target>
// and returns the absolute saved path. The hash is the first 8 chars of sha256
// of the content — stable enough for single-user iteration.
// The tmpDir is auto-created if it doesn't exist.
func saveStdinAttachment(tmpDir string, target string, data []byte) (string, error) {
	sum := sha256.Sum256(data)
	hash := hex.EncodeToString(sum[:])[:8]
	if err := os.MkdirAll(tmpDir, 0755); err != nil {
		return "", fmt.Errorf("creating %s: %w", tmpDir, err)
	}
	path := filepath.Join(tmpDir, hash+"-"+target)
	if err := os.WriteFile(path, data, 0644); err != nil {
		return "", fmt.Errorf("writing %s: %w", path, err)
	}
	return path, nil
}
