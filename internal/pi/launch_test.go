package pi

import (
	"context"
	"testing"
)

func TestQuoteCmd(t *testing.T) {
	cases := []struct{ in, want string }{
		{`pi`, `"pi"`},
		{`C:\Program Files\Pi\pi.cmd`, `"C:\Program Files\Pi\pi.cmd"`},
		{`--session-dir`, `"--session-dir"`},
		{`a"b`, `"a""b"`},
	}
	for _, c := range cases {
		if got := quoteCmd(c.in); got != c.want {
			t.Errorf("quoteCmd(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestIsExecutable(t *testing.T) {
	cases := map[string]bool{
		`pi.exe`: true,
		`pi.COM`: true,
		`pi.cmd`: false,
		`pi.ps1`: false,
		`pi`:     false,
	}
	for path, want := range cases {
		if got := isExecutable(path); got != want {
			t.Errorf("isExecutable(%q) = %v, want %v", path, got, want)
		}
	}
}

func TestShellCommand(t *testing.T) {
	l := launcher{exe: "cmd.exe", target: `C:\Program Files\Pi\pi.cmd`, shell: true}
	cmd := l.command(context.Background(), "--version")
	want := []string{"cmd.exe", "/d", "/s", "/c", `""C:\Program Files\Pi\pi.cmd" "--version""`}
	if len(cmd.Args) != len(want) {
		t.Fatalf("args = %q, want %q", cmd.Args, want)
	}
	for i := range want {
		if cmd.Args[i] != want[i] {
			t.Fatalf("args = %q, want %q", cmd.Args, want)
		}
	}
}
