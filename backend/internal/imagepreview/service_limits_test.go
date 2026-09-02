package imagepreview

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestServiceRejectsUnsupportedAndOversizedSources(t *testing.T) {
	data := pngFixture(t)
	service := New(testOptions(t))
	if !service.Available() {
		t.Skip("libvips is unavailable")
	}
	unsupported := previewRequest(data)
	unsupported.MIMEType = "image/svg+xml"
	if !errors.Is(func() error { _, err := service.Open(context.Background(), unsupported); return err }(), ErrUnsupported) {
		service.Close()
		t.Fatal("expected unsupported MIME error")
	}
	oversized := previewRequest(data)
	oversized.SourceSize++
	if !errors.Is(func() error { _, err := service.Open(context.Background(), oversized); return err }(), ErrInvalid) {
		service.Close()
		t.Fatal("expected source byte limit error")
	}
	service.Close()
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

func TestServiceCoalescesSameKeyAndLimitsDifferentConversions(t *testing.T) {
	data := pngFixture(t)
	options := testOptions(t)
	options.MaxConversions = 1
	service := New(options)
	if !service.Available() {
		t.Skip("libvips is unavailable")
	}
	var opens atomic.Int32
	request := previewRequest(data)
	request.Open = func(context.Context) (io.ReadCloser, error) {
		opens.Add(1)
		return io.NopCloser(bytes.NewReader(data)), nil
	}
	var group sync.WaitGroup
	results := make([]Result, 2)
	errorsFound := make([]error, 2)
	for index := range results {
		group.Add(1)
		go func(index int) {
			defer group.Done()
			results[index], errorsFound[index] = service.Open(context.Background(), request)
		}(index)
	}
	group.Wait()
	for index := range results {
		if errorsFound[index] != nil {
			t.Fatal(errorsFound[index])
		}
		_ = results[index].Reader.Close()
	}
	if opens.Load() != 1 {
		t.Fatalf("remote opens = %d, want 1", opens.Load())
	}
	service.Close()
}

func TestServiceLimitsDifferentConversions(t *testing.T) {
	data := pngFixture(t)
	options := testOptions(t)
	options.MaxConversions = 1
	service := New(options)
	if !service.Available() {
		t.Skip("libvips is unavailable")
	}
	var active, maximum atomic.Int32
	started := make(chan string, 2)
	release := make(chan struct{}, 2)
	open := func(path string) func(context.Context) (io.ReadCloser, error) {
		return func(ctx context.Context) (io.ReadCloser, error) {
			current := active.Add(1)
			for {
				previous := maximum.Load()
				if current <= previous || maximum.CompareAndSwap(previous, current) {
					break
				}
			}
			started <- path
			select {
			case <-release:
				active.Add(-1)
				return io.NopCloser(bytes.NewReader(data)), nil
			case <-ctx.Done():
				active.Add(-1)
				return nil, ctx.Err()
			}
		}
	}
	first := previewRequest(data)
	first.RelativePath = "first.png"
	first.Open = open(first.RelativePath)
	second := previewRequest(data)
	second.RelativePath = "second.png"
	second.Open = open(second.RelativePath)
	results := make(chan error, 2)
	go func() {
		result, err := service.Open(context.Background(), first)
		if err == nil {
			_ = result.Reader.Close()
		}
		results <- err
	}()
	if pathValue := <-started; pathValue != first.RelativePath {
		t.Fatalf("first opener was %q, want %q", pathValue, first.RelativePath)
	}
	go func() {
		result, err := service.Open(context.Background(), second)
		if err == nil {
			_ = result.Reader.Close()
		}
		results <- err
	}()
	select {
	case pathValue := <-started:
		t.Fatalf("second opener started before the first conversion finished: %q", pathValue)
	case <-time.After(100 * time.Millisecond):
	}
	release <- struct{}{}
	if pathValue := <-started; pathValue != second.RelativePath {
		t.Fatalf("second opener was %q, want %q", pathValue, second.RelativePath)
	}
	release <- struct{}{}
	for range 2 {
		if err := <-results; err != nil {
			t.Fatal(err)
		}
	}
	if maximum.Load() > 1 {
		t.Fatalf("maximum concurrent conversions = %d, want 1", maximum.Load())
	}
	service.Close()
}

func TestServiceHonorsConversionDeadline(t *testing.T) {
	options := testOptions(t)
	options.ConversionTimeout = time.Second
	service := New(options)
	if !service.Available() {
		t.Skip("libvips is unavailable")
	}
	request := previewRequest(pngFixture(t))
	request.Open = func(ctx context.Context) (io.ReadCloser, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	_, err := service.Open(context.Background(), request)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("deadline error = %v, want context deadline exceeded", err)
	}
	if items := service.cache.Stats().Items; items != 0 {
		t.Fatalf("cache entries = %d, want 0", items)
	}
	service.Close()
}

func TestServiceCleansStagingOnFailure(t *testing.T) {
	options := testOptions(t)
	data := pngFixture(t)
	options.MaxSourceBytes = int64(len(data))
	service := New(options)
	if !service.Available() {
		t.Skip("libvips is unavailable")
	}
	request := previewRequest(data[:len(data)/2])
	if _, err := service.Open(context.Background(), request); !errors.Is(err, ErrInvalid) {
		t.Fatalf("error = %v, want invalid source", err)
	}
	entries, err := os.ReadDir(service.stagingDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("staging entries remain: %d", len(entries))
	}
	service.Close()
}
