//go:build unix

package hookscripts

import (
	"os/exec"
	"syscall"
)

func setWatchProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// killWatchProcessGroup reaps every process the watcher started, even children
// that outlive the leader when a test fails before the watcher exits.
func killWatchProcessGroup(pid int) {
	_ = syscall.Kill(-pid, syscall.SIGKILL)
}
