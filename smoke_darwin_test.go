// These checks drive the production UI inside an actual MyGo WKWebView.
package main

import (
	"context"
	"encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo"

	"mygo-work/internal/workspace"
)

func TestMain(m *testing.M) {
	stage := os.Getenv("MYGO_WORK_SMOKE_STAGE")
	if stage == "" {
		os.Exit(m.Run())
	}
	results := make(chan error, 1)
	mygo.App.WhenReady(func() {
		go func() {
			results <- nativeJourney(stage)
			mygo.App.Quit()
		}()
	})
	main()
	if err := <-results; err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func TestMacOSDesktop(t *testing.T) {
	binary := os.Getenv("MYGO_WORK_TEST_OPENCODE")
	if binary == "" {
		t.Skip("set MYGO_WORK_TEST_OPENCODE to the OpenCode v2 executable")
	}
	root := t.TempDir()
	project := filepath.Join(root, "Work")
	profile := filepath.Join(root, "profile")
	config := filepath.Join(root, "config")
	for _, folder := range []string{project, config} {
		if err := os.MkdirAll(folder, 0700); err != nil {
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
	if err := os.WriteFile(filepath.Join(project, "sample.csv"), []byte("name,revenue\nAda,120\nLin,240\n"), 0600); err != nil {
		t.Fatal(err)
	}
	model := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/mcp" {
			fixtureMCP(w, r)
		} else {
			fixtureModel(w, r, project)
		}
	}))
	defer model.Close()
	settings := fmt.Sprintf(`{
	  "model": "fixture/smoke",
	  "small_model": "fixture/smoke",
	  "providers": {"fixture": {
	    "name": "Fixture", "package": "@opencode/ai/providers/openai-compatible",
	    "settings": {"baseURL": "%s/v1", "apiKey": "fake-local-only"},
	    "models": {"smoke": {"name": "Fixture Model", "capabilities": {"tools": true, "input": ["text"], "output": ["text"]}}}
	  }}
	}`, model.URL)
	if err := os.WriteFile(filepath.Join(project, "opencode.json"), []byte(settings), 0600); err != nil {
		t.Fatal(err)
	}

	for _, stage := range []string{"run", "restore"} {
		ctx, cancel := context.WithTimeout(t.Context(), 85*time.Second)
		cmd := exec.CommandContext(ctx, os.Args[0])
		cmd.Env = append(cmd.Environ(),
			"MYGO_WORK_SMOKE_STAGE="+stage,
			"MYGO_WORK_DATA_DIR="+profile,
			"MYGO_WORK_OPENCODE="+binary,
			"MYGO_WORK_SMOKE_PROJECT="+project,
			"MYGO_WORK_SMOKE_MCP="+model.URL+"/mcp",
			"XDG_CONFIG_HOME="+config,
		)
		cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
		err := cmd.Run()
		cancel()
		if err != nil {
			log, _ := os.ReadFile(filepath.Join(profile, "engine", "opencode.log"))
			t.Fatalf("native %s: %v\n%s", stage, err, log)
		}
	}
}

func nativeJourney(stage string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 32*time.Second)
	defer cancel()
	var window *mygo.Window
	for window == nil {
		windows := mygo.Windows()
		if len(windows) > 0 {
			window = windows[0]
			break
		}
		select {
		case <-ctx.Done():
			return errors.New("MyGo did not create a window")
		case <-time.After(50 * time.Millisecond):
		}
	}
	page := window.Page()
	await := func(label, predicate string) error {
		for ctx.Err() == nil {
			result, err := page.EvalContext(ctx, "return Boolean("+predicate+")")
			if err == nil && result == true {
				fmt.Println(label)
				return nil
			}
			select {
			case <-ctx.Done():
			case <-time.After(150 * time.Millisecond):
			}
		}
		debugCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		debug, _ := page.EvalContext(debugCtx, "return document.body.textContent")
		return fmt.Errorf("%s: condition never became true\n%v", label, debug)
	}
	eval := func(code string) error {
		_, err := page.EvalContext(ctx, code)
		return err
	}
	click := func(text string) error {
		value, _ := json.Marshal(text)
		return eval("const name = " + string(value) + "; const button = [...document.querySelectorAll('button')].find(b => b.getClientRects().length && (b.textContent.trim() === name || b.getAttribute('aria-label') === name)); if (!button) throw new Error('Missing button ' + name); button.click()")
	}
	send := func(text string) error {
		value, _ := json.Marshal(text)
		if err := eval("const field = document.querySelector('.composer textarea'); field.value = " + string(value) + "; field.dispatchEvent(new Event('input'));"); err != nil {
			return err
		}
		return click("Run task")
	}
	if strings.HasPrefix(stage, "pi-") {
		return nativePiJourney(stage, await, eval, click, send)
	}
	if stage == "run" {
		if err := await("fresh macOS profile defaults to Simplified Chinese", `document.documentElement.lang === "zh-CN" && document.querySelector(".hero")?.innerText.includes("你想完成什么任务？") && document.querySelector(".footer")?.innerText.includes("就绪")`); err != nil {
			return err
		}
		if mygo.App.Locale() != "zh-Hans" {
			return fmt.Errorf("native MyGo language is %s", mygo.App.Locale())
		}
		if err := click("设置"); err != nil {
			return err
		}
		if err := eval(`const select = document.querySelector("#interface-language"); select.value = "en"; select.dispatchEvent(new Event("change"))`); err != nil {
			return err
		}
		if err := await("switching to English updates settings and the workspace", `document.documentElement.lang === "en" && document.querySelector(".dialog-head")?.textContent.trim() === "Settings" && document.querySelector(".hero")?.innerText.includes("What do you need done?")`); err != nil {
			return err
		}
		if mygo.App.Locale() != "en" {
			return fmt.Errorf("native MyGo language is %s", mygo.App.Locale())
		}
		if err := click("Close"); err != nil {
			return err
		}
	}
	if err := await("native WKWebView loaded in English", `document.documentElement.lang === "en" && document.querySelector(".folder-row") && document.querySelector(".footer").innerText.includes("Ready for new tasks")`); err != nil {
		return err
	}
	if stage == "restore" {
		if err := click("Skills and connections"); err != nil {
			return err
		}
		if err := await("remote MCP connection restored after application restart", `document.querySelector(".right")?.innerText.includes("Smoke MCP") && document.querySelector(".right")?.innerText.includes("connected")`); err != nil {
			return err
		}
		if err := click("Close panel"); err != nil {
			return err
		}
		if err := click("Search sessions"); err != nil {
			return err
		}
		if err := await("session visible after application restart", `document.querySelector(".picker-item")`); err != nil {
			return err
		}
		if err := eval(`document.querySelector(".picker-item").click()`); err != nil {
			return err
		}
		if err := await("conversation restored", `document.querySelector(".transcript")?.innerText.includes("Smoke file created")`); err != nil {
			return err
		}
		if err := click("Settings"); err != nil {
			return err
		}
		if err := eval(`const select = document.querySelector("#interface-language"); select.value = "zh-CN"; select.dispatchEvent(new Event("change"))`); err != nil {
			return err
		}
		return await("switching back to Chinese retains conversation history", `document.documentElement.lang === "zh-CN" && document.querySelector(".dialog-head")?.textContent.trim() === "设置" && document.querySelector(".transcript")?.innerText.includes("Smoke file created")`)
	}
	if err := await("workspace model in composer", `document.querySelector(".composer .model")?.innerText.includes("Fixture Model")`); err != nil {
		return err
	}
	if err := click("Files"); err != nil {
		return err
	}
	if err := await("workspace files listed", `document.querySelector(".right .panel-content")?.innerText.includes("sample.csv")`); err != nil {
		return err
	}
	if err := click("sample.csv"); err != nil {
		return err
	}
	if err := await("CSV rendered inside the file panel", `document.querySelector(".preview-table")?.innerText.includes("Ada")`); err != nil {
		return err
	}
	if directory := os.Getenv("MYGO_WORK_SMOKE_SCREENSHOTS"); directory != "" {
		png, err := window.CapturePage()
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(directory, "workspace.png"), png, 0600); err != nil {
			return err
		}
	}
	if err := click("Edit"); err != nil {
		return err
	}
	if err := eval(`const cell = document.querySelector('input[aria-label="Row 2, column 2"]'); cell.value = "150"; cell.dispatchEvent(new Event("input"))`); err != nil {
		return err
	}
	if err := click("Save"); err != nil {
		return err
	}
	if err := await("CSV edits saved from the file panel", `document.querySelector(".preview-table:not(.editing-table)")?.innerText.includes("150")`); err != nil {
		return err
	}
	if err := click("Close panel"); err != nil {
		return err
	}
	if err := click("Skills and connections"); err != nil {
		return err
	}
	if err := await("skills and connections loaded", `[...document.querySelectorAll(".right button")].some(b => b.textContent.trim() === "Add MCP connection")`); err != nil {
		return err
	}
	if err := click("Add MCP connection"); err != nil {
		return err
	}
	address, _ := json.Marshal(os.Getenv("MYGO_WORK_SMOKE_MCP"))
	if err := eval(`const form = document.querySelector("dialog form"); form.querySelector('input[type="text"]').value = "Smoke MCP"; form.querySelector('input[type="url"]').value = ` + string(address)); err != nil {
		return err
	}
	if err := click("Add connection"); err != nil {
		return err
	}
	if err := await("connected to a remote MCP endpoint", `!document.querySelector("dialog").open && document.querySelector(".right")?.innerText.includes("Smoke MCP") && document.querySelector(".right")?.innerText.includes("connected")`); err != nil {
		return err
	}
	if err := click("Close panel"); err != nil {
		return err
	}
	if err := send("Smoke write"); err != nil {
		return err
	}
	if err := await("permission gate shown", `document.querySelector(".callout")?.innerText.includes("Permission requested · edit")`); err != nil {
		return err
	}
	product := filepath.Join(os.Getenv("MYGO_WORK_SMOKE_PROJECT"), "mygo-smoke.txt")
	if _, err := os.Stat(product); !errors.Is(err, os.ErrNotExist) {
		return errors.New("fixture file appeared before permission was granted")
	}
	if err := click("Allow once"); err != nil {
		return err
	}
	if err := await("assistant answer rendered", `document.querySelector(".transcript")?.innerText.includes("Smoke file created")`); err != nil {
		return err
	}
	got, err := os.ReadFile(product)
	if err != nil || string(got) != "Hello from the native smoke test.\n" {
		return fmt.Errorf("unexpected fixture file %q: %v", got, err)
	}
	if err := await("task settled", `document.querySelector(".footer")?.innerText.includes("Ready for new tasks")`); err != nil {
		return err
	}
	if err := send("Smoke stop"); err != nil {
		return err
	}
	if err := await("partial output appears before model completion", `document.querySelector(".transcript")?.innerText.includes("Streaming in progress") && document.querySelector(".footer")?.innerText.includes("Task in progress")`); err != nil {
		return err
	}
	if err := click("Stop"); err != nil {
		return err
	}
	if err := await("stop interrupts the model", `document.querySelector(".footer")?.innerText.includes("Ready for new tasks")`); err != nil {
		return err
	}
	if err := click("Session actions"); err != nil {
		return err
	}
	if err := click("Archive session"); err != nil {
		return err
	}
	if err := await("archiving returns to the workspace draft", `!document.querySelector("dialog").open && document.querySelector(".stage").classList.contains("draft")`); err != nil {
		return err
	}
	if err := click("Search sessions"); err != nil {
		return err
	}
	if err := eval(`const archived = document.querySelector("dialog input[type=checkbox]"); archived.click()`); err != nil {
		return err
	}
	if err := await("archived session found by search", `document.querySelector("dialog .picker-item")?.innerText.includes("Smoke workspace task")`); err != nil {
		return err
	}
	if err := eval(`document.querySelector("dialog .picker-item").click()`); err != nil {
		return err
	}
	if err := click("Session actions"); err != nil {
		return err
	}
	if err := click("Restore session"); err != nil {
		return err
	}
	return await("restored session returns to the workspace list", `!document.querySelector("dialog").open && document.querySelector(".session-row")?.innerText.includes("Smoke workspace task")`)
}

func fixtureModel(w http.ResponseWriter, r *http.Request, project string) {
	raw, _ := io.ReadAll(r.Body)
	if r.URL.Path != "/v1/chat/completions" {
		http.NotFound(w, r)
		return
	}
	var request struct {
		Model    string           `json:"model"`
		Stream   bool             `json:"stream"`
		Messages []map[string]any `json:"messages"`
		Tools    []struct {
			Function struct {
				Name string `json:"name"`
			} `json:"function"`
		} `json:"tools"`
	}
	if json.Unmarshal(raw, &request) != nil || len(request.Messages) == 0 {
		http.Error(w, "invalid fixture request", http.StatusBadRequest)
		return
	}
	last := request.Messages[len(request.Messages)-1]
	role, _ := last["role"].(string)
	text, _ := json.Marshal(last["content"])
	content := string(text)

	fragments := []map[string]any{{"content": "Smoke workspace task"}}
	finish := "stop"
	hold := false
	if len(request.Tools) > 0 {
		switch {
		case role == "tool":
			fragments = []map[string]any{{"content": "**Smoke file created.**"}}
		case strings.Contains(content, "Smoke stop"):
			fragments = []map[string]any{{"content": "Streaming in progress"}}
			hold = true
		case strings.Contains(content, "Smoke write"):
			for _, tool := range request.Tools {
				toolName := tool.Function.Name
				if toolName != "write" {
					continue
				}
				path := filepath.Join(project, "mygo-smoke.txt")
				args, _ := json.Marshal(map[string]string{"path": path, "content": "Hello from the native smoke test.\n"})
				fragments = []map[string]any{{"tool_calls": []any{map[string]any{
					"index": 0, "id": "call_smoke_write", "type": "function",
					"function": map[string]string{"name": toolName, "arguments": string(args)},
				}}}}
				finish = "tool_calls"
			}
		}
	}
	if !request.Stream {
		w.Header().Set("Content-Type", "application/json")
		_ = json.MarshalWrite(w, map[string]any{"id": "fixture", "object": "chat.completion", "created": time.Now().Unix(),
			"model": request.Model, "choices": []any{map[string]any{"index": 0, "finish_reason": "stop", "message": map[string]string{"role": "assistant", "content": "Smoke workspace task"}}}})
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	flush := w.(http.Flusher)
	send := func(delta map[string]any, reason any) {
		data, _ := json.Marshal(map[string]any{"id": "fixture", "object": "chat.completion.chunk", "created": time.Now().Unix(),
			"model": request.Model, "choices": []any{map[string]any{"index": 0, "finish_reason": reason, "delta": delta}}})
		fmt.Fprintf(w, "data: %s\n\n", data)
		flush.Flush()
	}
	send(map[string]any{"role": "assistant"}, nil)
	for _, fragment := range fragments {
		send(fragment, nil)
	}
	if hold {
		select {
		case <-r.Context().Done():
		case <-time.After(15 * time.Second):
		}
	}
	send(map[string]any{}, finish)
	fmt.Fprint(w, "data: [DONE]\n\n")
	flush.Flush()
}

func fixtureMCP(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var message struct {
		ID     any    `json:"id"`
		Method string `json:"method"`
		Params struct {
			Version string `json:"protocolVersion"`
		} `json:"params"`
	}
	if err := json.UnmarshalRead(r.Body, &message); err != nil {
		http.Error(w, "bad fixture RPC message", http.StatusBadRequest)
		return
	}
	if message.ID == nil {
		w.WriteHeader(http.StatusAccepted)
		return
	}
	var result any
	switch message.Method {
	case "initialize":
		result = map[string]any{
			"protocolVersion": message.Params.Version,
			"capabilities":    map[string]any{"tools": struct{}{}},
			"serverInfo":      map[string]string{"name": "smoke", "version": "1.0.0"},
		}
	case "tools/list":
		result = map[string]any{"tools": []any{}}
	default:
		result = struct{}{}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.MarshalWrite(w, map[string]any{"jsonrpc": "2.0", "id": message.ID, "result": result})
}
