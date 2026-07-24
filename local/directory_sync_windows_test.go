//go:build windows

package local

import (
	"testing"

	"golang.org/x/sys/windows"
)

func TestWindowsDirectorySyncUsesWriteCapableSharedDirectoryHandle(t *testing.T) {
	access, share, creation, flags := windowsDirectorySyncOpenParameters()
	if access&windows.GENERIC_WRITE == 0 {
		t.Fatalf("directory sync access %#x is not write-capable", access)
	}
	wantShare := uint32(windows.FILE_SHARE_READ | windows.FILE_SHARE_WRITE | windows.FILE_SHARE_DELETE)
	if share != wantShare {
		t.Fatalf("directory sync share = %#x, want %#x", share, wantShare)
	}
	if creation != windows.OPEN_EXISTING {
		t.Fatalf("directory sync creation = %#x, want OPEN_EXISTING", creation)
	}
	if flags&windows.FILE_FLAG_BACKUP_SEMANTICS == 0 {
		t.Fatalf("directory sync flags %#x lack FILE_FLAG_BACKUP_SEMANTICS", flags)
	}
}
