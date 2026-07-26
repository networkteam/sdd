//go:build windows

package local

import (
	"errors"
	"os"

	"golang.org/x/sys/windows"
)

func tryPinnedFileLock(file *os.File) (*pinnedFileLock, error) {
	var overlapped windows.Overlapped
	err := windows.LockFileEx(
		windows.Handle(file.Fd()),
		windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY,
		0, 1, 0, &overlapped,
	)
	if errors.Is(err, windows.ERROR_LOCK_VIOLATION) {
		return &pinnedFileLock{file: file}, nil
	}
	if err != nil {
		return nil, err
	}
	return &pinnedFileLock{file: file, locked: true}, nil
}

func lockPinnedFile(file *os.File) (*pinnedFileLock, error) {
	var overlapped windows.Overlapped
	if err := windows.LockFileEx(
		windows.Handle(file.Fd()), windows.LOCKFILE_EXCLUSIVE_LOCK,
		0, 1, 0, &overlapped,
	); err != nil {
		return nil, err
	}
	return &pinnedFileLock{file: file, locked: true}, nil
}

func unlockPinnedFile(file *os.File) error {
	var overlapped windows.Overlapped
	return windows.UnlockFileEx(windows.Handle(file.Fd()), 0, 1, 0, &overlapped)
}
