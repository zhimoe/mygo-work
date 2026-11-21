package main

import (
	"cmp"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/egoist/mygo"
)

type preferences struct {
	mu       sync.Mutex
	path     string
	language string
}

func loadPreferences(dir string) (*preferences, error) {
	path := filepath.Join(dir, "language")
	raw, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	value := strings.TrimSpace(string(raw))
	if value != "en" {
		value = "zh-CN"
	}
	return &preferences{path: path, language: value}, nil
}

func (d *Desktop) Language() string {
	d.prefs.mu.Lock()
	defer d.prefs.mu.Unlock()
	return cmp.Or(d.prefs.language, "zh-CN")
}

func (d *Desktop) SetLanguage(language string) error {
	if language != "en" && language != "zh-CN" {
		return errors.New("unsupported language")
	}
	d.prefs.mu.Lock()
	defer d.prefs.mu.Unlock()
	if d.prefs.language == language {
		return nil
	}

	f, err := os.CreateTemp(filepath.Dir(d.prefs.path), ".language-")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	_, writeErr := f.WriteString(language + "\n")
	if err := errors.Join(writeErr, f.Close()); err != nil {
		return err
	}
	if err := os.Rename(f.Name(), d.prefs.path); err != nil {
		return err
	}
	d.prefs.language = language
	setNativeLocale(language)
	return nil
}

func (d *Desktop) nativeLabel(chinese, english string) string {
	if d.Language() == "en" {
		return english
	}
	return chinese
}

func setNativeLocale(language string) {
	if language == "en" {
		mygo.App.SetLocale("en")
	} else {
		mygo.App.SetLocale("zh-Hans")
	}
}
