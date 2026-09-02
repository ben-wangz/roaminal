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
		"root": root, "marker": filepath.Join(root, markerName), "fcache": dataDir, "staging": stagingDir,
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
