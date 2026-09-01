package imagepreview

import (
	"context"
	"fmt"
	"math"
	"path/filepath"
	"sync"

	"github.com/davidbyttow/govips/v2/vips"
)

var (
	vipsStartOnce    sync.Once
	vipsStartErr     error
	vipsShutdownOnce sync.Once
)

func startVips(concurrency int) error {
	vipsStartOnce.Do(func() {
		vipsStartErr = vips.Startup(&vips.Config{ConcurrencyLevel: concurrency, MaxCacheMem: 0, MaxCacheFiles: 0, MaxCacheSize: 0})
	})
	return vipsStartErr
}

// ShutdownVips is called once by the process composition root after all
// image-preview services have stopped. govips cannot be started again after
// Shutdown, so a service Close intentionally stops only its janitor.
func ShutdownVips() {
	vipsShutdownOnce.Do(func() {
		vips.Shutdown()
	})
}

func vipsVersion() string { return vips.Version }

type imageDetails struct {
	Width  int
	Height int
	Frames int
}

func convertFile(ctx context.Context, file string, options Options) ([]byte, imageDetails, error) {
	if err := ctx.Err(); err != nil {
		return nil, imageDetails{}, err
	}
	params := vips.NewImportParams()
	params.FailOnError.Set(true)
	if options.MaxFrames < math.MaxInt-1 {
		params.NumPages.Set(options.MaxFrames + 1)
	}
	image, err := vips.LoadImageFromFile(filepath.Clean(file), params)
	if err != nil {
		return nil, imageDetails{}, fmt.Errorf("%w: decode image", ErrInvalid)
	}
	defer image.Close()
	frames := image.Pages()
	if frames < 1 {
		frames = 1
	}
	pageHeight := image.PageHeight()
	if pageHeight < 1 {
		pageHeight = image.Height()
	}
	if frames > options.MaxFrames {
		return nil, imageDetails{}, fmt.Errorf("%w: frame limit", ErrInvalid)
	}
	framePixels, ok := pixelProduct(image.Width(), pageHeight)
	if !ok {
		return nil, imageDetails{}, fmt.Errorf("%w: image dimensions overflow", ErrInvalid)
	}
	if frames == 1 {
		if framePixels > options.MaxStaticPixels {
			return nil, imageDetails{}, fmt.Errorf("%w: static pixel limit", ErrInvalid)
		}
	} else {
		totalPixels, ok := multiply(framePixels, uint64(frames))
		if !ok || totalPixels > options.MaxAnimatedPixels {
			return nil, imageDetails{}, fmt.Errorf("%w: animated pixel limit", ErrInvalid)
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, imageDetails{}, err
	}
	if err := image.AutoRotate(); err != nil {
		return nil, imageDetails{}, fmt.Errorf("%w: orientation normalization", ErrInvalid)
	}
	if image.ColorSpace() != vips.InterpretationSRGB && image.ColorSpace() != vips.InterpretationBW {
		if err := image.ToColorSpace(vips.InterpretationSRGB); err != nil {
			return nil, imageDetails{}, fmt.Errorf("%w: color conversion", ErrInvalid)
		}
	}
	if err := image.RemoveMetadata(); err != nil {
		return nil, imageDetails{}, fmt.Errorf("%w: metadata removal", ErrInvalid)
	}
	if err := image.RemoveICCProfile(); err != nil {
		return nil, imageDetails{}, fmt.Errorf("%w: profile removal", ErrInvalid)
	}
	if err := ctx.Err(); err != nil {
		return nil, imageDetails{}, err
	}
	data, _, err := image.ExportWebp(&vips.WebpExportParams{StripMetadata: true, Quality: outputQuality, Lossless: false, ReductionEffort: outputEffort})
	if err != nil {
		return nil, imageDetails{}, fmt.Errorf("%w: WebP encoding", ErrInvalid)
	}
	if int64(len(data)) > options.MaxOutputBytes {
		return nil, imageDetails{}, fmt.Errorf("%w: output byte limit", ErrInvalid)
	}
	return data, imageDetails{Width: image.Width(), Height: image.PageHeight(), Frames: image.Pages()}, nil
}

func pixelProduct(width, height int) (uint64, bool) {
	if width <= 0 || height <= 0 {
		return 0, false
	}
	return multiply(uint64(width), uint64(height))
}

func multiply(left, right uint64) (uint64, bool) {
	if left != 0 && right > math.MaxUint64/left {
		return 0, false
	}
	return left * right, true
}

func eligibleMIME(value string) bool {
	switch value {
	case "image/jpeg", "image/png", "image/gif", "image/webp", "image/tiff", "image/avif", "image/heic", "image/heif", "image/jxl", "image/bmp", "image/x-icon":
		return true
	default:
		return false
	}
}
