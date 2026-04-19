package model

import "testing"

func TestIsDevVersion(t *testing.T) {
	cases := []struct {
		v      string
		wantIs bool
	}{
		{"dev", true},
		{"", true},
		{"abc123", true},
		{"v0.1.0", false},
		{"0.1.0", false},
		{"v1.2.3-beta.1", false},
		{"v1.2.3+meta", false},
		{"v1.2", true},
		{"1", true},
	}
	for _, c := range cases {
		got := IsDevVersion(c.v)
		if got != c.wantIs {
			t.Errorf("IsDevVersion(%q) = %v, want %v", c.v, got, c.wantIs)
		}
	}
}

func TestCheckCompatibility_DevBypass(t *testing.T) {
	meta := SchemaMeta{GraphSchemaVersion: 999}
	min := "v99.0.0"
	meta.MinimumVersion = &min

	got := CheckCompatibility(meta, "dev", 1)
	if !got.Compatible || !got.IsDevBuild {
		t.Errorf("dev build must bypass all gates: %+v", got)
	}
}

func TestCheckCompatibility_SchemaMismatch(t *testing.T) {
	got := CheckCompatibility(SchemaMeta{GraphSchemaVersion: 2}, "v0.2.0", 1)
	if got.Compatible {
		t.Errorf("schema mismatch should refuse: %+v", got)
	}
	if got.Reason == "" {
		t.Errorf("expected reason text for schema mismatch")
	}
}

func TestCheckCompatibility_MinVersionGate(t *testing.T) {
	v := "v0.3.0"
	meta := SchemaMeta{GraphSchemaVersion: 1, MinimumVersion: &v}

	if got := CheckCompatibility(meta, "v0.2.0", 1); got.Compatible {
		t.Errorf("binary below minimum should refuse: %+v", got)
	}
	if got := CheckCompatibility(meta, "v0.3.0", 1); !got.Compatible {
		t.Errorf("binary == minimum should pass: %+v", got)
	}
	if got := CheckCompatibility(meta, "v1.0.0", 1); !got.Compatible {
		t.Errorf("binary > minimum should pass: %+v", got)
	}
}

func TestCheckCompatibility_NoMinVersion(t *testing.T) {
	got := CheckCompatibility(SchemaMeta{GraphSchemaVersion: 1}, "v0.1.0", 1)
	if !got.Compatible {
		t.Errorf("absent minimum_version should pass: %+v", got)
	}
}

func TestCheckCompatibility_InvalidMinVersion(t *testing.T) {
	bad := "not-a-version"
	got := CheckCompatibility(SchemaMeta{GraphSchemaVersion: 1, MinimumVersion: &bad}, "v1.0.0", 1)
	if got.Compatible {
		t.Errorf("invalid minimum_version should refuse: %+v", got)
	}
}

func TestSchemaMetaRoundTrip(t *testing.T) {
	v := "v0.2.0"
	orig := SchemaMeta{GraphSchemaVersion: 1, MinimumVersion: &v}

	data, err := FormatSchemaMeta(orig)
	if err != nil {
		t.Fatalf("FormatSchemaMeta: %v", err)
	}
	got, err := ParseSchemaMeta(data)
	if err != nil {
		t.Fatalf("ParseSchemaMeta: %v", err)
	}
	if got.GraphSchemaVersion != 1 {
		t.Errorf("schema version: got %d, want 1", got.GraphSchemaVersion)
	}
	if got.MinimumVersion == nil || *got.MinimumVersion != "v0.2.0" {
		t.Errorf("minimum_version round-trip lost value: %+v", got.MinimumVersion)
	}
}

func TestSchemaMetaOmitsAbsentMinVersion(t *testing.T) {
	orig := SchemaMeta{GraphSchemaVersion: 1}
	data, err := FormatSchemaMeta(orig)
	if err != nil {
		t.Fatalf("FormatSchemaMeta: %v", err)
	}
	// omitempty on *string should drop the field entirely.
	if contains(data, "minimum_version") {
		t.Errorf("expected omitempty to drop minimum_version, got: %s", data)
	}
}

func contains(b []byte, s string) bool {
	return indexOf(b, s) >= 0
}

func indexOf(b []byte, s string) int {
	if len(s) == 0 {
		return 0
	}
	for i := 0; i+len(s) <= len(b); i++ {
		if string(b[i:i+len(s)]) == s {
			return i
		}
	}
	return -1
}
