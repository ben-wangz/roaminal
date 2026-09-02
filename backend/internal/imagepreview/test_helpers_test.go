package imagepreview

import (
	"bytes"
	"context"
	"encoding/binary"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"
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

func min(left, right int) int {
	if left < right {
		return left
	}
	return right
}
