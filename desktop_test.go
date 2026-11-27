package main

import (
	"context"
	"errors"
	"testing"

	"mygo-work/internal/agent"
)

// stubRuntime only implements Version; the desktop default check does not
// call any other Runtime method.
type stubRuntime struct {
	agent.Runtime
	version string
	err     error
}

func (s stubRuntime) Version(context.Context) (string, error) { return s.version, s.err }

func TestDefaultRuntimePrefersPi(t *testing.T) {
	d := &Desktop{runtimes: map[string]agent.Runtime{
		piID:       stubRuntime{version: "1.6.0"},
		openCodeID: stubRuntime{version: "2.3.2"},
	}}
	if got := d.defaultRuntime(context.Background()); got != piID {
		t.Fatalf("defaultRuntime() = %q, want %q", got, piID)
	}
}

func TestDefaultRuntimeFallsBackWhenPiUnavailable(t *testing.T) {
	cases := map[string]map[string]agent.Runtime{
		"version error": {
			piID:       stubRuntime{err: errors.New("pi is missing")},
			openCodeID: stubRuntime{version: "2.3.2"},
		},
		"runtime not registered": {
			openCodeID: stubRuntime{version: "2.3.2"},
		},
	}
	for name, runtimes := range cases {
		t.Run(name, func(t *testing.T) {
			d := &Desktop{runtimes: runtimes}
			if got := d.defaultRuntime(context.Background()); got != openCodeID {
				t.Fatalf("defaultRuntime() = %q, want %q", got, openCodeID)
			}
		})
	}
}
