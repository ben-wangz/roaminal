package imagepreview

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ydylla/fcache"
)

func TestPreviewETagIncludesEverySourceIdentityField(t *testing.T) {
	data := pngFixture(t)
	base := previewRequest(data)
	service := &Service{}
	original, err := service.ETag(base)
	if err != nil {
		t.Fatal(err)
	}
	variants := map[string]func(*Request){
		"connection":   func(value *Request) { value.ConnectionInstanceID = "other-instance" },
		"root path":    func(value *Request) { value.RootAbsolutePath = "/other-root" },
		"revision":     func(value *Request) { value.RootRevision = "root-2" },
		"relative":     func(value *Request) { value.RelativePath = "nested/image.png" },
		"source token": func(value *Request) { value.SourceToken = "token-2" },
		"source size":  func(value *Request) { value.SourceSize++ },
	}
	for name, mutate := range variants {
		t.Run(name, func(t *testing.T) {
			value := base
			mutate(&value)
			etag, err := service.ETag(value)
			if err != nil {
				t.Fatal(err)
			}
			if etag == original {
				t.Fatalf("identity mutation did not change ETag %q", etag)
			}
		})
	}
	if original == `"token-1"` {
		t.Fatal("preview ETag must not reuse the source ETag")
	}
}

func TestServiceCacheCorruptionRegeneratesOnce(t *testing.T) {
	data := pngFixture(t)
	service := New(testOptions(t))
	if !service.Available() {
		t.Skip("libvips is unavailable")
	}
	request := previewRequest(data)
	if result, err := service.Open(context.Background(), request); err != nil {
		t.Fatal(err)
	} else {
		_ = result.Reader.Close()
	}
	cacheFile := firstCacheFile(t, service.cacheDir)
	if err := os.WriteFile(cacheFile, []byte("corrupt"), 0o600); err != nil {
		t.Fatal(err)
	}
	var opens atomic.Int32
	request.Open = func(context.Context) (io.ReadCloser, error) {
		opens.Add(1)
		return io.NopCloser(bytes.NewReader(data)), nil
	}
	result, err := service.Open(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	output, err := io.ReadAll(result.Reader)
	_ = result.Reader.Close()
	if err != nil || !bytes.HasPrefix(output, []byte("RIFF")) || opens.Load() != 1 {
		t.Fatalf("regenerated output bytes=%d err=%v opens=%d", len(output), err, opens.Load())
	}
	service.Close()
}

func TestServiceStartupClearsCacheAndStaging(t *testing.T) {
	options := testOptions(t)
	dataDir, stagingDir, err := prepareManagedDirectory(options.CacheDir)
	if err != nil {
		t.Fatal(err)
	}
	cache, err := buildCache(dataDir, options.CacheTargetBytes)
	if err != nil {
		t.Fatal(err)
	}
	request := previewRequest(pngFixture(t))
	digest := identityDigest(request)
	key := cacheKey(digest)
	if _, err := cache.Put(key, []byte("prior"), options.CacheMaxAge); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stagingDir, "orphan"), []byte("prior"), 0o600); err != nil {
		t.Fatal(err)
	}
	service := New(options)
	if !service.Available() {
		t.Skip("libvips is unavailable")
	}
	if items := service.cache.Stats().Items; items != 0 {
		t.Fatalf("startup cache entries = %d, want 0", items)
	}
	if entries, err := os.ReadDir(service.stagingDir); err != nil {
		t.Fatal(err)
	} else if len(entries) != 0 {
		t.Fatalf("startup staging entries = %d, want 0", len(entries))
	}
	service.Close()
}

func TestServiceJanitorDeletesTrackedExpiredEntry(t *testing.T) {
	service := New(testOptions(t))
	if !service.Available() {
		t.Skip("libvips is unavailable")
	}
	request := previewRequest(pngFixture(t))
	result, err := service.Open(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	_ = result.Reader.Close()
	key := cacheKey(identityDigest(request))
	service.trackedMu.Lock()
	service.tracked[key] = time.Now().Add(-time.Second)
	service.trackedMu.Unlock()
	service.cleanupExpired()
	if _, _, err := service.cache.GetReader(key); !errors.Is(err, fcache.ErrNotFound) {
		t.Fatalf("expired cache lookup error = %v, want not found", err)
	}
	service.Close()
}

func TestCacheEvictsWhenTargetSizeIsExceeded(t *testing.T) {
	root, err := os.MkdirTemp("/var/tmp", "roaminal-image-preview-cache-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(root) })
	dataDir, _, err := prepareManagedDirectory(filepath.Join(root, "cache"))
	if err != nil {
		t.Fatal(err)
	}
	cache, err := buildCache(dataDir, 4)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := cache.Put(1, []byte("1234"), 0); err != nil {
		t.Fatal(err)
	}
	if _, err := cache.Put(2, []byte("5678"), 0); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	for cache.Stats().Evictions == 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	stats := cache.Stats()
	if stats.Evictions == 0 {
		t.Fatalf("cache eviction did not complete: %+v", stats)
	}
	if stats.Bytes > 4 || stats.Items > 1 {
		t.Fatalf("cache exceeded target after eviction: %+v", stats)
	}
}

func TestManagedCacheRequiresPrivateOwnedPaths(t *testing.T) {
	root, err := os.MkdirTemp("/var/tmp", "roaminal-image-preview-managed-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(root) })
	if err := os.Chmod(root, 0o755); err != nil {
		t.Fatal(err)
	}
	dataDir, stagingDir, err := prepareManagedDirectory(root)
	if err != nil {
		t.Fatal(err)
	}
	for name, pathValue := range map[string]string{
		"root":    root,
		"marker":  filepath.Join(root, markerName),
		"fcache":  dataDir,
		"staging": stagingDir,
	} {
		t.Run(name, func(t *testing.T) {
			info, statErr := os.Stat(pathValue)
			if statErr != nil {
				t.Fatal(statErr)
			}
			want := os.FileMode(0o700)
			if name == "marker" {
				want = 0o600
			}
			if got := info.Mode().Perm(); got != want {
				t.Fatalf("mode = %o, want %o", got, want)
			}
		})
	}
}

func TestManagedCacheRejectsMarkerSymlink(t *testing.T) {
	root, err := os.MkdirTemp("/var/tmp", "roaminal-image-preview-marker-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(root) })
	target := filepath.Join(root, "target")
	if err := os.WriteFile(target, []byte(markerContent), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(root, markerName)); err != nil {
		t.Fatal(err)
	}
	if _, _, err := prepareManagedDirectory(root); err == nil {
		t.Fatal("expected marker symlink to be rejected")
	}
}

func TestServiceRejectsPixelFrameAndOutputLimitsWithoutCacheEntry(t *testing.T) {
	data := pngFixture(t)
	for name, configure := range map[string]func(*Options){
		"static pixels":   func(options *Options) { options.MaxStaticPixels = 1 },
		"output bytes":    func(options *Options) { options.MaxOutputBytes = 1 },
		"frames":          func(options *Options) { options.MaxFrames = 1 },
		"animated pixels": func(options *Options) { options.MaxAnimatedPixels = 1 },
	} {
		t.Run(name, func(t *testing.T) {
			options := testOptions(t)
			configure(&options)
			service := New(options)
			if !service.Available() {
				t.Skip("libvips is unavailable")
			}
			request := previewRequest(data)
			if name == "frames" || name == "animated pixels" {
				animated := animatedGIFFixture(t)
				request = Request{
					ConnectionInstanceID: "instance", RootAbsolutePath: "/workspace", RootRevision: "root-1",
					RelativePath: "animation.gif", MIMEType: "image/gif", SourceSize: int64(len(animated)),
					SourceToken: "animation", Open: func(context.Context) (io.ReadCloser, error) {
						return io.NopCloser(bytes.NewReader(animated)), nil
					},
				}
			}
			if _, err := service.Open(context.Background(), request); !errors.Is(err, ErrInvalid) {
				t.Fatalf("error = %v, want invalid", err)
			}
			if items := service.cache.Stats().Items; items != 0 {
				t.Fatalf("cache entries = %d, want 0", items)
			}
			service.Close()
		})
	}
}

func firstCacheFile(t *testing.T, directory string) string {
	t.Helper()
	var result string
	err := filepath.WalkDir(directory, func(pathValue string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if result == "" && !entry.IsDir() {
			result = pathValue
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if result == "" {
		t.Fatal("cache entry not found")
	}
	return result
}

func cacheKey(digest [32]byte) uint64 {
	return uint64(digest[0])<<56 | uint64(digest[1])<<48 | uint64(digest[2])<<40 | uint64(digest[3])<<32 | uint64(digest[4])<<24 | uint64(digest[5])<<16 | uint64(digest[6])<<8 | uint64(digest[7])
}
