package pi

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"mygo-work/internal/process"
)

type launcher struct {
	exe    string
	script string
}

func executable() (launcher, error) {
	home, _ := os.UserHomeDir()
	dirs := []string{"pi", filepath.Join(home, ".local", "bin", "pi"), filepath.Join(home, ".pi", "agent", "bin", "pi")}
	if runtime.GOOS == "windows" {
		dirs = append(dirs, filepath.Join(os.Getenv("APPDATA"), "npm", "pi.cmd"))
	} else {
		dirs = append(dirs, "/opt/homebrew/bin/pi", "/usr/local/bin/pi")
	}
	if override := os.Getenv("MYGO_WORK_PI"); override != "" {
		dirs = []string{override}
	}
	for _, candidate := range dirs {
		if path, err := exec.LookPath(candidate); err == nil {
			if runtime.GOOS != "windows" || strings.EqualFold(filepath.Ext(path), ".exe") {
				return launcher{exe: path}, nil
			}
			if found, ok := nodeLauncher(path); ok {
				return found, nil
			}
		}
	}
	return launcher{}, errors.New("Pi executable unavailable. Install Pi Coding Agent or set MYGO_WORK_PI")
}

// Resolve the launchers produced by the Pi installer and npm. Launch Node
// directly so workspace paths never pass through the Windows command parser.
func nodeLauncher(path string) (launcher, bool) {
	if ext := strings.ToLower(filepath.Ext(path)); ext != ".cmd" && ext != ".ps1" {
		return launcher{}, false
	}
	bin := filepath.Dir(path)
	for _, script := range []string{
		filepath.Join(bin, "pi-launcher.js"),
		filepath.Join(bin, "node_modules", "@earendil-works", "pi-coding-agent", "dist", "bundle", "cli.js"),
	} {
		if info, err := os.Stat(script); err != nil || !info.Mode().IsRegular() {
			continue
		}
		for _, candidate := range []string{filepath.Join(bin, "node.exe"), filepath.Join(os.Getenv("LOCALAPPDATA"), "pi-node", "current", "node.exe"), "node.exe"} {
			if node, err := exec.LookPath(candidate); err == nil {
				return launcher{exe: node, script: script}, true
			}
		}
	}
	return launcher{}, false
}

func (l launcher) command(ctx context.Context, args ...string) *exec.Cmd {
	if l.script != "" {
		args = append([]string{l.script}, args...)
	}
	cmd := process.Command(ctx, l.exe, args...)
	if runtime.GOOS == "darwin" {
		cmd.Env = append(os.Environ(), "PATH="+os.Getenv("PATH")+":/opt/homebrew/bin:/usr/local/bin")
	}
	return cmd
}
