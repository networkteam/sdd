package handlers

import (
	"fmt"
	"os"
	"path/filepath"
)

// configFile is one config document on disk. Every config write in sdd goes
// through it as a field-level patch rather than a whole-file rewrite, so keys
// this binary does not know — and the comments around them — survive a write
// by any version (20260810-144515-s-tac-8ae).
//
// The committed layer is world-readable; the two that may carry API keys are
// not.
type configFile struct {
	path string
	perm os.FileMode
}

func (h *Handler) repoConfigFile() (configFile, error) {
	if h.sddDir == "" {
		return configFile{}, fmt.Errorf("not inside an sdd repo — this needs .sdd/ (run `sdd init` first)")
	}
	return configFile{path: filepath.Join(h.sddDir, "config.yaml"), perm: 0o644}, nil
}

func (h *Handler) localConfigFile() (configFile, error) {
	if h.sddDir == "" {
		return configFile{}, fmt.Errorf("not inside an sdd repo — `--local` needs .sdd/ (run `sdd init` first)")
	}
	return configFile{path: filepath.Join(h.sddDir, "config.local.yaml"), perm: 0o600}, nil
}

func (h *Handler) globalConfigFile() (configFile, error) {
	if h.repos == nil {
		return configFile{}, fmt.Errorf("no global config support configured")
	}
	return configFile{path: h.repos.Registry().ConfigPath(), perm: 0o600}, nil
}

// read returns nil bytes for a missing file, which the patchers take as an
// empty document to create.
func (f configFile) read() ([]byte, error) {
	data, err := os.ReadFile(f.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading %s: %w", f.path, err)
	}
	return data, nil
}

// patch rewrites the document through mutate. Returning nil bytes from mutate
// means nothing changed, and nothing is written.
func (f configFile) patch(mutate func(existing []byte) ([]byte, error)) error {
	existing, err := f.read()
	if err != nil {
		return err
	}
	patched, err := mutate(existing)
	if err != nil {
		return err
	}
	if patched == nil {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(f.path), 0o700); err != nil {
		return fmt.Errorf("creating config dir for %s: %w", f.path, err)
	}
	if err := os.WriteFile(f.path, patched, f.perm); err != nil {
		return fmt.Errorf("writing %s: %w", f.path, err)
	}
	return nil
}
