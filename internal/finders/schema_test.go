package finders_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/networkteam/sdd/internal/finders"
	"github.com/networkteam/sdd/internal/model"
	"github.com/networkteam/sdd/internal/query"
)

func TestSchemaStatus_MissingMeta(t *testing.T) {
	tmp := t.TempDir()
	f := finders.New(finders.Options{})
	res, err := f.SchemaStatus(context.Background(), query.SchemaStatusQuery{
		SDDDir:              tmp,
		BinaryVersion:       "v0.2.0",
		BinarySchemaVersion: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.MetaExists {
		t.Errorf("MetaExists should be false when file absent")
	}
	if !res.Compatibility.Compatible {
		t.Errorf("absent meta should be treated as compatible")
	}
}

func TestSchemaStatus_ParsesAndChecks(t *testing.T) {
	tmp := t.TempDir()
	min := "v0.2.0"
	data, err := model.FormatSchemaMeta(model.SchemaMeta{GraphSchemaVersion: 1, MinimumVersion: &min})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmp, model.SchemaMetaFileName), data, 0o644); err != nil {
		t.Fatal(err)
	}

	f := finders.New(finders.Options{})

	// Binary at minimum passes.
	res, err := f.SchemaStatus(context.Background(), query.SchemaStatusQuery{
		SDDDir: tmp, BinaryVersion: "v0.2.0", BinarySchemaVersion: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !res.MetaExists {
		t.Error("MetaExists should be true")
	}
	if !res.Compatibility.Compatible {
		t.Errorf("v0.2.0 >= minimum_version v0.2.0 should pass: %+v", res.Compatibility)
	}

	// Binary below minimum refuses.
	res, err = f.SchemaStatus(context.Background(), query.SchemaStatusQuery{
		SDDDir: tmp, BinaryVersion: "v0.1.0", BinarySchemaVersion: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Compatibility.Compatible {
		t.Errorf("v0.1.0 < minimum_version v0.2.0 should refuse")
	}

	// Schema mismatch refuses.
	res, err = f.SchemaStatus(context.Background(), query.SchemaStatusQuery{
		SDDDir: tmp, BinaryVersion: "v0.2.0", BinarySchemaVersion: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Compatibility.Compatible {
		t.Errorf("schema version mismatch should refuse")
	}

	// Dev build bypasses.
	res, err = f.SchemaStatus(context.Background(), query.SchemaStatusQuery{
		SDDDir: tmp, BinaryVersion: "dev", BinarySchemaVersion: 999,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Compatibility.Compatible || !res.Compatibility.IsDevBuild {
		t.Errorf("dev build should bypass: %+v", res.Compatibility)
	}
}
