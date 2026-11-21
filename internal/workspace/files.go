package workspace

import (
	"bytes"
	"cmp"
	"crypto/rand"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"unicode/utf8"
)

const previewLimit = 512 << 10

type Entry struct {
	Name      string `json:"name"`
	Path      string `json:"path"`
	Directory bool   `json:"directory"`
}

type Preview struct {
	Text     string     `json:"text"`
	Rows     [][]string `json:"rows"`
	Binary   bool       `json:"binary"`
	Partial  bool       `json:"partial"`
	Revision string     `json:"revision"`
}

func (s *Store) root(directory string) (*os.Root, error) {
	folder, err := s.Get(directory)
	if err != nil {
		return nil, err
	}
	return os.OpenRoot(folder.Directory)
}

func (s *Store) Files(directory, path string) ([]Entry, error) {
	if !filepath.IsLocal(path) {
		return nil, fs.ErrInvalid
	}
	root, err := s.root(directory)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	files, err := fs.ReadDir(root.FS(), filepath.ToSlash(path))
	if err != nil {
		return nil, err
	}
	var out []Entry
	for _, item := range files {
		if strings.HasPrefix(item.Name(), ".") || (!item.IsDir() && !item.Type().IsRegular()) {
			continue
		}
		out = append(out, Entry{Name: item.Name(), Path: filepath.ToSlash(filepath.Join(path, item.Name())), Directory: item.IsDir()})
	}
	slices.SortFunc(out, func(a, b Entry) int {
		if a.Directory != b.Directory {
			if a.Directory {
				return -1
			}
			return 1
		}
		return cmp.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name))
	})
	return out, nil
}

func (s *Store) Preview(directory, path string) (Preview, error) {
	if !filepath.IsLocal(path) {
		return Preview{}, fs.ErrInvalid
	}
	root, err := s.root(directory)
	if err != nil {
		return Preview{}, err
	}
	defer root.Close()
	return previewFile(root, path)
}

func previewFile(root *os.Root, path string) (Preview, error) {
	stat, err := root.Lstat(path)
	if err != nil {
		return Preview{}, err
	}
	if !stat.Mode().IsRegular() {
		return Preview{}, errors.New("select a regular file")
	}
	f, err := root.Open(path)
	if err != nil {
		return Preview{}, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, previewLimit+1))
	if err != nil {
		return Preview{}, err
	}
	partial := len(data) > previewLimit
	if partial {
		data = data[:previewLimit]
		for remaining := 3; remaining > 0 && !utf8.Valid(data); remaining-- {
			data = data[:len(data)-1]
		}
	}
	if !utf8.Valid(data) || bytes.ContainsRune(data, 0) {
		return Preview{Binary: true}, nil
	}
	out := Preview{Text: string(data), Partial: partial}
	ext := strings.ToLower(filepath.Ext(path))
	if ext == ".csv" || ext == ".tsv" {
		reader := csv.NewReader(bytes.NewReader(data))
		reader.FieldsPerRecord = -1
		if ext == ".tsv" {
			reader.Comma = '\t'
		}
		for range 501 {
			row, err := reader.Read()
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil || len(row) > 80 {
				out.Rows = nil
				break
			}
			if len(out.Rows) == 500 {
				out.Partial = true
				break
			}
			out.Rows = append(out.Rows, row)
		}
	}
	if !out.Partial {
		hash := sha256.Sum256(data)
		out.Revision = hex.EncodeToString(hash[:])
	}
	return out, nil
}

func (s *Store) Resolve(directory, path string) (string, error) {
	if !filepath.IsLocal(path) {
		return "", fs.ErrInvalid
	}
	if _, err := s.Get(directory); err != nil {
		return "", err
	}
	real, err := filepath.EvalSymlinks(filepath.Join(directory, path))
	if err != nil {
		return "", err
	}
	relative, err := filepath.Rel(directory, real)
	if err != nil || !filepath.IsLocal(relative) {
		return "", fs.ErrPermission
	}
	return real, nil
}

func (s *Store) SaveFile(directory, path, revision, text string, rows [][]string) error {
	if !filepath.IsLocal(path) || revision == "" {
		return fs.ErrInvalid
	}
	data := []byte(text)
	if rows != nil {
		var buffer bytes.Buffer
		writer := csv.NewWriter(&buffer)
		switch strings.ToLower(filepath.Ext(path)) {
		case ".csv":
		case ".tsv":
			writer.Comma = '\t'
		default:
			return errors.New("table edits require a CSV or TSV file")
		}
		if err := writer.WriteAll(rows); err != nil {
			return err
		}
		data = buffer.Bytes()
	}
	if len(data) > previewLimit || !utf8.Valid(data) || bytes.ContainsRune(data, 0) {
		return errors.New("save supports text files up to 512 KiB")
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	root, err := s.root(directory)
	if err != nil {
		return err
	}
	defer root.Close()
	stat, err := root.Lstat(path)
	if err != nil {
		return err
	}
	if !stat.Mode().IsRegular() {
		return errors.New("select a regular file")
	}

	temporary := filepath.Join(filepath.Dir(path), ".mygo-work-"+rand.Text())
	file, err := root.OpenFile(temporary, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer root.Remove(temporary)
	_, writeErr := file.Write(data)
	modeErr := file.Chmod(stat.Mode().Perm())
	closeErr := file.Close()
	if err := errors.Join(writeErr, modeErr, closeErr); err != nil {
		return err
	}
	current, err := previewFile(root, path)
	if err != nil {
		return err
	}
	if current.Revision != revision {
		return errors.New("the file changed on disk. Reopen it before saving")
	}
	return root.Rename(temporary, path)
}
