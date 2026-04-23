package meta

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestReadLastFetch_MissingReturnsZero(t *testing.T) {
	sddDir := t.TempDir()
	got, err := ReadLastFetch(sddDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !got.IsZero() {
		t.Errorf("expected zero time for missing marker, got %v", got)
	}
}

func TestTouchLastFetch_CreatesAndUpdates(t *testing.T) {
	sddDir := t.TempDir()

	// First touch creates the file.
	if err := TouchLastFetch(sddDir); err != nil {
		t.Fatalf("first TouchLastFetch: %v", err)
	}
	first, err := ReadLastFetch(sddDir)
	if err != nil {
		t.Fatalf("ReadLastFetch after first touch: %v", err)
	}
	if first.IsZero() {
		t.Fatal("expected non-zero mtime after touch")
	}

	// Verify the file lives at .sdd/tmp/last-fetch.
	expected := filepath.Join(sddDir, "tmp", "last-fetch")
	if _, err := os.Stat(expected); err != nil {
		t.Errorf("expected marker at %s, got %v", expected, err)
	}

	// Second touch updates the mtime.
	if err := os.Chtimes(expected, time.Now().Add(-time.Hour), time.Now().Add(-time.Hour)); err != nil {
		t.Fatalf("backdating mtime: %v", err)
	}
	backdated, _ := ReadLastFetch(sddDir)
	if err := TouchLastFetch(sddDir); err != nil {
		t.Fatalf("second TouchLastFetch: %v", err)
	}
	after, _ := ReadLastFetch(sddDir)
	if !after.After(backdated) {
		t.Errorf("second touch did not bump mtime: before=%v after=%v", backdated, after)
	}
}
