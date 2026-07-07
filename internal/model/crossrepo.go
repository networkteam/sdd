package model

import (
	"fmt"
	"strings"
)

// Cross-repo reference IDs take the form <repo-id>:<entry-id>, where the
// repo-id is the target repository's canonical URL-shaped identity
// (host/path, e.g. github.com/networkteam/sdd) declared in that repo's
// committed .sdd/config.yaml, and the entry-id is a full local entry ID
// within that graph. The separator is the first colon. Cross-repo refs are
// stored verbatim, excluded from local reverse indexes and dangling-ref
// validation, and lifecycle edges (closes, supersedes) never cross the
// boundary — a remote target's own derived status is read from its cached
// graph for display only.

// SplitCrossRepoID splits id at the first colon into its repo-id prefix and
// entry-id remainder. isCrossRepo reports whether a colon was present at
// all — it does not validate either part (see ValidateCrossRepoID), so
// callers can distinguish "not cross-repo shaped" from "cross-repo shaped
// but malformed".
func SplitCrossRepoID(id string) (repoID, entryID string, isCrossRepo bool) {
	before, after, found := strings.Cut(id, ":")
	if !found {
		return "", "", false
	}
	return before, after, true
}

// IsCrossRepoID reports whether id is cross-repo shaped (contains a colon).
func IsCrossRepoID(id string) bool {
	return strings.ContainsRune(id, ':')
}

// ValidateRepoID checks that repoID is URL-shaped: a dotted hostname
// followed by at least one path segment (host/path, the Go-module
// convention), each segment limited to letters, digits, '.', '-' and '_'.
func ValidateRepoID(repoID string) error {
	if repoID == "" {
		return fmt.Errorf("repo ID is empty")
	}
	segments := strings.Split(repoID, "/")
	if len(segments) < 2 {
		return fmt.Errorf("repo ID %q must be host/path (e.g. github.com/org/repo)", repoID)
	}
	for _, seg := range segments {
		if seg == "" {
			return fmt.Errorf("repo ID %q has an empty path segment", repoID)
		}
		for _, c := range seg {
			if !isRepoIDChar(c) {
				return fmt.Errorf("repo ID %q contains invalid character %q", repoID, c)
			}
		}
	}
	if !strings.ContainsRune(segments[0], '.') {
		return fmt.Errorf("repo ID %q must start with a hostname (e.g. github.com)", repoID)
	}
	return nil
}

func isRepoIDChar(c rune) bool {
	switch {
	case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9':
		return true
	case c == '.', c == '-', c == '_':
		return true
	}
	return false
}

// DeriveRepoID normalizes a git remote URL to the canonical URL-shaped repo
// identity (host/path). ssh and https forms of the same remote derive equal:
// scheme, user info, and a trailing .git are stripped, the scp-like ssh form
// (git@host:org/repo) is folded into host/path, and the host lowercases
// (path case is preserved). An empty or unrecognizable remote returns an
// error — a repo without a derivable identity stays local-only.
func DeriveRepoID(remoteURL string) (string, error) {
	raw := strings.TrimSpace(remoteURL)
	if raw == "" {
		return "", fmt.Errorf("remote URL is empty")
	}

	// Strip scheme (https://, ssh://, git://).
	if i := strings.Index(raw, "://"); i >= 0 {
		raw = raw[i+3:]
	} else if at := strings.Index(raw, "@"); at >= 0 && strings.Contains(raw[at:], ":") {
		// scp-like ssh form: git@host:org/repo — fold the colon into a path
		// separator after dropping the user.
		raw = raw[at+1:]
		raw = strings.Replace(raw, ":", "/", 1)
	}
	// Strip user info remaining after a scheme (ssh://git@host/...).
	if at := strings.Index(raw, "@"); at >= 0 && at < strings.IndexByte(raw+"/", '/') {
		raw = raw[at+1:]
	}
	raw = strings.TrimSuffix(raw, "/")
	raw = strings.TrimSuffix(raw, ".git")

	slash := strings.IndexByte(raw, '/')
	if slash <= 0 || slash == len(raw)-1 {
		return "", fmt.Errorf("remote URL %q has no host/path form", remoteURL)
	}
	host := strings.ToLower(raw[:slash])
	// Drop an explicit port — identity is host-scoped, not endpoint-scoped.
	if i := strings.IndexByte(host, ':'); i >= 0 {
		host = host[:i]
	}
	id := host + raw[slash:]
	if err := ValidateRepoID(id); err != nil {
		return "", fmt.Errorf("remote URL %q does not derive a valid repo ID: %w", remoteURL, err)
	}
	return id, nil
}

// ValidateCrossRepoID syntactically validates both portions of a cross-repo
// reference ID: the repo-id must be URL-shaped and the entry-id must parse
// as a full entry ID. It does not resolve the target — resolution against
// the cached remote graph is the capture path's job.
func ValidateCrossRepoID(id string) error {
	repoID, entryID, ok := SplitCrossRepoID(id)
	if !ok {
		return fmt.Errorf("%q is not a cross-repo ID (missing colon)", id)
	}
	if err := ValidateRepoID(repoID); err != nil {
		return err
	}
	if _, err := ParseID(entryID); err != nil {
		return fmt.Errorf("cross-repo ref %q: invalid entry ID after colon: %w", id, err)
	}
	return nil
}
