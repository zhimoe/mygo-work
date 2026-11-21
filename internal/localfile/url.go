package localfile

import (
	"errors"
	"net/url"
	"path/filepath"
	"runtime"
	"strings"
)

func URL(path string) string {
	normalized := filepath.ToSlash(path)
	host := ""
	if runtime.GOOS == "windows" {
		if unc, ok := strings.CutPrefix(normalized, "//"); ok {
			host, normalized, _ = strings.Cut(unc, "/")
		}
		normalized = "/" + normalized
	}
	return (&url.URL{Scheme: "file", Host: host, Path: normalized}).String()
}

func Path(uri string) (string, error) {
	u, err := url.Parse(uri)
	if err != nil || u.Scheme != "file" || u.Opaque != "" || u.RawQuery != "" || u.Fragment != "" || u.User != nil {
		return "", errors.New("attachment must be a local file")
	}
	path := u.Path
	if runtime.GOOS == "windows" {
		if u.Host != "" {
			path = "//" + u.Host + path
		} else {
			path = strings.TrimPrefix(path, "/")
		}
	} else if u.Host != "" {
		return "", errors.New("attachment must be a local file")
	}
	path = filepath.FromSlash(path)
	if !filepath.IsAbs(path) {
		return "", errors.New("attachment must be a local file")
	}
	return path, nil
}
