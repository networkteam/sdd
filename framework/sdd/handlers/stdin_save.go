package handlers

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
)

// stdinSaveDir is the directory under graph-dir where rejected/dry-run stdin
// attachments are persisted so users can re-pipe them without re-sending.
const stdinSaveDir = ".sdd-tmp"

// saveStdinAttachment writes stdin content to <graph-dir>/.sdd-tmp/<hash>-<target>
// and returns the absolute saved path. The hash is the first 8 chars of sha256
// of the content — stable enough for single-user iteration.
func saveStdinAttachment(graphDir string, target string, data []byte) (string, error) {
	sum := sha256.Sum256(data)
	hash := hex.EncodeToString(sum[:])[:8]
	dir := filepath.Join(graphDir, stdinSaveDir)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("creating %s: %w", dir, err)
	}
	path := filepath.Join(dir, hash+"-"+target)
	if err := os.WriteFile(path, data, 0644); err != nil {
		return "", fmt.Errorf("writing %s: %w", path, err)
	}
	return path, nil
}
