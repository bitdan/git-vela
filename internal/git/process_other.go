//go:build !windows

package git

import "os/exec"

func configureProcess(_ *exec.Cmd) {}
