package workspace

import (
	"encoding/json/v2"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
)

type Folder struct {
	Directory string `json:"directory"`
	Name      string `json:"name"`
	Runtime   string `json:"runtime"`
}

type Store struct {
	mu      sync.Mutex
	writeMu sync.Mutex
	path    string
	folders []Folder
}

func Open(directory string) (*Store, error) {
	if err := os.MkdirAll(directory, 0700); err != nil {
		return nil, err
	}
	s := &Store{path: filepath.Join(directory, "workspaces.json")}
	data, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	return s, json.Unmarshal(data, &s.folders)
}

func (s *Store) List() []Folder {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.folders)
}

func (s *Store) Get(path string) (Folder, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, folder := range s.folders {
		if folder.Directory == path {
			return folder, nil
		}
	}
	return Folder{}, errors.New("workspace is not registered")
}

func (s *Store) Add(path, runtime string) (Folder, error) {
	real, err := filepath.EvalSymlinks(path)
	if err != nil {
		return Folder{}, err
	}
	real, err = filepath.Abs(real)
	if err != nil {
		return Folder{}, err
	}
	info, err := os.Stat(real)
	if err != nil {
		return Folder{}, err
	}
	if !info.IsDir() {
		return Folder{}, errors.New("choose a folder")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, folder := range s.folders {
		if folder.Directory == real {
			return folder, nil
		}
	}
	folder := Folder{Directory: real, Name: filepath.Base(real), Runtime: runtime}
	return folder, s.save(append(slices.Clone(s.folders), folder))
}

func (s *Store) Rename(path, name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return errors.New("workspace name is empty")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	next := slices.Clone(s.folders)
	index := slices.IndexFunc(next, func(folder Folder) bool { return folder.Directory == path })
	if index == -1 {
		return errors.New("workspace is not registered")
	}
	next[index].Name = name
	return s.save(next)
}

func (s *Store) SetRuntime(path, runtime string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	next := slices.Clone(s.folders)
	index := slices.IndexFunc(next, func(folder Folder) bool { return folder.Directory == path })
	if index == -1 {
		return errors.New("workspace is not registered")
	}
	next[index].Runtime = runtime
	return s.save(next)
}

func (s *Store) Remove(path string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.save(slices.DeleteFunc(slices.Clone(s.folders), func(folder Folder) bool {
		return folder.Directory == path
	}))
}

func (s *Store) save(folders []Folder) error {
	f, err := os.CreateTemp(filepath.Dir(s.path), ".workspaces-")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	writeErr := json.MarshalWrite(f, folders)
	closeErr := f.Close()
	if err := errors.Join(writeErr, closeErr); err != nil {
		return err
	}
	if err := os.Rename(f.Name(), s.path); err != nil {
		return err
	}
	s.folders = folders
	return nil
}
