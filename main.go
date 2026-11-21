package main

import (
	"context"
	"embed"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/egoist/mygo"
	_ "github.com/egoist/mygo/locales/zh-Hans"

	"mygo-work/internal/agent"
	"mygo-work/internal/opencode"
	"mygo-work/internal/pi"
	"mygo-work/internal/workspace"
)

//go:embed web/*
var assets embed.FS

const (
	openCodeID = "opencode"
	piID       = "pi"
)

var agentEvent = mygo.NewEvent[agent.Event]("agentUpdate")

func main() {
	dir := os.Getenv("MYGO_WORK_DATA_DIR")
	if dir == "" {
		config, err := os.UserConfigDir()
		if err != nil {
			log.Fatal(err)
		}
		dir = filepath.Join(config, "MyGo Work")
	}
	dir, err := filepath.Abs(dir)
	if err != nil {
		log.Fatal(err)
	}
	mygo.App.SetName("MyGo Work")
	if !mygo.App.RequestSingleInstanceLock() {
		return
	}
	folders, err := workspace.Open(dir)
	if err != nil {
		log.Fatal(err)
	}
	prefs, err := loadPreferences(dir)
	if err != nil {
		log.Fatal(err)
	}
	mygo.App.SetLanguages("zh-Hans", "en")
	setNativeLocale(prefs.language)
	frontend, err := fs.Sub(assets, "web")
	if err != nil {
		log.Fatal(err)
	}
	process := opencode.NewProcess(dir)
	runtimes := map[string]agent.Runtime{openCodeID: process}
	var closePi func()
	if piRuntime, err := pi.Open(dir); err == nil {
		runtimes[piID] = piRuntime
		closePi = piRuntime.Close
	} else {
		log.Printf("Pi runtime: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	closeRuntimes := func() {
		cancel()
		process.Close()
		if closePi != nil {
			closePi()
		}
	}
	defer closeRuntimes()
	desktop := &Desktop{folders: folders, prefs: prefs, code: process, runtimes: runtimes}
	mygo.SetFrontend(frontend)
	mygo.Bind(desktop)
	mygo.App.OnQuit(closeRuntimes)
	mygo.App.WhenReady(func() {
		if runtime.GOOS == "windows" && !mygo.WebViewAvailable() {
			showWebViewError(desktop)
			mygo.App.Quit()
			return
		}
		options := mygo.WindowOptions{
			Title: "MyGo Work", URL: "/",
			Width: 1180, Height: 800, MinWidth: 780, MinHeight: 560,
			BackgroundColor: "#fafafa",
		}
		if runtime.GOOS == "darwin" {
			options.TitleBarStyle = mygo.TitleBarHidden
			options.TrafficLightPosition = &mygo.Point{X: 15, Y: 16}
		}
		win := mygo.NewWindow(options)
		win.OnClosed(func() { mygo.App.Quit() })
		mygo.App.OnSecondInstance(func([]string, string) { win.Focus() })
		for id, runtime := range desktop.runtimes {
			go watch(ctx, id, runtime, win)
		}
	})
	if err := mygo.App.Run(); err != nil {
		log.Print(err)
	}
}

func showWebViewError(desktop *Desktop) {
	result, err := mygo.Dialog.Message(mygo.MessageOptions{
		Title: "MyGo Work",
		Type:  mygo.MessageError,
		Message: desktop.nativeLabel(
			"请安装 Microsoft Edge WebView2 运行时，然后重新打开 MyGo Work。",
			"Install the Microsoft Edge WebView2 Runtime, then reopen MyGo Work.",
		),
		Buttons:      []string{desktop.nativeLabel("打开下载页", "Open download page"), desktop.nativeLabel("关闭", "Close")},
		CancelButton: 1,
	})
	if err == nil && result.Button == 0 {
		_ = mygo.Shell.OpenExternal("https://developer.microsoft.com/microsoft-edge/webview2/")
	}
}

func watch(ctx context.Context, id string, runtime agent.Runtime, window *mygo.Window) {
	delay := time.Second
	for ctx.Err() == nil {
		err := runtime.Watch(ctx, func(event agent.Event) {
			if event.Kind == "connected" {
				delay = time.Second
			}
			event.Runtime = id
			_ = agentEvent.Emit(window, event)
		})
		if ctx.Err() != nil {
			return
		}
		if err != nil {
			log.Printf("agent event stream: %v", err)
		}
		_ = agentEvent.Emit(window, agent.Event{Kind: "disconnected", Runtime: id})
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
			delay = min(delay*2, 15*time.Second)
		}
	}
}
