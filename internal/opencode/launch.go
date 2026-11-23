package opencode

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

// launcher starts OpenCode: a native executable, Node running a JavaScript
// entrypoint, or cmd.exe running a .cmd/.ps1 shim.
type launcher struct {
	exe    string
	script string
	shell  bool
	target string
}

var (
	jsExtensions   = map[string]bool{".js": true, ".mjs": true, ".cjs": true}
	shimExtensions = map[string]bool{".cmd": true, ".bat": true, ".ps1": true}
	nodeFlags      = []string{"--experimental-ffi", "--use-system-ca", "--disable-warning=ExperimentalWarning"}
)

// resolveLauncher prefers MYGO_WORK_OPENCODE, then a JavaScript bundle or
// native opencode executable next to the app, in the data directory, or on
// PATH. Native opencode.exe/opencode remains supported. Any executable that
// accepts --version is accepted; the version is not checked.
func resolveLauncher(ctx context.Context, dir string) (launcher, error) {
	var firstErr error
	for _, candidate := range launcherCandidates(dir) {
		if candidate == "" {
			continue
		}
		path, err := resolvePath(candidate)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		found, err := newLauncher(path)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		if _, err := found.command(ctx, "--version").Output(); err != nil {
			if firstErr == nil {
				firstErr = fmt.Errorf("%s --version: %w", path, err)
			}
			continue
		}
		return found, nil
	}
	if firstErr == nil {
		firstErr = errors.New("no OpenCode executable found")
	}
	return launcher{}, fmt.Errorf("OpenCode executable unavailable: %w. Install OpenCode or set MYGO_WORK_OPENCODE", firstErr)
}

// resolvePath accepts a plain file path without requiring the executable bit:
// a .mjs bundle is run by node, not executed directly.
func resolvePath(candidate string) (string, error) {
	if strings.ContainsAny(candidate, `/\`) {
		info, err := os.Stat(candidate)
		if err != nil {
			return "", err
		}
		if info.IsDir() {
			return "", fmt.Errorf("%s is a directory", candidate)
		}
		return filepath.Abs(candidate)
	}
	return exec.LookPath(candidate)
}

func launcherCandidates(dir string) []string {
	if override := strings.Trim(strings.TrimSpace(os.Getenv("MYGO_WORK_OPENCODE")), `"'`); override != "" {
		return []string{override}
	}
	var names []string
	if runtime.GOOS == "windows" {
		names = []string{"opencode.mjs", "opencode.js", "opencode.cjs", "opencode.cmd", "opencode.exe"}
	} else {
		names = []string{"opencode", "opencode.mjs", "opencode.js", "opencode.cjs"}
	}
	var candidates []string
	addFolder := func(folder string) {
		for _, name := range names {
			candidates = append(candidates, filepath.Join(folder, name))
		}
	}
	if exe, err := os.Executable(); err == nil {
		folder := filepath.Dir(exe)
		addFolder(folder)
		addFolder(filepath.Join(folder, "..", "Resources"))
	}
	addFolder(filepath.Join(dir, "bin"))
	// Location does not matter for an external install as long as opencode or
	// opencode2 is on PATH.
	candidates = append(candidates, "opencode2", "opencode")
	return candidates
}

func newLauncher(path string) (launcher, error) {
	ext := strings.ToLower(filepath.Ext(path))
	switch {
	case jsExtensions[ext]:
		node, err := nodeBinary(filepath.Dir(path))
		if err != nil {
			return launcher{}, err
		}
		return launcher{exe: node, script: path}, nil
	case shimExtensions[ext]:
		shell, err := commandProcessor()
		if err != nil {
			return launcher{}, err
		}
		return launcher{exe: shell, target: path, shell: true}, nil
	default:
		return launcher{exe: path}, nil
	}
}

func nodeBinary(dir string) (string, error) {
	if override := strings.Trim(strings.TrimSpace(os.Getenv("MYGO_WORK_NODE")), `"'`); override != "" {
		if path, err := exec.LookPath(override); err == nil {
			return path, nil
		}
	}
	nodeNames := []string{"node", "node.exe"}
	if runtime.GOOS == "windows" {
		nodeNames = []string{"node.exe", "node"}
	}
	for _, name := range nodeNames {
		candidate := filepath.Join(dir, name)
		if info, err := os.Stat(candidate); err == nil && info.Mode().IsRegular() {
			return candidate, nil
		}
	}
	if path, err := exec.LookPath("node"); err == nil {
		return path, nil
	}
	return "", errors.New("node is unavailable; put node on PATH or place node.exe next to opencode.mjs")
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
		cmd := process.Command(ctx, l.exe)
		process.SetCmdLine(cmd, shellCommandLine(l.exe, l.target, args))
		return cmd
	}
	if l.script != "" {
		args = append(append([]string{}, nodeFlags...), append([]string{l.script}, args...)...)
	}
	cmd := process.Command(ctx, l.exe, args...)
	if runtime.GOOS == "darwin" {
		cmd.Env = append(os.Environ(), "PATH="+os.Getenv("PATH")+":/opt/homebrew/bin:/usr/local/bin")
	}
	return cmd
}

// shellCommandLine builds the CreateProcess command line for cmd.exe. The
// inner command is wrapped in quotes so cmd /s does not strip its own quotes.
func shellCommandLine(shell, name string, args []string) string {
	return quoteCmd(shell) + " /d /s /c " + commandLine(name, args)
}

func commandLine(name string, args []string) string {
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
