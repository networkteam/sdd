package index_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/networkteam/sdd/internal/index"
)

func TestOpenBuilt_OpensNoIndexWhereNoneIsBuilt(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "index")
	store, err := index.OpenBuilt(dir)
	if err != nil || store != nil {
		t.Fatalf("a missing index should open as none, got %v, %v", store, err)
	}
	if _, err := os.Stat(dir); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("opening a missing index should not create it, got %v", err)
	}
}
