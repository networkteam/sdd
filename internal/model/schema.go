package model

import (
	"encoding/json"
	"fmt"
	"regexp"

	"golang.org/x/mod/semver"
)

// SchemaMetaFileName is the committed metadata file under .sdd/ that records
// graph schema version and the oldest binary permitted to write to the graph.
const SchemaMetaFileName = "meta.json"

// CurrentGraphSchemaVersion is the schema version this binary knows how to
// read and write. Bumped only by an explicit schema migration.
const CurrentGraphSchemaVersion = 1

// SchemaMeta is the decoded contents of .sdd/meta.json.
//
// GraphSchemaVersion is required and advances through explicit migrations.
// MinimumVersion is written once at initial init (set to the binary's semver
// at creation time) and preserved thereafter; a nil pointer means the field
// is absent on disk.
type SchemaMeta struct {
	GraphSchemaVersion int     `json:"graph_schema_version"`
	MinimumVersion     *string `json:"minimum_version,omitempty"`
}

// ParseSchemaMeta decodes .sdd/meta.json bytes into a SchemaMeta.
func ParseSchemaMeta(data []byte) (*SchemaMeta, error) {
	var m SchemaMeta
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parsing schema meta: %w", err)
	}
	return &m, nil
}

// FormatSchemaMeta returns a pretty-printed JSON representation with a
// trailing newline, suitable for writing to disk.
func FormatSchemaMeta(m SchemaMeta) ([]byte, error) {
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("formatting schema meta: %w", err)
	}
	return append(data, '\n'), nil
}

// CompatibilityResult reports whether a binary is permitted to write to a
// graph given its .sdd/meta.json contents.
type CompatibilityResult struct {
	// Compatible is true when the binary may proceed. Dev builds are always
	// compatible (see IsDevBuild).
	Compatible bool

	// IsDevBuild is true when the binary version is non-semver; both gates
	// are bypassed.
	IsDevBuild bool

	// Reason is a human-readable explanation when !Compatible.
	Reason string
}

// semverVersionRE matches versions in the form vMAJOR.MINOR.PATCH with an
// optional pre-release or build suffix. Both leading-v and bare numeric forms
// are accepted; IsDevVersion treats anything else as a dev build.
var semverVersionRE = regexp.MustCompile(`^v?\d+\.\d+\.\d+([-+]\S+)?$`)

// IsDevVersion reports whether v is a non-release build. Dev builds (including
// the literal "dev", commit hashes, or any non-semver string) bypass write
// gates to avoid blocking local development against a released graph.
func IsDevVersion(v string) bool {
	return !semverVersionRE.MatchString(v)
}

// normalizeSemver ensures a version string carries a leading "v" so it can be
// compared with golang.org/x/mod/semver functions.
func normalizeSemver(v string) string {
	if v == "" {
		return v
	}
	if v[0] != 'v' {
		return "v" + v
	}
	return v
}

// CheckCompatibility evaluates meta against the binary's own version and
// declared schema version.
//
// Dev builds (non-semver binary versions) are always compatible; both gates
// are bypassed so local development isn't blocked by a released graph's
// minimum_version or a bumped schema_version.
//
// For released binaries: a schema_version mismatch errors with a pointer to
// sdd init / sdd migrate. A minimum_version higher than the binary errors
// with upgrade guidance. Either failure leaves Compatible = false.
func CheckCompatibility(meta SchemaMeta, binaryVersion string, binarySchemaVersion int) CompatibilityResult {
	if IsDevVersion(binaryVersion) {
		return CompatibilityResult{Compatible: true, IsDevBuild: true}
	}

	if meta.GraphSchemaVersion != binarySchemaVersion {
		return CompatibilityResult{
			Compatible: false,
			Reason: fmt.Sprintf(
				"graph schema version %d does not match binary schema version %d; run `sdd init` or a future `sdd migrate`",
				meta.GraphSchemaVersion, binarySchemaVersion,
			),
		}
	}

	if meta.MinimumVersion != nil {
		min := normalizeSemver(*meta.MinimumVersion)
		bin := normalizeSemver(binaryVersion)
		if !semver.IsValid(min) {
			return CompatibilityResult{
				Compatible: false,
				Reason:     fmt.Sprintf("minimum_version %q in .sdd/meta.json is not a valid semver string", *meta.MinimumVersion),
			}
		}
		if semver.Compare(bin, min) < 0 {
			return CompatibilityResult{
				Compatible: false,
				Reason: fmt.Sprintf(
					"binary version %s is below the graph's minimum_version %s; upgrade sdd to continue",
					binaryVersion, *meta.MinimumVersion,
				),
			}
		}
	}

	return CompatibilityResult{Compatible: true}
}
