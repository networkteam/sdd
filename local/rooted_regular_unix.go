//go:build !windows

package local

import (
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

func openRootedRegular(root *os.Root, name string) (*os.File, error) {
	before, err := root.Lstat(name)
	if err != nil {
		return nil, err
	}
	if !before.Mode().IsRegular() {
		return nil, fmt.Errorf("rooted input %s is not a regular file", name)
	}
	runRootedRegularOpenHook(name)
	file, err := root.OpenFile(name, os.O_RDONLY|unix.O_NONBLOCK|unix.O_NOFOLLOW, 0)
	if err != nil {
		return nil, err
	}
	opened, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, err
	}
	after, err := root.Lstat(name)
	if err != nil || !opened.Mode().IsRegular() || !after.Mode().IsRegular() ||
		!os.SameFile(opened, after) {
		_ = file.Close()
		if err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("rooted input %s changed while opening or is not regular", name)
	}
	return file, nil
}
