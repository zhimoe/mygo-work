package pi

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"mygo-work/internal/process"
)

type launcher struct {
	exe    string
	target string // target of cmd.exe when shell is set
	shell  bool
}

// executable uses MYGO_WORK_PI when it is set, otherwise pi from PATH. On
// Windows a .cmd or .ps1 launcher runs through cmd.exe; node is not checked
// separately: the launcher itself reports whether it can start.
func executable() (launcher, error) {
	command := "pi"
	override := strings.Trim(strings.TrimSpace(os.Getenv("MYGO_WORK_PI")), `"'`)
	if override != "" {
		command = override
	}
	path, err := exec.LookPath(command)
	if err == nil {
		if runtime.GOOS == "windows" && !isExecutable(path) {
			shell, err := commandProcessor()
			if err != nil {
				return launcher{}, err
			}
			return launcher{exe: shell, target: path, shell: true}, nil
		}
		return launcher{exe: path}, nil
	}
	if runtime.GOOS != "windows" && override == "" {
		home, _ := os.UserHomeDir()
		for _, candidate := range []string{
			filepath.Join(home, ".local", "bin", "pi"),
			filepath.Join(home, ".pi", "agent", "bin", "pi"),
			"/opt/homebrew/bin/pi",
			"/usr/local/bin/pi",
		} {
			if path, err := exec.LookPath(candidate); err == nil {
				return launcher{exe: path}, nil
			}
		}
	}
	return launcher{}, fmt.Errorf("Pi executable unavailable: %v", err)
}

func isExecutable(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".exe", ".com":
		return true
	}
	return false
}

func commandProcessor() (string, error) {
	if comspec := strings.TrimSpace(os.Getenv("COMSPEC")); comspec != "" {
		if path, err := exec.LookPath(comspec); err == nil {
			return path, nil
		}
	}
	path, err := exec.LookPath("cmd.exe")
	if err != nil {
		return "", errors.New("cmd.exe is unavailable")
	}
	return path, nil
}

func (l launcher) command(ctx context.Context, args ...string) *exec.Cmd {
	if l.shell {
		line := "\"" + quoteCmd(l.target)
		for _, arg := range args {
			line += " " + quoteCmd(arg)
		}
		line += "\""
		return process.Command(ctx, l.exe, "/d", "/s", "/c", line)
	}
	cmd := process.Command(ctx, l.exe, args...)
	if runtime.GOOS == "darwin" {
		cmd.Env = append(os.Environ(), "PATH="+os.Getenv("PATH")+":/opt/homebrew/bin:/usr/local/bin")
	}
	return cmd
}

func quoteCmd(s string) string {
	return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
}
