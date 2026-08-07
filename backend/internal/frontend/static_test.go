package frontend

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHandlerServesFrontendCachePolicies(t *testing.T) {
	dir := t.TempDir()
	writeFrontendFile(t, dir, "index.html", "<main>Roaminal</main>")
	writeFrontendFile(t, dir, "assets/app-123.js", "console.log('ok')")
	writeFrontendFile(t, dir, "favicon.svg", "<svg></svg>")
	handler, err := Handler(dir)
	if err != nil {
		t.Fatalf("Handler() error = %v", err)
	}

	tests := []struct {
		name   string
		path   string
		status int
		cache  string
		body   string
	}{
		{name: "root", path: "/", status: http.StatusOK, cache: "no-cache, max-age=0", body: "Roaminal"},
		{name: "index", path: "/index.html", status: http.StatusOK, cache: "no-cache, max-age=0", body: "Roaminal"},
		{name: "hashed asset", path: "/assets/app-123.js", status: http.StatusOK, cache: "public, max-age=31536000, immutable", body: "console.log"},
		{name: "ordinary asset", path: "/favicon.svg", status: http.StatusOK, cache: "public, max-age=300", body: "<svg"},
		{name: "missing asset", path: "/missing.js", status: http.StatusNotFound, cache: "", body: "404"},
		{name: "traversal", path: "/../index.html", status: http.StatusNotFound, cache: "public, max-age=300", body: "404"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, "http://roaminal.test"+test.path, nil)
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != test.status {
				t.Fatalf("status = %d, want %d", response.Code, test.status)
			}
			if got := response.Header().Get("Cache-Control"); got != test.cache {
				t.Fatalf("Cache-Control = %q, want %q", got, test.cache)
			}
			if !strings.Contains(response.Body.String(), test.body) {
				t.Fatalf("body %q does not contain %q", response.Body.String(), test.body)
			}
		})
	}
}

func TestHandlerRejectsMissingIndex(t *testing.T) {
	if _, err := Handler(t.TempDir()); err == nil {
		t.Fatal("Handler() succeeded without index.html")
	}
}

func TestHandlerRejectsNonRegularIndex(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "index.html"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := Handler(dir); err == nil {
		t.Fatal("Handler() succeeded with a directory named index.html")
	}
}

func writeFrontendFile(t *testing.T, dir, name, content string) {
	t.Helper()
	path := filepath.Join(dir, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
