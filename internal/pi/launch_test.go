package pi

import "testing"

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

func TestWindowsProcessCommandLine(t *testing.T) {
	shell := `C:\Windows\system32\cmd.exe`
	cases := []struct {
		name string
		args []string
		want string
	}{
		{"pi", []string{"--version"}, `"C:\Windows\system32\cmd.exe" /d /s /c "pi "--version""`},
		{`C:\Program Files\Pi\pi.cmd`, []string{"--version"}, `"C:\Windows\system32\cmd.exe" /d /s /c ""C:\Program Files\Pi\pi.cmd" "--version""`},
	}
	for _, c := range cases {
		if got := windowsProcessCommandLine(shell, c.name, c.args); got != c.want {
			t.Errorf("windowsProcessCommandLine(%q, %q) = %q, want %q", c.name, c.args, got, c.want)
		}
	}
}

func TestWindowsCommandLine(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want string
	}{
		{"pi", []string{"--version"}, `"pi "--version""`},
		{`C:\Program Files\Pi\pi.cmd`, []string{"--version"}, `""C:\Program Files\Pi\pi.cmd" "--version""`},
		{"pi", []string{"--mode", "rpc", "--session-dir", `C:\work dir`}, `"pi "--mode" "rpc" "--session-dir" "C:\work dir""`},
	}
	for _, c := range cases {
		if got := windowsCommandLine(c.name, c.args); got != c.want {
			t.Errorf("windowsCommandLine(%q, %q) = %q, want %q", c.name, c.args, got, c.want)
		}
	}
}
