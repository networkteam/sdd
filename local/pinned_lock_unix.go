//go:build !windows

package local

import (
	"errors"
	"os"

	"golang.org/x/sys/unix"
)

func tryPinnedFileLock(file *os.File) (*pinnedFileLock, error) {
	err := unix.Flock(int(file.Fd()), unix.LOCK_EX|unix.LOCK_NB)
	if errors.Is(err, unix.EWOULDBLOCK) {
		return &pinnedFileLock{file: file}, nil
	}
	if err != nil {
		return nil, err
	}
	return &pinnedFileLock{file: file, locked: true}, nil
}

func lockPinnedFile(file *os.File) (*pinnedFileLock, error) {
	if err := unix.Flock(int(file.Fd()), unix.LOCK_EX); err != nil {
		return nil, err
	}
	return &pinnedFileLock{file: file, locked: true}, nil
}

func unlockPinnedFile(file *os.File) error {
	return unix.Flock(int(file.Fd()), unix.LOCK_UN)
}
