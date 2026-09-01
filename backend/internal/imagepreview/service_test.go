package imagepreview

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/davidbyttow/govips/v2/vips"
)

func testOptions(t *testing.T) Options {
	t.Helper()
	root, err := os.MkdirTemp("/var/tmp", "roaminal-image-preview-test-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(root) })
	return Options{
		CacheDir:          filepath.Join(root, "cache"),
		CacheTargetBytes:  1 << 20,
		CacheMaxAge:       time.Minute,
		CleanupInterval:   time.Minute,
		MaxConversions:    1,
		MaxSourceBytes:    1 << 20,
		MaxOutputBytes:    1 << 20,
		MaxStaticPixels:   1_000_000,
		MaxFrames:         10,
		MaxAnimatedPixels: 2_000_000,
		ConversionTimeout: 10 * time.Second,
	}
}

func pngFixture(t *testing.T) []byte {
	t.Helper()
	var output bytes.Buffer
	imageValue := image.NewNRGBA(image.Rect(0, 0, 4, 3))
	for y := 0; y < 3; y++ {
		for x := 0; x < 4; x++ {
			imageValue.SetNRGBA(x, y, color.NRGBA{R: uint8(x * 40), G: uint8(y * 60), B: 120, A: uint8(80 + x*40)})
		}
	}
	if err := png.Encode(&output, imageValue); err != nil {
		t.Fatal(err)
	}
	return output.Bytes()
}

func jpegWithOrientationFixture(t *testing.T, orientation uint16) []byte {
	t.Helper()
	imageValue := image.NewRGBA(image.Rect(0, 0, 2, 3))
	for y := 0; y < 3; y++ {
		for x := 0; x < 2; x++ {
			imageValue.Set(x, y, color.RGBA{R: uint8(x * 100), G: uint8(y * 70), B: 80, A: 255})
		}
	}
	var encoded bytes.Buffer
	if err := jpeg.Encode(&encoded, imageValue, &jpeg.Options{Quality: 90}); err != nil {
		t.Fatal(err)
	}
	exif := make([]byte, 6+8+2+12+4)
	copy(exif, []byte("Exif\x00\x00"))
	exif[6], exif[7] = 'I', 'I'
	binary.LittleEndian.PutUint16(exif[8:10], 42)
	binary.LittleEndian.PutUint32(exif[10:14], 8)
	binary.LittleEndian.PutUint16(exif[14:16], 1)
	binary.LittleEndian.PutUint16(exif[16:18], 0x0112)
	binary.LittleEndian.PutUint16(exif[18:20], 3)
	binary.LittleEndian.PutUint32(exif[20:24], 1)
	binary.LittleEndian.PutUint16(exif[24:26], orientation)
	segmentLength := len(exif) + 2
	output := bytes.NewBuffer(append([]byte(nil), encoded.Bytes()[:2]...))
	output.Write([]byte{0xff, 0xe1, byte(segmentLength >> 8), byte(segmentLength)})
	output.Write(exif)
	output.Write(encoded.Bytes()[2:])
	return output.Bytes()
}

func animatedGIFFixture(t *testing.T) []byte {
	t.Helper()
	frames := make([]*image.Paletted, 2)
	for index := range frames {
		frame := image.NewPaletted(image.Rect(0, 0, 3, 2), []color.Color{color.Black, color.White})
		for pixel := range frame.Pix {
			frame.Pix[pixel] = uint8((pixel + index) % 2)
		}
		frames[index] = frame
	}
	var output bytes.Buffer
	if err := gif.EncodeAll(&output, &gif.GIF{Image: frames, Delay: []int{4, 8}, LoopCount: 2}); err != nil {
		t.Fatal(err)
	}
	return output.Bytes()
}

func previewRequest(data []byte) Request {
	return Request{
		ConnectionInstanceID: "instance",
		RootAbsolutePath:     "/workspace",
		RootRevision:         "root-1",
		RelativePath:         "image.png",
		MIMEType:             "image/png",
		SourceSize:           int64(len(data)),
		SourceToken:          "token-1",
		Open: func(context.Context) (io.ReadCloser, error) {
			return io.NopCloser(bytes.NewReader(data)), nil
		},
	}
}

func TestServiceConvertsTransparentPNGAndCachesIt(t *testing.T) {
	data := pngFixture(t)
	service := New(testOptions(t))
	if !service.Available() {
		t.Skip("libvips is unavailable")
	}
	first, err := service.Open(context.Background(), previewRequest(data))
	if err != nil {
		t.Fatal(err)
	}
	firstBytes, err := io.ReadAll(first.Reader)
	_ = first.Reader.Close()
	if err != nil {
		t.Fatal(err)
	}
	if len(firstBytes) < 12 || string(firstBytes[:4]) != "RIFF" || string(firstBytes[8:12]) != "WEBP" {
		t.Fatalf("not WebP output: %x", firstBytes[:min(len(firstBytes), 12)])
	}
	decoded, err := vips.LoadImageFromBuffer(firstBytes, vips.NewImportParams())
	if err != nil {
		t.Fatal(err)
	}
	if decoded.Width() != 4 || decoded.Height() != 3 || !decoded.HasAlpha() {
		decoded.Close()
		t.Fatalf("decoded dimensions/alpha = %dx%d/%t, want 4x3/true", decoded.Width(), decoded.Height(), decoded.HasAlpha())
	}
	decoded.Close()
	second, err := service.Open(context.Background(), previewRequest(data))
	if err != nil {
		t.Fatal(err)
	}
	if !second.Hit || second.ETag != first.ETag {
		t.Fatalf("second request = hit %v etag %q, first etag %q", second.Hit, second.ETag, first.ETag)
	}
	secondBytes, err := io.ReadAll(second.Reader)
	_ = second.Reader.Close()
	if err != nil || !bytes.Equal(secondBytes, firstBytes) {
		t.Fatalf("cache hit returned different bytes: length=%d err=%v", len(secondBytes), err)
	}
	service.Close()
}

func TestServiceNormalizesJPEGOrientationWithoutResizing(t *testing.T) {
	data := jpegWithOrientationFixture(t, 6)
	service := New(testOptions(t))
	if !service.Available() {
		t.Skip("libvips is unavailable")
	}
	result, err := service.Open(context.Background(), Request{
		ConnectionInstanceID: "instance", RootAbsolutePath: "/workspace", RootRevision: "root-1",
		RelativePath: "photo.jpg", MIMEType: "image/jpeg", SourceSize: int64(len(data)), SourceToken: "jpeg-1",
		Open: func(context.Context) (io.ReadCloser, error) { return io.NopCloser(bytes.NewReader(data)), nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	output, err := io.ReadAll(result.Reader)
	_ = result.Reader.Close()
	if err != nil {
		t.Fatal(err)
	}
	params := vips.NewImportParams()
	decoded, err := vips.LoadImageFromBuffer(output, params)
	if err != nil {
		t.Fatal(err)
	}
	defer decoded.Close()
	if decoded.Width() != 3 || decoded.Height() != 2 {
		t.Fatalf("normalized dimensions = %dx%d, want 3x2", decoded.Width(), decoded.Height())
	}
	service.Close()
}

func TestServicePreservesAnimatedFrames(t *testing.T) {
	data := animatedGIFFixture(t)
	options := testOptions(t)
	options.MaxSourceBytes = int64(len(data)) + 1
	service := New(options)
	if !service.Available() {
		t.Skip("libvips is unavailable")
	}
	inputParams := vips.NewImportParams()
	inputParams.NumPages.Set(-1)
	inputImage, err := vips.LoadImageFromBuffer(data, inputParams)
	if err != nil {
		t.Fatal(err)
	}
	inputDelays, err := inputImage.PageDelay()
	if err != nil {
		inputImage.Close()
		t.Fatal(err)
	}
	inputPages, inputHeight, inputWidth, inputLoop := inputImage.Pages(), inputImage.PageHeight(), inputImage.Width(), inputImage.Loop()
	inputImage.Close()
	result, err := service.Open(context.Background(), Request{
		ConnectionInstanceID: "instance",
		RootAbsolutePath:     "/workspace",
		RootRevision:         "root-1",
		RelativePath:         "animation.gif",
		MIMEType:             "image/gif",
		SourceSize:           int64(len(data)),
		SourceToken:          "token-animation",
		Open:                 func(context.Context) (io.ReadCloser, error) { return io.NopCloser(bytes.NewReader(data)), nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	output, err := io.ReadAll(result.Reader)
	_ = result.Reader.Close()
	if err != nil || len(output) < 12 || string(output[:4]) != "RIFF" || string(output[8:12]) != "WEBP" {
		t.Fatalf("animated output invalid: bytes=%d err=%v", len(output), err)
	}
	params := vips.NewImportParams()
	params.NumPages.Set(-1)
	decoded, err := vips.LoadImageFromBuffer(output, params)
	if err != nil {
		t.Fatal(err)
	}
	defer decoded.Close()
	if decoded.Pages() != inputPages || decoded.PageHeight() != inputHeight || decoded.Width() != inputWidth {
		t.Fatalf("animated metadata = pages=%d pageHeight=%d width=%d, want %d/%d/%d", decoded.Pages(), decoded.PageHeight(), decoded.Width(), inputPages, inputHeight, inputWidth)
	}
	delays, err := decoded.PageDelay()
	if err != nil {
		t.Fatal(err)
	}
	if len(delays) != len(inputDelays) {
		t.Fatalf("animated delays = %v, want %v", delays, inputDelays)
	}
	for index := range delays {
		if delays[index] != inputDelays[index] {
			t.Fatalf("animated delay[%d] = %d, want %d", index, delays[index], inputDelays[index])
		}
	}
	if decoded.Loop() != inputLoop {
		t.Fatalf("animated loop = %d, want %d", decoded.Loop(), inputLoop)
	}
	service.Close()
}

func TestServiceRejectsUnsupportedAndOversizedSources(t *testing.T) {
	data := pngFixture(t)
	service := New(testOptions(t))
	if !service.Available() {
		t.Skip("libvips is unavailable")
	}
	unsupported := previewRequest(data)
	unsupported.MIMEType = "image/svg+xml"
	if !errors.Is(func() error { _, err := service.Open(context.Background(), unsupported); return err }(), ErrUnsupported) {
		t.Fatal("expected unsupported MIME error")
	}
	oversized := previewRequest(data)
	oversized.SourceSize++
	if !errors.Is(func() error { _, err := service.Open(context.Background(), oversized); return err }(), ErrInvalid) {
		t.Fatal("expected source byte limit error")
	}
	service.Close()
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

func min(left, right int) int {
	if left < right {
		return left
	}
	return right
}
