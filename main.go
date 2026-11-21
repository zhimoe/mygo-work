package main

import (
	"context"
	"embed"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/egoist/mygo"
	_ "github.com/egoist/mygo/locales/zh-Hans"

	"mygo-work/internal/agent"
	"mygo-work/internal/opencode"
	"mygo-work/internal/workspace"
)

//go:embed web/*
var assets embed.FS

const openCodeID = "opencode"

var agentEvent = mygo.NewEvent[agent.Event]("agentUpdate")

func main() {
	dir := os.Getenv("MYGO_WORK_DATA_DIR")
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			log.Fatal(err)
		}
		dir = filepath.Join(home, "Library", "Application Support", "MyGo Work")
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
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	defer process.Close()
	desktop := &Desktop{folders: folders, prefs: prefs, code: process, runtimes: map[string]agent.Runtime{openCodeID: process}}
	mygo.SetFrontend(frontend)
	mygo.Bind(desktop)
	mygo.App.OnQuit(func() {
		cancel()
		process.Close()
	})
	mygo.App.WhenReady(func() {
		win := mygo.NewWindow(mygo.WindowOptions{
			Title: "MyGo Work", URL: "/",
			Width: 1180, Height: 800, MinWidth: 780, MinHeight: 560,
			TitleBarStyle:        mygo.TitleBarHidden,
			TrafficLightPosition: &mygo.Point{X: 15, Y: 16},
			BackgroundColor:      "#fafafa",
		})
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
