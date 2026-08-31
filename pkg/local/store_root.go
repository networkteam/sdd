package local

import (
	"errors"
	"fmt"
	"os"
)

// openStoreRoot is the whole of path containment: one os.Root at the store
// directory, so a defect in key derivation or location resolution cannot write
// outside it. Symlink-rebinding hardening is deliberately absent — per
// 20260730-093011-d-tac-k4q its threat model needs an attacker who already has
// write access to the user's own state directory, where containment defends
// nothing.
func openStoreRoot(dir string, create bool) (*os.Root, error) {
	if dir == "" {
		return nil, fmt.Errorf("sdd: store directory is required")
	}
	if create {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return nil, fmt.Errorf("sdd: creating store directory %s: %w", dir, err)
		}
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, err
	}
	return root, nil
}

// syncDir flushes a directory entry so a rename is durable.
func syncDir(dir string) error {
	handle, err := os.Open(dir)
	if err != nil {
		return err
	}
	if err := handle.Sync(); err != nil {
		return errors.Join(err, handle.Close())
	}
	return handle.Close()
}
