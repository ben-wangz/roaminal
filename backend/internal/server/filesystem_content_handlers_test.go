package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"image"
	"image/color"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ben-wangz/roaminal/backend/internal/filesystem"
	"github.com/ben-wangz/roaminal/backend/internal/imagepreview"
	"github.com/ben-wangz/roaminal/backend/internal/ports"
)

type contentPreviewRemote struct {
	data            []byte
	entry           ports.RemoteFileEntry
	entryAfterFirst *ports.RemoteFileEntry
	statCalls       atomic.Int32
}

func (r *contentPreviewRemote) ConnectionInstance(string) (ports.ConnectionInstanceView, error) {
	alias := "fixture"
	return ports.ConnectionInstanceView{ID: "instance-1", Type: "ssh", Lifecycle: "live", SourceHostAlias: &alias}, nil
}

func (r *contentPreviewRemote) ProbeTmuxRoot(context.Context, string, string) (ports.RemoteRoot, error) {
	return ports.RemoteRoot{}, errors.New("tmux unavailable")
}

func (r *contentPreviewRemote) ProbeConfiguredRoot(context.Context, string, string) (ports.RemoteRoot, error) {
	return ports.RemoteRoot{AbsolutePath: "/workspace", Source: "configured", Status: "fallback"}, nil
}

func (*contentPreviewRemote) List(context.Context, string, string, string) ([]ports.RemoteFileEntry, error) {
	return nil, nil
}

func (r *contentPreviewRemote) Stat(context.Context, string, string, string) (ports.RemoteFileEntry, error) {
	if r.statCalls.Add(1) > 1 && r.entryAfterFirst != nil {
		return *r.entryAfterFirst, nil
	}
	return r.entry, nil
}

func (r *contentPreviewRemote) OpenContent(_ context.Context, _ string, _ string, _ string, start, length int64) (io.ReadCloser, error) {
	if start < 0 || length < 0 || start > int64(len(r.data)) || length > int64(len(r.data))-start {
		return nil, errors.New("invalid fixture range")
	}
	return io.NopCloser(bytes.NewReader(r.data[start : start+length])), nil
}

func (*contentPreviewRemote) AcquireRemoteTransfer(context.Context, string) (ports.RemoteTransferLease, error) {
	return nil, errors.New("transfer unavailable")
}

func (*contentPreviewRemote) ResolveUploadTarget(context.Context, string, string, string) (string, error) {
	return "", errors.New("upload unavailable")
}

func (*contentPreviewRemote) UploadConflicts(context.Context, string, string, string, []string) ([]string, error) {
	return nil, errors.New("upload unavailable")
}

func (*contentPreviewRemote) RsyncAvailable(context.Context, string) (bool, error) { return false, nil }

func (*contentPreviewRemote) CreateDirectories(context.Context, string, string, string, []string) error {
	return errors.New("upload unavailable")
}

func (*contentPreviewRemote) ShouldUploadWithScp(context.Context, string, string, string, int64) (bool, error) {
	return false, errors.New("upload unavailable")
}

func imagePreviewPNG(t *testing.T) []byte {
	t.Helper()
	value := image.NewRGBA(image.Rect(0, 0, 16, 12))
	for y := 0; y < value.Bounds().Dy(); y++ {
		for x := 0; x < value.Bounds().Dx(); x++ {
			value.Set(x, y, color.RGBA{R: uint8(x * 11), G: uint8(y * 17), B: 120, A: 255})
		}
	}
	var output bytes.Buffer
	if err := png.Encode(&output, value); err != nil {
		t.Fatal(err)
	}
	return output.Bytes()
}

func newContentPreviewServer(t *testing.T, data []byte, name string) (*Server, *contentPreviewRemote, filesystem.RootContext) {
	t.Helper()
	modified := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	size := int64(len(data))
	remote := &contentPreviewRemote{data: data, entry: ports.RemoteFileEntry{Name: name, Type: "file", Size: &size, ModifiedAt: &modified, Mode: 0o644}}
	fileSystem := filesystem.NewWithRemote(remote, nil, nil, "")
	root, err := fileSystem.Root(context.Background(), "instance-1")
	if err != nil {
		t.Fatal(err)
	}
	cacheDir, err := os.MkdirTemp("/var/tmp", "roaminal-server-image-preview-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(cacheDir) })
	preview := imagepreview.New(imagepreview.Options{
		CacheDir: cacheDir, CacheTargetBytes: 1 << 20, CacheMaxAge: time.Minute, CleanupInterval: time.Minute,
		MaxConversions: 1, MaxSourceBytes: 1 << 20, MaxOutputBytes: 1 << 20, MaxStaticPixels: 1_000_000,
		MaxFrames: 10, MaxAnimatedPixels: 2_000_000, ConversionTimeout: 10 * time.Second,
	})
	if !preview.Available() {
		t.Skip("libvips is unavailable")
	}
	t.Cleanup(preview.Close)
	return New(Dependencies{FileSystem: fileSystem, ImagePreview: preview}), remote, root
}

func contentRequest(root filesystem.RootContext, query url.Values) *http.Request {
	query.Set("path", "image.png")
	query.Set("rootRevision", root.Revision)
	request := httptest.NewRequest(http.MethodGet, "/api/v2/connection-instances/instance-1/filesystem/content?"+query.Encode(), nil)
	request.SetPathValue("connectionInstanceId", "instance-1")
	return request
}

func serveContent(s *Server, request *http.Request) *httptest.ResponseRecorder {
	response := httptest.NewRecorder()
	s.filesystemContent(response, request, "")
	return response
}

func TestFilesystemContentImageVariants(t *testing.T) {
	data := imagePreviewPNG(t)
	server, _, root := newContentPreviewServer(t, data, "image.png")

	preview := serveContent(server, contentRequest(root, url.Values{"variant": []string{"preview"}}))
	if preview.Code != http.StatusOK || preview.Header().Get("Content-Type") != imagepreview.OutputContentType || preview.Header().Get("X-Roaminal-Image-Variant") != "preview" {
		t.Fatalf("preview response status=%d headers=%v body=%s", preview.Code, preview.Header(), preview.Body.String())
	}
	if len(preview.Body.Bytes()) < 12 || !bytes.HasPrefix(preview.Body.Bytes(), []byte("RIFF")) || !bytes.Equal(preview.Body.Bytes()[8:12], []byte("WEBP")) {
		t.Fatalf("preview body is not WebP: %x", preview.Body.Bytes()[:minInt(len(preview.Body.Bytes()), 12)])
	}
	previewETag := preview.Header().Get("ETag")
	if preview.Header().Get("X-Roaminal-Content-Truncated") != "" {
		t.Fatal("complete preview must not be marked truncated")
	}

	original := serveContent(server, contentRequest(root, url.Values{"variant": []string{"original"}}))
	if original.Code != http.StatusOK || original.Header().Get("Content-Type") != "image/png" || original.Header().Get("X-Roaminal-Image-Variant") != "original" || !bytes.Equal(original.Body.Bytes(), data) {
		t.Fatalf("original response status=%d headers=%v bytes=%d", original.Code, original.Header(), original.Body.Len())
	}
	if original.Header().Get("ETag") == previewETag || original.Header().Get("Content-Disposition") != "" {
		t.Fatalf("original response has invalid etag/disposition: etag=%q disposition=%q", original.Header().Get("ETag"), original.Header().Get("Content-Disposition"))
	}
	legacy := serveContent(server, contentRequest(root, url.Values{}))
	legacyRequest := contentRequest(root, url.Values{})
	legacyRequest.Header.Set("If-None-Match", legacy.Header().Get("ETag"))
	legacyResponse := serveContent(server, legacyRequest)
	if legacyResponse.Code != http.StatusOK {
		t.Fatalf("legacy content conditional status=%d, want 200", legacyResponse.Code)
	}

	download := serveContent(server, contentRequest(root, url.Values{"variant": []string{"preview"}, "download": []string{"1"}}))
	if download.Code != http.StatusOK || download.Header().Get("Content-Type") != "image/png" || !bytes.Equal(download.Body.Bytes(), data) || download.Header().Get("Content-Disposition") == "" {
		t.Fatalf("download response status=%d headers=%v bytes=%d", download.Code, download.Header(), download.Body.Len())
	}

	ranged := contentRequest(root, url.Values{"variant": []string{"preview"}})
	ranged.Header.Set("Range", "bytes=0-3")
	rangeResponse := serveContent(server, ranged)
	if rangeResponse.Code != http.StatusPartialContent || rangeResponse.Header().Get("Content-Range") != "bytes 0-3/"+itoa(preview.Body.Len()) || rangeResponse.Body.Len() != 4 {
		t.Fatalf("preview range status=%d range=%q bytes=%d", rangeResponse.Code, rangeResponse.Header().Get("Content-Range"), rangeResponse.Body.Len())
	}

	conditional := contentRequest(root, url.Values{"variant": []string{"preview"}})
	conditional.Header.Set("If-None-Match", previewETag)
	conditionalResponse := serveContent(server, conditional)
	if conditionalResponse.Code != http.StatusNotModified {
		t.Fatalf("conditional preview status=%d, want 304", conditionalResponse.Code)
	}
}

func TestFilesystemContentRejectsInvalidAndUnsupportedImageVariants(t *testing.T) {
	data := imagePreviewPNG(t)
	server, _, root := newContentPreviewServer(t, data, "image.png")
	invalid := serveContent(server, contentRequest(root, url.Values{"variant": []string{"thumbnail"}}))
	var invalidBody struct {
		Code string `json:"code"`
	}
	if invalid.Code != http.StatusBadRequest || json.Unmarshal(invalid.Body.Bytes(), &invalidBody) != nil || invalidBody.Code != "filesystem_variant_invalid" {
		t.Fatalf("invalid variant status=%d body=%s", invalid.Code, invalid.Body.String())
	}

	unsupportedServer, _, unsupportedRoot := newContentPreviewServer(t, data, "image.svg")
	unsupported := serveContent(unsupportedServer, contentRequest(unsupportedRoot, url.Values{"variant": []string{"preview"}}))
	if unsupported.Code != http.StatusUnsupportedMediaType || !bytes.Contains(unsupported.Body.Bytes(), []byte("filesystem_image_preview_unsupported")) {
		t.Fatalf("unsupported variant status=%d body=%s", unsupported.Code, unsupported.Body.String())
	}
}

func TestFilesystemContentRejectsSourceChangeDuringPreview(t *testing.T) {
	data := imagePreviewPNG(t)
	server, remote, root := newContentPreviewServer(t, data, "image.png")
	changed := remote.entry
	changed.Mode++
	remote.entryAfterFirst = &changed
	response := serveContent(server, contentRequest(root, url.Values{"variant": []string{"preview"}}))
	if response.Code != http.StatusConflict || !bytes.Contains(response.Body.Bytes(), []byte("filesystem_content_unavailable")) {
		t.Fatalf("source-change response status=%d body=%s", response.Code, response.Body.String())
	}
}

func itoa(value int) string {
	return strconv.Itoa(value)
}

func minInt(left, right int) int {
	if left < right {
		return left
	}
	return right
}
