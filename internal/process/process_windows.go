package process

import (
	"context"
	"os/exec"
	"syscall"

	"golang.org/x/sys/windows"
)

func Command(ctx context.Context, binary string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, binary, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_NO_WINDOW}
	return cmd
}

// SetCmdLine passes line to CreateProcess verbatim. cmd.exe parses its own
// quoting, so rebuilding the line from escaped arguments breaks .cmd names.
func SetCmdLine(cmd *exec.Cmd, line string) {
	cmd.SysProcAttr.CmdLine = line
}
