package application_test

import (
	"errors"
	"io/fs"
	"testing"
	"testing/fstest"

	sdd "github.com/networkteam/sdd/pkg/application"
)

func TestPageAttachment(t *testing.T) {
	const entry = "20260713-120000-s-tac-att"
	one := fstest.MapFS{
		"graph/2026/07/13-120000-s-tac-att.md":              {Data: []byte("---\ntype: signal\n---\n")},
		"graph/2026/07/13-120000-s-tac-att/evidence.txt":    {Data: []byte("0123456789")},
		"graph/2026/07/13-120000-s-tac-att/nested/deep.txt": {Data: []byte("not an attachment")},
	}

	first, err := sdd.PageAttachment(one, "graph", entry, "", 0, 4)
	if err != nil {
		t.Fatal(err)
	}
	if first.Filename != "evidence.txt" || string(first.Content) != "0123" || first.NextOffset != 4 || first.TotalSize != 10 || !first.More {
		t.Fatalf("first page = %+v", first)
	}
	last, err := sdd.PageAttachment(one, "graph", entry, "evidence.txt", first.NextOffset, 100)
	if err != nil {
		t.Fatal(err)
	}
	if string(last.Content) != "456789" || last.More || last.NextOffset != 10 || last.Digest != first.Digest || last.Digest.Algorithm != "sha256" {
		t.Fatalf("last page = %+v", last)
	}
	past, err := sdd.PageAttachment(one, "graph", entry, "evidence.txt", 50, 4)
	if err != nil || len(past.Content) != 0 || past.Offset != 10 || past.More {
		t.Fatalf("page past the end = %+v, %v", past, err)
	}

	two := fstest.MapFS{
		"2026/07/13-120000-s-tac-att/a.txt": {Data: []byte("a")},
		"2026/07/13-120000-s-tac-att/b.txt": {Data: []byte("b")},
	}
	if page, err := sdd.PageAttachment(two, "", entry, "b.txt", 0, 8); err != nil || string(page.Content) != "b" {
		t.Fatalf("named attachment at the graph root = %+v, %v", page, err)
	}

	for name, call := range map[string]func() error{
		"bad range":     func() error { _, err := sdd.PageAttachment(one, "graph", entry, "", -1, 4); return err },
		"zero page":     func() error { _, err := sdd.PageAttachment(one, "graph", entry, "", 0, 0); return err },
		"name required": func() error { _, err := sdd.PageAttachment(two, "", entry, "", 0, 8); return err },
		"no attachments": func() error {
			_, err := sdd.PageAttachment(fstest.MapFS{"2026/07/13-120000-s-tac-att/.keep": {Mode: fs.ModeDir}}, "", entry, "", 0, 8)
			return err
		},
		"nested name":    func() error { _, err := sdd.PageAttachment(one, "graph", entry, "nested/deep.txt", 0, 8); return err },
		"escaping name":  func() error { _, err := sdd.PageAttachment(one, "graph", entry, "..", 0, 8); return err },
		"backslash name": func() error { _, err := sdd.PageAttachment(one, "graph", entry, `a\b`, 0, 8); return err },
	} {
		err := call()
		var appErr *sdd.ApplicationError
		if !errors.As(err, &appErr) || appErr.Code != sdd.ErrorInvalidArgument {
			t.Errorf("%s: err = %v, want invalid_argument", name, err)
		}
	}

	if _, err := sdd.PageAttachment(one, "graph", entry, "missing.txt", 0, 8); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("missing file = %v, want fs.ErrNotExist", err)
	}
	if _, err := sdd.PageAttachment(one, "graph", "20260713-120000-s-tac-oth", "", 0, 8); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("missing entry dir = %v, want fs.ErrNotExist", err)
	}
}
