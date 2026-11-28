package application_test

import (
	"context"
	"errors"
	"io/fs"
	"path"
	"reflect"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	sdd "github.com/networkteam/sdd/pkg/application"
)

type snapshotOpenFS struct {
	fs.FS
	blocked map[string]error
	opened  []string
}

func (f *snapshotOpenFS) Open(name string) (fs.File, error) {
	f.opened = append(f.opened, name)
	if err := f.blocked[name]; err != nil {
		return nil, err
	}
	return f.FS.Open(name)
}

func TestWalkSnapshotFilesClassifiesWithoutOpeningContent(t *testing.T) {
	for _, graphDir := range []string{"", ".", "graph"} {
		t.Run("root="+graphDir, func(t *testing.T) {
			files := map[string]bool{
				"2026/07/13-010000-s-tac-api.md":              true,
				"2026/07/malformed.md":                        true,
				"wip/20260713-010000-reader.md":               true,
				"wip/nested/marker.md":                        true,
				"README.md":                                   false,
				"2026/07/13-010000-s-tac-api/evidence.md":     false,
				"2026/07/13-010000-s-tac-api/image.bin":       false,
				"2026/07/13-010000-s-tac-api/nested/notes.md": false,
			}
			underlying := fstest.MapFS{}
			blocked := map[string]error{}
			want := map[string]bool{}
			for name, document := range files {
				filename := path.Join(graphDir, name)
				underlying[filename] = &fstest.MapFile{Data: []byte("unread content")}
				blocked[filename] = errors.New("file content must not open")
				want[filename] = document
			}
			link := path.Join(graphDir, "2026/07/14-010000-s-tac-link.md")
			underlying[link] = &fstest.MapFile{Mode: fs.ModeSymlink}
			blocked[link] = errors.New("symlink content must not open")
			want[link] = true
			metadata := path.Join(graphDir, ".sdd")
			underlying[path.Join(metadata, "hidden.md")] = &fstest.MapFile{}
			blocked[metadata] = errors.New("metadata directory must not open")
			fsys := &snapshotOpenFS{FS: underlying, blocked: blocked}
			got := map[string]bool{}
			for file, err := range sdd.WalkSnapshotFiles(t.Context(), fsys, graphDir) {
				if err != nil {
					t.Fatal(err)
				}
				if _, duplicate := got[file.Path]; duplicate {
					t.Fatalf("duplicate file %q", file.Path)
				}
				got[file.Path] = file.Document
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("files = %#v, want %#v", got, want)
			}
			for _, opened := range fsys.opened {
				if blocked[opened] != nil {
					t.Fatalf("walk opened excluded content %q", opened)
				}
			}
		})
	}
}

func TestWalkSnapshotFilesStopsTraversal(t *testing.T) {
	for _, cancel := range []bool{false, true} {
		t.Run(map[bool]string{false: "consumer stops", true: "context canceled"}[cancel], func(t *testing.T) {
			ctx, cancelContext := context.WithCancel(t.Context())
			defer cancelContext()
			fsys := &snapshotOpenFS{
				FS:      fstest.MapFS{"a.txt": {}, "later/b.txt": {}},
				blocked: map[string]error{"later": errors.New("later directory must not open")},
			}
			var files, failures int
			for file, err := range sdd.WalkSnapshotFiles(ctx, fsys, ".") {
				if err != nil {
					failures++
					if !cancel || !errors.Is(err, context.Canceled) {
						t.Fatalf("walk error = %v", err)
					}
					continue
				}
				files++
				if file.Path != "a.txt" {
					t.Fatalf("unexpected file after stop: %q", file.Path)
				}
				if !cancel {
					break
				}
				cancelContext()
			}
			if files != 1 || (cancel && failures != 1) {
				t.Fatalf("files = %d, errors = %d", files, failures)
			}
			if slices.Contains(fsys.opened, "later") {
				t.Fatal("walk opened a directory after stopping")
			}
		})
	}
}

func TestWalkSnapshotFilesAlreadyCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	fsys := &snapshotOpenFS{FS: fstest.MapFS{}}
	var failures int
	for _, err := range sdd.WalkSnapshotFiles(ctx, fsys, ".") {
		failures++
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("walk error = %v, want context.Canceled", err)
		}
	}
	if failures != 1 || len(fsys.opened) != 0 {
		t.Fatalf("errors = %d, opened = %v", failures, fsys.opened)
	}
}

func TestWalkSnapshotFilesDirectoryResults(t *testing.T) {
	readFailure := errors.New("directory unavailable")
	for _, tc := range []struct {
		name string
		fsys fs.FS
		want error
	}{
		{name: "empty", fsys: fstest.MapFS{"graph": {Mode: fs.ModeDir}}},
		{name: "missing", fsys: fstest.MapFS{}, want: fs.ErrNotExist},
		{name: "unreadable", fsys: &snapshotOpenFS{
			FS:      fstest.MapFS{"graph/later/file.txt": {}},
			blocked: map[string]error{"graph/later": readFailure},
		}, want: readFailure},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var got error
			var failures int
			for file, err := range sdd.WalkSnapshotFiles(t.Context(), tc.fsys, "graph") {
				if err == nil {
					t.Fatalf("unexpected file: %+v", file)
				}
				failures++
				got = err
			}
			if !errors.Is(got, tc.want) || failures > 1 {
				t.Fatalf("walk error = %v, count = %d, want %v", got, failures, tc.want)
			}
		})
	}
}

func TestLoadSnapshotFSPreservesAttachmentNamesWithoutOpeningBodies(t *testing.T) {
	const entryPath = "2026/07/13-010000-s-tac-api.md"
	const entryID = "20260713-010000-s-tac-api"
	const wip = "---\nentry: " + entryID + "\nparticipant: Reader\n---\n\nSnapshot loader work."
	for _, graphDir := range []string{"", ".", "graph"} {
		t.Run("root="+graphDir, func(t *testing.T) {
			attachmentDir := strings.TrimSuffix(entryPath, ".md")
			underlying := fstest.MapFS{
				path.Join(graphDir, entryPath):                       {Data: []byte(applicationEntry)},
				path.Join(graphDir, "wip/20260713-010000-reader.md"): {Data: []byte(wip)},
			}
			blocked := map[string]error{}
			for _, name := range []string{"evidence.md", "image.bin", "nested/notes.md"} {
				filename := path.Join(graphDir, attachmentDir, name)
				underlying[filename] = &fstest.MapFile{Data: []byte("attachment content")}
				blocked[filename] = errors.New("attachment body must not open")
			}
			fsys := &snapshotOpenFS{FS: underlying, blocked: blocked}
			snapshot, err := sdd.LoadSnapshotFS(t.Context(), "base", "r1", fsys, graphDir)
			if err != nil {
				t.Fatal(err)
			}
			for _, opened := range fsys.opened {
				if blocked[opened] != nil {
					t.Fatalf("loader opened attachment body %q", opened)
				}
			}
			store := staticGraphStore{snapshot: snapshot}
			app := preparationApp(t, acquiredRuntime(t, "base", store), nil, nil)
			identity := sdd.RequestIdentity{Subject: "reader"}
			shown, err := app.Show(t.Context(), identity, "base", sdd.ShowRequest{IDs: []string{entryID}})
			if err != nil || !strings.Contains(showText(shown), "The protocol-neutral runtime owns graph reads") {
				t.Fatalf("entry = %q, %v", showText(shown), err)
			}
			markers, err := app.View(t.Context(), identity, "base", sdd.ViewRequest{Layout: "source(wip):as-wip-list"})
			if err != nil || !strings.Contains(markers.Sections, "Snapshot loader work.") {
				t.Fatalf("WIP = %q, %v", markers.Sections, err)
			}
			attachments, err := app.ReadAttachment(t.Context(), identity, "base", sdd.ReadAttachmentRequest{EntryID: entryID, Filename: "evidence.md", MaxBytes: 1})
			if err != nil || !slices.Equal(attachments.Available, []string{"evidence.md", "image.bin"}) {
				t.Fatalf("attachment names = %v, %v", attachments.Available, err)
			}
		})
	}
}

func TestLoadSnapshotFSPreservesDocumentReadErrors(t *testing.T) {
	readFailure := errors.New("document unavailable")
	for _, filename := range []string{"graph/2026/07/13-010000-s-tac-api.md", "graph/wip/20260713-010000-reader.md"} {
		t.Run(filename, func(t *testing.T) {
			fsys := &snapshotOpenFS{FS: fstest.MapFS{filename: {}}, blocked: map[string]error{filename: readFailure}}
			snapshot, err := sdd.LoadSnapshotFS(t.Context(), "base", "r1", fsys, "graph")
			if snapshot != nil || !errors.Is(err, readFailure) {
				t.Fatalf("snapshot = %v, error = %v, want document read error", snapshot, err)
			}
		})
	}
}

func TestLoadSnapshotFSReportsMalformedDocumentHealth(t *testing.T) {
	snapshot, err := sdd.LoadSnapshotFS(t.Context(), "base", "r1", fstest.MapFS{
		"graph/2026/07/13-010000-s-tac-api.md": {Data: []byte("---\n[invalid YAML\n---\n")},
	}, "graph")
	if err != nil {
		t.Fatal(err)
	}
	if health := snapshot.Health(); health.LoadErrors != 1 {
		t.Fatalf("health = %+v, want one unreadable document", health)
	}
}
