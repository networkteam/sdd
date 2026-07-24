//go:build windows

package local

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/sys/windows"
)

func windowsDirectorySyncOpenParameters() (access, share, creation, flags uint32) {
	return windows.GENERIC_WRITE,
		windows.FILE_SHARE_READ | windows.FILE_SHARE_WRITE | windows.FILE_SHARE_DELETE,
		windows.OPEN_EXISTING,
		windows.FILE_FLAG_BACKUP_SEMANTICS
}

func syncRootDirectory(root *os.Root, relative string) error {
	if relative == "" {
		relative = "."
	}
	if !filepath.IsLocal(relative) {
		return fmt.Errorf("directory sync path is not local: %s", relative)
	}
	before, err := root.Lstat(relative)
	if err != nil {
		return err
	}
	if !before.IsDir() {
		return fmt.Errorf("directory sync target is not a directory: %s", relative)
	}
	absolute := filepath.Join(root.Name(), relative)
	path, err := windows.UTF16PtrFromString(absolute)
	if err != nil {
		return err
	}
	access, share, creation, flags := windowsDirectorySyncOpenParameters()
	handle, err := windows.CreateFile(path, access, share, nil, creation, flags, 0)
	if err != nil {
		return err
	}
	directory := os.NewFile(uintptr(handle), absolute)
	if directory == nil {
		_ = windows.CloseHandle(handle)
		return fmt.Errorf("wrapping directory sync handle")
	}
	opened, statErr := directory.Stat()
	after, nameErr := root.Lstat(relative)
	if err := errors.Join(statErr, nameErr); err != nil {
		return errors.Join(err, directory.Close())
	}
	if !opened.IsDir() || !after.IsDir() ||
		!os.SameFile(before, opened) || !os.SameFile(opened, after) {
		return errors.Join(
			fmt.Errorf("directory sync target changed while opening: %s", relative),
			directory.Close(),
		)
	}
	return errors.Join(windows.FlushFileBuffers(handle), directory.Close())
}
