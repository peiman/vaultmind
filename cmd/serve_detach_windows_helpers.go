//go:build windows

package cmd

import "os/exec"

// serveSupported: on Windows every command runs in its own process; the warm
// server is not offered there.
const serveSupported = false

func detach(*exec.Cmd) {}
