//go:build !windows

package signerservice

import (
	"os"
	"strings"
	"testing"
	"time"
)

// noSysInfo is a FileInfo whose Sys() is not a *syscall.Stat_t.
type noSysInfo struct{}

func (noSysInfo) Name() string       { return "x" }
func (noSysInfo) Size() int64        { return 0 }
func (noSysInfo) Mode() os.FileMode  { return os.ModeDir | 0o700 }
func (noSysInfo) ModTime() time.Time { return time.Time{} }
func (noSysInfo) IsDir() bool        { return true }
func (noSysInfo) Sys() any           { return nil }

func TestLauncherDirOwnerUID_ReportsTheOwnerOfARealDir(t *testing.T) {
	fi, err := os.Lstat(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	uid, ok := launcherDirOwnerUID(fi)
	if !ok || uid != currentUID() {
		t.Fatalf("launcherDirOwnerUID = %d, %v; want %d, true", uid, ok, currentUID())
	}
}

func TestLauncherDirOwnerUID_NoOwnerWhenSysIsNotStatT(t *testing.T) {
	if uid, ok := launcherDirOwnerUID(noSysInfo{}); ok {
		t.Fatalf("launcherDirOwnerUID = %d, true; want ok=false", uid)
	}
}

// "/" is owned by root, so for any other user checkLauncherDir must refuse it.
func TestCheckLauncherDir_RefusesADirOwnedByAnotherUser(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("running as root: / is owned by the current user")
	}
	err := checkLauncherDir("/")
	if err == nil || !strings.Contains(err.Error(), "owned by uid 0") {
		t.Fatalf("checkLauncherDir(/) = %v; want an 'owned by uid 0' refusal", err)
	}
}
