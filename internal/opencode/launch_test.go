package opencode

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"testing"
)

func TestLauncherJSCommand(t *testing.T) {
	l := launcher{exe: "node", script: "/tmp/opencode.mjs"}
	cmd := l.command(context.Background(), "--version")
	want := []string{"node", "--experimental-ffi", "--use-system-ca", "--disable-warning=ExperimentalWarning", "/tmp/opencode.mjs", "--version"}
	if !slices.Equal(cmd.Args, want) {
		t.Fatalf("args = %q, want %q", cmd.Args, want)
	}
}

func TestShellCommandLine(t *testing.T) {
	got := shellCommandLine("cmd.exe", `C:\Program Files\OpenCode\opencode.cmd`, []string{"--version"})
	want := `"cmd.exe" /d /s /c ""C:\Program Files\OpenCode\opencode.cmd" "--version""`
	if got != want {
		t.Fatalf("shellCommandLine() = %q, want %q", got, want)
	}
}

func TestResolvePathAllowsNonExecutableJS(t *testing.T) {
	script := filepath.Join(t.TempDir(), "opencode.mjs")
	if err := os.WriteFile(script, []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := resolvePath(script)
	if err != nil || got != script {
		t.Fatalf("resolvePath() = %q, %v; want %q", got, err, script)
	}
}

func TestNewLauncherJS(t *testing.T) {
	t.Setenv("MYGO_WORK_NODE", "")
	dir := t.TempDir()
	nodeName := "node"
	if runtime.GOOS == "windows" {
		nodeName = "node.exe"
	}
	node := filepath.Join(dir, nodeName)
	if err := os.WriteFile(node, []byte(""), 0o755); err != nil {
		t.Fatal(err)
	}
	script := filepath.Join(dir, "opencode.mjs")
	if err := os.WriteFile(script, []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}
	found, err := newLauncher(script)
	if err != nil {
		t.Fatal(err)
	}
	if found.exe != node || found.script != script || found.shell {
		t.Fatalf("launcher = %+v, want node %q script %q", found, node, script)
	}
}

func TestLauncherCandidatesOverride(t *testing.T) {
	t.Setenv("MYGO_WORK_OPENCODE", `"C:\Tools\opencode.mjs"`)
	got := launcherCandidates(t.TempDir())
	want := []string{`C:\Tools\opencode.mjs`}
	if !slices.Equal(got, want) {
		t.Fatalf("candidates = %q, want %q", got, want)
	}
}

func TestLauncherCandidatesUsePath(t *testing.T) {
	t.Setenv("MYGO_WORK_OPENCODE", "")
	got := launcherCandidates(t.TempDir())
	for _, name := range []string{"opencode", "opencode2"} {
		if !slices.Contains(got, name) {
			t.Fatalf("candidates = %q, missing PATH command %q", got, name)
		}
	}
}
