//go:build !windows

package process

import (
	"context"
	"os/exec"
	"syscall"
)

func Command(ctx context.Context, binary string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, binary, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
	return cmd
}

// SetCmdLine is a no-op outside Windows.
func SetCmdLine(*exec.Cmd, string) {}
