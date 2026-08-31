package local_test

import (
	"path/filepath"
	"testing"
)

func canonicalTempDir(t *testing.T) string {
	t.Helper()
	path := t.TempDir()
	canonical, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatal(err)
	}
	return canonical
}
