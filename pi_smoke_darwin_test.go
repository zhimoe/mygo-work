package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"mygo-work/internal/workspace"
)

func TestMacOSPiDesktop(t *testing.T) {
	pi := os.Getenv("MYGO_WORK_TEST_PI")
	openCode := os.Getenv("MYGO_WORK_TEST_OPENCODE")
	if pi == "" || openCode == "" {
		t.Skip("set MYGO_WORK_TEST_PI and MYGO_WORK_TEST_OPENCODE")
	}
	root := t.TempDir()
	project, profile := filepath.Join(root, "Work"), filepath.Join(root, "profile")
	piConfig, codeConfig := filepath.Join(root, "pi-config"), filepath.Join(root, "opencode-config")
	for _, dir := range []string{project, filepath.Join(piConfig, "extensions"), codeConfig} {
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
	}
	store, err := workspace.Open(profile)
	if err != nil {
		t.Fatal(err)
	}
	folder, err := store.Add(project, openCodeID)
	if err != nil {
		t.Fatal(err)
	}
	project = folder.Directory
	if err := os.WriteFile(filepath.Join(profile, "language"), []byte("en\n"), 0600); err != nil {
		t.Fatal(err)
	}
	inference := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		fixtureModel(w, req, project)
	}))
	defer inference.Close()
	models := fmt.Sprintf(`{
		"providers": {
			"fixture": {
				"baseUrl": "%s/v1",
				"api": "openai-completions",
				"apiKey": "fake-local-only",
				"models": [
					{"id": "smoke", "name": "Pi Fixture", "input": ["text"]},
					{"id": "alternate", "name": "Pi Alternate", "input": ["text"]}
				]
			}
		}
	}`, inference.URL)
	if err := os.WriteFile(filepath.Join(piConfig, "models.json"), []byte(models), 0600); err != nil {
		t.Fatal(err)
	}

	extension := `import { writeFile } from "node:fs/promises";
import { join } from "node:path";
export default function (pi) {
  pi.registerCommand("mygo-confirm", {
    description: "Ask the desktop fixture for confirmation",
    async handler(_args, ctx) {
      const confirmed = await ctx.ui.confirm("Continue fixture task?", "Verify the desktop confirmation.");
      await writeFile(join(ctx.cwd, "pi-extension.answer"), String(confirmed));
    }
  });
}`
	if err := os.WriteFile(filepath.Join(piConfig, "extensions", "smoke.ts"), []byte(extension), 0600); err != nil {
		t.Fatal(err)
	}

	for _, stage := range []string{"pi-run", "pi-restore"} {
		ctx, cancel := context.WithTimeout(t.Context(), 45*time.Second)
		cmd := exec.CommandContext(ctx, os.Args[0])
		cmd.Env = append(cmd.Environ(),
			"MYGO_WORK_SMOKE_STAGE="+stage, "MYGO_WORK_DATA_DIR="+profile, "MYGO_WORK_SMOKE_PROJECT="+project,
			"MYGO_WORK_OPENCODE="+openCode, "XDG_CONFIG_HOME="+codeConfig,
			"MYGO_WORK_PI="+pi, "PI_CODING_AGENT_DIR="+piConfig, "PI_OFFLINE=1", "PI_TELEMETRY=0",
		)
		cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
		err := cmd.Run()
		cancel()
		if err != nil {
			t.Fatalf("native %s: %v", stage, err)
		}
	}
}

func nativePiJourney(stage string, await func(string, string) error, eval, click, send func(string) error) error {
	switchEngine := func(name string) error {
		if err := click("Workspace actions"); err != nil {
			return err
		}
		if err := click("Coding engine"); err != nil {
			return err
		}
		return click(name)
	}
	const ready = `document.querySelector(".footer")?.innerText.includes("Ready for new tasks")`
	const piView = `document.querySelector(".composer .engine-label")?.textContent === "Pi"`
	if stage == "pi-run" {
		if err := await("OpenCode workspace loaded before engine switch", ready+` && document.querySelector(".folder-row")`); err != nil {
			return err
		}
		if err := switchEngine("Pi Coding Agent"); err != nil {
			return err
		}
		if err := await("Pi workspace ready with models from Pi", ready+" && "+piView+` && document.querySelector(".composer .model")?.textContent.includes("Pi ")`); err != nil {
			return err
		}
		if err := eval(`document.querySelector(".composer .model").click()`); err != nil {
			return err
		}
		if err := eval(`[...document.querySelectorAll("dialog .picker-item")].find(button => button.firstElementChild.textContent === "Pi Alternate").click()`); err != nil {
			return err
		}
		if err := send("Smoke write"); err != nil {
			return err
		}
		if err := await("Pi executed a tool and rendered its result", `document.querySelector(".transcript")?.innerText.includes("Smoke file created") && document.querySelector(".transcript .tool summary")?.innerText.includes("completed")`); err != nil {
			return err
		}
		if output, err := os.ReadFile(filepath.Join(os.Getenv("MYGO_WORK_SMOKE_PROJECT"), "mygo-smoke.txt")); err != nil || string(output) != "Hello from the native smoke test.\n" {
			return fmt.Errorf("unexpected Pi tool file %q: %v", output, err)
		}
		if err := await("Pi selected model persisted in task", ready+` && document.querySelector(".composer .model")?.textContent.includes("Pi Alternate")`); err != nil {
			return err
		}
		if err := send("Smoke stop"); err != nil {
			return err
		}
		if err := await("Pi partial output arrived while the fixture was still streaming", `document.querySelector(".transcript")?.innerText.includes("Streaming in progress") && document.querySelector(".footer")?.innerText.includes("Task in progress")`); err != nil {
			return err
		}
		if err := click("Stop"); err != nil {
			return err
		}
		if err := await("Pi abort settled the run", ready); err != nil {
			return err
		}
		if err := click("New task"); err != nil {
			return err
		}
		if err := send("/mygo-confirm"); err != nil {
			return err
		}
		if err := await("Pi extension request visible before prompt acknowledgement", `document.querySelector(".callout form")?.innerText.includes("Continue fixture task?")`); err != nil {
			return err
		}
		if err := eval(`document.querySelector(".callout select").value = "true"`); err != nil {
			return err
		}
		if err := click("Submit"); err != nil {
			return err
		}
		if err := await("Pi extension confirmation returned", ready+` && !document.querySelector(".callout form")`); err != nil {
			return err
		}
		if output, err := os.ReadFile(filepath.Join(os.Getenv("MYGO_WORK_SMOKE_PROJECT"), "pi-extension.answer")); err != nil || string(output) != "true" {
			return fmt.Errorf("unexpected extension response %q: %v", output, err)
		}
		return click("Smoke write")
	}
	if err := await("Pi workspace and task restored after relaunch", ready+" && "+piView+` && document.querySelector(".transcript")?.innerText.includes("Smoke file created")`); err != nil {
		return err
	}
	if err := switchEngine("OpenCode v2"); err != nil {
		return err
	}
	if err := await("OpenCode shows its own task list", ready+" && !("+piView+`) && !document.querySelector(".session-row")`); err != nil {
		return err
	}
	if err := switchEngine("Pi Coding Agent"); err != nil {
		return err
	}
	return await("returning to Pi preserves the separate session list and transcript", ready+" && "+piView+` && document.querySelector(".transcript")?.innerText.includes("Smoke file created")`)
}
