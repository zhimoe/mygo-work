package pi

import (
	"context"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"mygo-work/internal/process"
)

// command runs pi from PATH. Availability is decided by running it: if
// `pi --version` produces a version, Pi works. On Windows pi is a .cmd
// launcher, so cmd.exe starts it and the launcher finds node itself.
func command(ctx context.Context, args ...string) *exec.Cmd {
	name := strings.Trim(strings.TrimSpace(os.Getenv("MYGO_WORK_PI")), `"'`)
	if name == "" {
		name = "pi"
	}
	if runtime.GOOS == "windows" {
		shell := strings.TrimSpace(os.Getenv("COMSPEC"))
		if shell == "" {
			shell = "cmd.exe"
		}
		cmd := process.Command(ctx, shell)
		process.SetCmdLine(cmd, windowsProcessCommandLine(shell, name, args))
		return cmd
	}
	cmd := process.Command(ctx, name, args...)
	if runtime.GOOS == "darwin" {
		cmd.Env = append(os.Environ(), "PATH="+os.Getenv("PATH")+":/opt/homebrew/bin:/usr/local/bin")
	}
	return cmd
}

func windowsProcessCommandLine(shell, name string, args []string) string {
	return quoteCmd(shell) + " /d /s /c " + windowsCommandLine(name, args)
}

// windowsCommandLine leaves a bare command name for cmd.exe to resolve from
// PATH and quotes names with spaces. The outer quote pair works with cmd /s.
func windowsCommandLine(name string, args []string) string {
	if strings.ContainsAny(name, " \t") {
		name = quoteCmd(name)
	}
	line := name
	for _, arg := range args {
		line += " " + quoteCmd(arg)
	}
	return `"` + line + `"`
}

func quoteCmd(s string) string {
	return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
}
