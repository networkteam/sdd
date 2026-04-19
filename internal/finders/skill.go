package finders

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/networkteam/sdd/internal/bundledskills"
	"github.com/networkteam/sdd/internal/model"
	"github.com/networkteam/sdd/internal/query"
)

// SkillStatus loads the embedded skill bundle for the query's target, then
// walks each entry to compare its embedded content against the file on disk.
// Classification happens in model.ComputeSkillStatus — the finder only
// supplies inputs.
func (f *Finder) SkillStatus(ctx context.Context, q query.SkillStatusQuery) (*query.SkillStatusResult, error) {
	target := q.Target
	if target == "" {
		target = model.DefaultAgentTarget
	}
	scope := q.Scope
	if scope == "" {
		scope = model.DefaultScope
	}

	installDir, err := model.SkillInstallDir(target, scope, q.RepoRoot, q.UserHome)
	if err != nil {
		return nil, fmt.Errorf("resolving skill install dir: %w", err)
	}

	bundle, err := bundledskills.Load(target)
	if err != nil {
		return nil, fmt.Errorf("loading embedded bundle: %w", err)
	}

	result := &query.SkillStatusResult{
		InstallDir: installDir,
		Entries:    make([]query.SkillStatusEntry, 0, len(bundle.Entries)),
	}
	for _, e := range bundle.Entries {
		abs := filepath.Join(installDir, e.Skill, e.RelPath)
		installed, err := readSkillFile(abs)
		if err != nil {
			return nil, fmt.Errorf("reading installed skill %s: %w", abs, err)
		}
		status := model.ComputeSkillStatus(e, installed)
		result.Entries = append(result.Entries, query.SkillStatusEntry{
			Skill:     e.Skill,
			RelPath:   e.RelPath,
			AbsPath:   abs,
			Status:    status,
			Embedded:  e,
			Installed: installed,
		})
	}
	return result, nil
}

// readSkillFile returns a parsed SkillFile for path, or nil if the file does
// not exist. Any other error is returned so callers can distinguish real
// read failures from missing files.
func readSkillFile(path string) (*model.SkillFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	return model.ParseSkillFile(path, data), nil
}
