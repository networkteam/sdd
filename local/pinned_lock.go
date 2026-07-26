package local

import (
	"errors"
	"os"
)

type pinnedFileLock struct {
	file   *os.File
	locked bool
}

func (l *pinnedFileLock) Unlock() error {
	if l == nil || l.file == nil {
		return nil
	}
	var unlockErr error
	if l.locked {
		unlockErr = unlockPinnedFile(l.file)
		l.locked = false
	}
	closeErr := l.file.Close()
	l.file = nil
	return errors.Join(unlockErr, closeErr)
}
