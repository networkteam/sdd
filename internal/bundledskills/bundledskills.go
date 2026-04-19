// Package bundledskills exposes the skill files embedded in the sdd binary.
//
// Skills are committed under this package (one subdirectory per agent target)
// and compiled into the binary via //go:embed. At runtime, sdd init extracts
// them to the target agent's skill directory with install-time stamps
// injected into each file's frontmatter.
//
// Editing: skill source of truth is here (e.g. claude/sdd/SKILL.md). After
// changes, rebuild the binary and run ./bin/sdd init to refresh the installed
// copy under .claude/skills/ (or ~/.claude/skills/).
package bundledskills

import (
	"embed"
	"fmt"
	"io/fs"
	"path"
	"strings"

	"github.com/networkteam/sdd/internal/model"
)

// claudeSkillsFS holds the embedded Claude skill tree.
//
// The `all:` prefix ensures files starting with '.' or '_' (e.g. a future
// .gitignore inside a skill's references/ dir) are still embedded; without it
// //go:embed silently skips them.
//
//go:embed all:claude
var claudeSkillsFS embed.FS

// Load returns the embedded skill bundle for the given agent target.
func Load(target model.AgentTarget) (*model.SkillBundle, error) {
	switch target {
	case model.AgentClaude:
		return loadFromFS(claudeSkillsFS, "claude", target)
	default:
		return nil, fmt.Errorf("unsupported agent target: %s", target)
	}
}

// loadFromFS walks the embedded FS under root and builds a SkillBundle. Each
// leaf file becomes a SkillBundleEntry with its RelPath expressed relative to
// the skill directory (e.g. "SKILL.md", "references/cli-reference.md").
func loadFromFS(efs embed.FS, root string, target model.AgentTarget) (*model.SkillBundle, error) {
	bundle := &model.SkillBundle{Target: target}

	err := fs.WalkDir(efs, root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}

		// Relative to root: e.g. "sdd/SKILL.md", "sdd/references/cli-reference.md".
		rel, err := relPath(root, p)
		if err != nil {
			return err
		}

		// First path segment is the skill directory name.
		skill, sub, ok := strings.Cut(rel, "/")
		if !ok {
			// File sitting directly under claude/ — skip; every file must
			// live inside a skill dir.
			return nil
		}

		data, err := efs.ReadFile(p)
		if err != nil {
			return fmt.Errorf("reading embedded %s: %w", p, err)
		}

		bundle.Entries = append(bundle.Entries, model.SkillBundleEntry{
			Skill:   skill,
			RelPath: sub,
			Content: data,
		})
		return nil
	})
	if err != nil {
		return nil, err
	}
	return bundle, nil
}

func relPath(root, full string) (string, error) {
	rel := strings.TrimPrefix(full, root)
	rel = strings.TrimPrefix(rel, "/")
	if rel == "" {
		return "", fmt.Errorf("unexpected empty relative path for %q", full)
	}
	// Normalize in case the platform embedder returned backslashes (it
	// shouldn't — embed uses forward slashes — but be defensive).
	return path.Clean(rel), nil
}
