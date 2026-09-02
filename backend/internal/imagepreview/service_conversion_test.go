package imagepreview

import (
	"bytes"
	"context"
	"io"
	"testing"

	"github.com/davidbyttow/govips/v2/vips"
)

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
