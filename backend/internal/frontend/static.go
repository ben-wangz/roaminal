package frontend

import (
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// Handler serves a built frontend from a directory owned by the backend.
// The directory is validated before the server starts so production images
// cannot silently serve an empty shell.
func Handler(configured string) (http.Handler, error) {
	dir, err := resolveDir(configured)
	if err != nil {
		return nil, err
	}
	fileSystem := os.DirFS(dir)
	if info, err := fs.Stat(fileSystem, "index.html"); err != nil || !info.Mode().IsRegular() {
		if err == nil {
			err = errors.New("index.html is not a regular file")
		}
		return nil, fmt.Errorf("frontend assets are invalid: %w", err)
	}
	files := http.FileServer(http.FS(fileSystem))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(filepath.ToSlash(r.URL.Path), "/")
		if name == "" || name == "index.html" {
			w.Header().Set("Cache-Control", "no-cache, max-age=0")
		} else if strings.HasPrefix(name, "assets/") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		} else {
			w.Header().Set("Cache-Control", "public, max-age=300")
		}
		if r.URL.Path == "/" || r.URL.Path == "/index.html" {
			data, err := fs.ReadFile(fileSystem, "index.html")
			if err != nil {
				http.Error(w, "not found", http.StatusNotFound)
				return
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write(data)
			return
		}
		if !fs.ValidPath(name) {
			http.NotFound(w, r)
			return
		}
		files.ServeHTTP(w, r)
	}), nil
}

func resolveDir(configured string) (string, error) {
	candidates := []string{configured}
	if configured == "" {
		candidates = nil
	}
	candidates = append(candidates, filepath.Join("frontend", "dist"), filepath.Join("..", "frontend", "dist"), "/opt/roaminal/frontend")
	for _, candidate := range candidates {
		if candidate == "" {
			continue
		}
		absolute, err := filepath.Abs(candidate)
		if err != nil {
			continue
		}
		info, err := os.Stat(absolute)
		if err == nil && info.IsDir() {
			if _, err := os.Stat(filepath.Join(absolute, "index.html")); err == nil {
				return absolute, nil
			}
		}
	}
	return "", fmt.Errorf("frontend assets not found (configured directory %q)", configured)
}
