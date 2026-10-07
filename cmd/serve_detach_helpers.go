//go:build !windows

package cmd

import (
	"os/exec"
	"syscall"
)

// serveSupported: the warm server listens on a Unix socket.
const serveSupported = true

// detach puts the server in its own session, so it is not stopped with the
// terminal or hook that started the ask.
func detach(c *exec.Cmd) {
	c.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}
