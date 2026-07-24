//go:build !windows

package local

import (
	"errors"
	"os"
)

func syncRootDirectory(root *os.Root, relative string) error {
	if relative == "" {
		relative = "."
	}
	directory, err := root.Open(relative)
	if err != nil {
		return err
	}
	return errors.Join(directory.Sync(), directory.Close())
}
