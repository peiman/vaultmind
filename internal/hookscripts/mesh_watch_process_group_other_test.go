//go:build !unix

package hookscripts

import "os/exec"

func setWatchProcessGroup(_ *exec.Cmd) {}

func killWatchProcessGroup(_ int) {}
