//go:build !windows

package chromium

import "os/exec"

func hideWindow(cmd *exec.Cmd) {}
