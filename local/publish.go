package local

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// publishBytes installs one payload durably: write to a temporary name, flush
// it, rename it into place, then flush the directory. An interrupted publish
// leaves either the previous content or a stray temporary file — never a
// half-written payload — which is the whole durability guarantee this store
// needs now that nothing has to be moved or rolled back.
func publishBytes(root *os.Root, name string, data []byte) error {
	directory := filepath.Dir(name)
	temporary, err := temporaryName(directory)
	if err != nil {
		return err
	}
	file, err := root.OpenFile(temporary, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	if _, err := file.Write(data); err != nil {
		return errors.Join(err, file.Close(), root.Remove(temporary))
	}
	if err := errors.Join(file.Sync(), file.Close()); err != nil {
		return errors.Join(err, root.Remove(temporary))
	}
	if err := root.Rename(temporary, name); err != nil {
		return errors.Join(err, root.Remove(temporary))
	}
	return syncRootDir(root, directory)
}

func publishJSON(root *os.Root, name string, value any) error {
	encoded, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return publishBytes(root, name, encoded)
}

// writeJSONAtomic is the path-addressed form, for the graph store which sits
// outside this subsystem's containment root.
func writeJSONAtomic(filename string, value any) error {
	encoded, err := json.Marshal(value)
	if err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(filename), ".sdd-publish-*")
	if err != nil {
		return err
	}
	name := temporary.Name()
	defer func() { _ = os.Remove(name) }()
	if _, err := temporary.Write(encoded); err != nil {
		return errors.Join(err, temporary.Close())
	}
	if err := errors.Join(temporary.Sync(), temporary.Close()); err != nil {
		return err
	}
	if err := os.Rename(name, filename); err != nil {
		return err
	}
	return syncDir(filepath.Dir(filename))
}

func temporaryName(directory string) (string, error) {
	raw := make([]byte, 8)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return filepath.Join(directory, ".sdd-publish-"+hex.EncodeToString(raw)), nil
}

func syncRootDir(root *os.Root, directory string) error {
	handle, err := root.Open(directory)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("sdd: publishing into missing directory %s: %w", directory, err)
		}
		return err
	}
	if err := handle.Sync(); err != nil {
		return errors.Join(err, handle.Close())
	}
	return handle.Close()
}
