package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ben-wangz/roaminal/backend/internal/api"
	"github.com/ben-wangz/roaminal/backend/internal/filesystem"
)

type uploadHandlerRemote struct {
	contentPreviewRemote
	done <-chan struct{}
}

func (r *uploadHandlerRemote) ResolveUploadTarget(ctx context.Context, _, _, _ string) (string, error) {
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case <-r.done:
		return "", context.Canceled
	}
}

func uploadHandlerServer(t *testing.T) (*Server, string) {
	t.Helper()
	state := t.TempDir()
	if err := filesystem.PrepareUploadStaging(state); err != nil {
		t.Fatal(err)
	}
	service := filesystem.NewWithRemote(&uploadHandlerRemote{done: t.Context().Done()}, nil, nil, state)
	return &Server{filesystem: service}, filepath.Join(state, "upload-staging")
}

func uploadHandlerRequest(body io.Reader, contentType string) *http.Request {
	request := httptest.NewRequest(http.MethodPost, "/filesystem/uploads", body)
	request.Header.Set("Content-Type", contentType)
	request.SetPathValue("connectionInstanceId", "instance-1")
	return request
}

func serveUpload(s *Server, request *http.Request) *httptest.ResponseRecorder {
	response := httptest.NewRecorder()
	s.filesystemCreateUpload(response, request, "")
	return response
}

func assertUploadError(t *testing.T, response *httptest.ResponseRecorder, status int, code string) api.ErrorResponse {
	t.Helper()
	var body api.ErrorResponse
	if response.Code != status || json.Unmarshal(response.Body.Bytes(), &body) != nil || string(body.Code) != code {
		t.Fatalf("upload status = %d, body = %s; want %d, %s", response.Code, response.Body, status, code)
	}
	return body
}

type uploadZeros struct{}

func (uploadZeros) Read(p []byte) (int, error) { clear(p); return len(p), nil }

type uploadGate struct{ reading, release chan struct{} }

func (r uploadGate) Read([]byte) (int, error) {
	close(r.reading)
	<-r.release
	return 0, io.EOF
}

func waitUploadHTTP[T any](t *testing.T, ch <-chan T) T {
	t.Helper()
	select {
	case value := <-ch:
		return value
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for upload")
		panic("unreachable")
	}
}

const uploadTestManifest = `{"targetPath":".","conflictPolicy":"overwrite","files":[{"part":"file-0","relativePath":"nested/file.bin","size":%d}]}`

func TestFilesystemUploadStreamsPastOldSpillThreshold(t *testing.T) {
	s, staging := uploadHandlerServer(t)
	tmp := t.TempDir()
	t.Setenv("TMPDIR", tmp)
	const size = 9 << 20
	var wire bytes.Buffer
	writer := multipart.NewWriter(&wire)
	if err := writer.WriteField("manifest", fmt.Sprintf(uploadTestManifest, size)); err != nil {
		t.Fatal(err)
	}
	if _, err := writer.CreateFormFile("file-0", "ignored/filename.bin"); err != nil {
		t.Fatal(err)
	}
	prefix := wire.Len()
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	gate := uploadGate{make(chan struct{}), make(chan struct{})}
	release := func() {
		select {
		case <-gate.release:
		default:
			close(gate.release)
		}
	}
	defer release()
	body := io.MultiReader(bytes.NewReader(wire.Bytes()[:prefix]), io.LimitReader(uploadZeros{}, size-1), gate, strings.NewReader("x"), bytes.NewReader(wire.Bytes()[prefix:]))
	request := uploadHandlerRequest(body, writer.FormDataContentType())
	finished := make(chan *httptest.ResponseRecorder, 1)
	go func() { finished <- serveUpload(s, request) }()
	select {
	case <-gate.reading:
	case response := <-finished:
		t.Fatalf("upload stopped before streaming the payload: %d %s", response.Code, response.Body)
	case <-time.After(30 * time.Second):
		t.Fatal("timed out waiting for streamed payload")
	}
	if entries, err := os.ReadDir(tmp); err != nil || len(entries) != 0 {
		t.Fatalf("unexpected multipart temp files: %v, %v", entries, err)
	}
	dirs, err := os.ReadDir(staging)
	if err != nil || len(dirs) != 1 {
		t.Fatalf("expected one staging directory before full body: %v, %v", dirs, err)
	}
	path := filepath.Join(staging, dirs[0].Name(), "nested", "file.bin")
	if file, err := os.Stat(path); err != nil || file.Size() <= 8<<20 || file.Size() >= size || file.Mode().Perm() != 0o600 {
		t.Fatalf("payload not streamed into its private manifest path: %v, %v", file, err)
	}
	release()
	response := waitUploadHTTP(t, finished)
	var status filesystem.UploadStatus
	if response.Code != http.StatusAccepted || json.Unmarshal(response.Body.Bytes(), &status) != nil || status.Status != "queued" || status.BytesTotal != size {
		t.Fatalf("upload status = %d, body = %s", response.Code, response.Body)
	}
	if file, err := os.Stat(path); err != nil || file.Size() != size {
		t.Errorf("accepted file not fully staged: %v, %v", file, err)
	}
	if _, err := s.filesystem.CancelUpload("instance-1", status.UploadID); err != nil {
		t.Fatal(err)
	}
}

type uploadWirePart struct{ name, filename, data string }

func uploadWire(t *testing.T, parts ...uploadWirePart) (*bytes.Buffer, string) {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for _, part := range parts {
		if part.filename == "" {
			if err := writer.WriteField(part.name, part.data); err != nil {
				t.Fatal(err)
			}
		} else {
			file, err := writer.CreateFormFile(part.name, part.filename)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := io.WriteString(file, part.data); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return &body, writer.FormDataContentType()
}

func TestFilesystemUploadRejectsMalformedParts(t *testing.T) {
	s, staging := uploadHandlerServer(t)
	manifest := uploadWirePart{name: "manifest", data: fmt.Sprintf(uploadTestManifest, 4)}
	file := uploadWirePart{"file-0", "file.bin", "data"}
	for _, test := range []struct {
		name   string
		parts  []uploadWirePart
		status int
		code   string
	}{
		{"file first", []uploadWirePart{file, manifest}, 400, "filesystem_upload_manifest_invalid"},
		{"duplicate manifest", []uploadWirePart{manifest, file, manifest}, 400, "filesystem_upload_manifest_invalid"},
		{"duplicate file", []uploadWirePart{manifest, file, file}, 400, "filesystem_upload_manifest_invalid"},
		{"unexpected file", []uploadWirePart{manifest, file, {"file-1", "other", "other"}}, 400, "filesystem_upload_manifest_invalid"},
		{"unknown field", []uploadWirePart{{name: "manifest", data: `{"unexpected":true}`}}, 400, "filesystem_upload_manifest_invalid"},
		{"unknown file field", []uploadWirePart{{name: "manifest", data: `{"files":[{"unexpected":true}]}`}}, 400, "filesystem_upload_manifest_invalid"},
		{"file count limit", []uploadWirePart{{name: "manifest", data: `{"files":[` + strings.Repeat(`{},`, filesystem.MaxUploadFiles) + `{}]}`}}, 413, "filesystem_content_too_large"},
		{"trailing JSON", []uploadWirePart{{name: "manifest", data: manifest.data + ` {}`}}, 400, "filesystem_upload_manifest_invalid"},
		{"missing file", []uploadWirePart{manifest}, 409, "filesystem_content_unavailable"},
		{"short file", []uploadWirePart{manifest, {"file-0", "file.bin", "abc"}}, 409, "filesystem_content_unavailable"},
		{"long file", []uploadWirePart{manifest, {"file-0", "file.bin", "abcde"}}, 409, "filesystem_content_unavailable"},
		{"stale root", []uploadWirePart{{name: "manifest", data: `{"rootRevision":"old",` + manifest.data[1:]}, file}, 409, "filesystem_root_changed"},
		{"manifest limit", []uploadWirePart{{name: "manifest", data: strings.Repeat(" ", uploadManifestLimit+1)}}, 413, "filesystem_content_too_large"},
		{"payload limit", []uploadWirePart{{name: "manifest", data: fmt.Sprintf(uploadTestManifest, filesystem.MaxUploadBytes+1)}}, 413, "filesystem_content_too_large"},
	} {
		t.Run(test.name, func(t *testing.T) {
			body, contentType := uploadWire(t, test.parts...)
			result := assertUploadError(t, serveUpload(s, uploadHandlerRequest(body, contentType)), test.status, test.code)
			if test.name == "stale root" && result.Details == nil {
				t.Error("missing current root details")
			}
			if entries, err := os.ReadDir(staging); err != nil || len(entries) != 0 {
				t.Fatalf("staging not empty after rejected upload: %v, %v", entries, err)
			}
		})
	}
}

func TestFilesystemUploadBoundsHTTPBody(t *testing.T) {
	s, staging := uploadHandlerServer(t)
	manifest := uploadWirePart{name: "manifest", data: fmt.Sprintf(uploadTestManifest, 0)}
	file := uploadWirePart{"file-0", "file.bin", ""}
	body, contentType := uploadWire(t, manifest, file)
	request := uploadHandlerRequest(bytes.NewReader(body.Bytes()), contentType)
	request.ContentLength = filesystemUploadBodyLimit + 1
	assertUploadError(t, serveUpload(s, request), http.StatusRequestEntityTooLarge, "filesystem_content_too_large")

	// A terminating multipart boundary cannot bypass the wire-body limit.
	chunked := io.MultiReader(body, io.LimitReader(uploadZeros{}, uploadMultipartOverhead+1))
	assertUploadError(t, serveUpload(s, uploadHandlerRequest(chunked, contentType)), http.StatusRequestEntityTooLarge, "filesystem_content_too_large")
	if entries, err := os.ReadDir(staging); err != nil || len(entries) != 0 {
		t.Fatalf("staging not empty after oversized body: %v, %v", entries, err)
	}
}
