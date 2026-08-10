// Package repostest builds user-global config fixtures for tests in other
// packages. It is separate from repos so that no production code can reach a
// whole-document config writer — real writes patch, and dropping the keys and
// comments a rewrite would drop is what 20260810-144515-s-tac-8ae is about.
package repostest

import (
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/networkteam/sdd/internal/repos"
)

// WriteConfig renders cfg over the file at path, creating parent directories.
func WriteConfig(t *testing.T, path string, cfg *repos.GlobalConfig) {
	t.Helper()
	data, err := yaml.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshaling global config fixture: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("creating config dir for fixture: %v", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("writing global config fixture %s: %v", path, err)
	}
}
