package imagepreview

import (
	"context"
	"errors"
	"io"
	"log"
	"math"
	"strings"
	"time"
)

const (
	OutputContentType = "image/webp"
	PipelineVersion   = "filesystem-image-preview-v1"
	outputFormat      = "webp"
	outputQuality     = 75
	outputEffort      = 3
)

var (
	ErrUnsupported = errors.New("image preview is unsupported")
	ErrInvalid     = errors.New("image preview is unavailable for this image")
	ErrUnavailable = errors.New("image preview service is unavailable")
)

// Request contains an already-authorized source. The preview package does not
// know how a connection or remote path is resolved; the callbacks remain
// inside the FileSystem boundary.
type Request struct {
	ConnectionInstanceID string
	RootAbsolutePath     string
	RootRevision         string
	RelativePath         string
	MIMEType             string
	SourceSize           int64
	SourceToken          string
	Open                 func(context.Context) (io.ReadCloser, error)
	Validate             func(context.Context) error
}

type Result struct {
	Reader io.ReadSeekCloser
	Size   int64
	ETag   string
	Hit    bool
}

type Options struct {
	CacheDir          string
	CacheTargetBytes  int64
	CacheMaxAge       time.Duration
	CleanupInterval   time.Duration
	MaxConversions    int
	MaxSourceBytes    int64
	MaxOutputBytes    int64
	MaxStaticPixels   uint64
	MaxFrames         int
	MaxAnimatedPixels uint64
	ConversionTimeout time.Duration
	Logger            *log.Logger
}

func (o Options) validate() error {
	if !validCachePath(o.CacheDir) {
		return errors.New("cache directory must be an absolute private path outside reserved directories")
	}
	if o.CacheTargetBytes <= 0 || o.CacheTargetBytes >= math.MaxInt64 || o.CacheMaxAge < time.Minute || o.CacheMaxAge > 8760*time.Hour || o.CleanupInterval < time.Minute || o.CleanupInterval > 8760*time.Hour || o.MaxConversions <= 0 || o.MaxSourceBytes <= 0 || o.MaxOutputBytes <= 0 || o.MaxStaticPixels == 0 || o.MaxFrames <= 0 || o.MaxAnimatedPixels == 0 || o.ConversionTimeout < time.Second || o.ConversionTimeout >= 70*time.Second {
		return errors.New("image preview limits are invalid")
	}
	if o.MaxConversions > 1024 || o.MaxFrames > 100000 || o.MaxSourceBytes >= int64(^uint64(0)>>1) || o.MaxOutputBytes >= int64(^uint64(0)>>1) {
		return errors.New("image preview limits exceed supported bounds")
	}
	if o.MaxStaticPixels > 1<<48 || o.MaxAnimatedPixels > 1<<48 {
		return errors.New("image preview pixel limits exceed supported bounds")
	}
	return nil
}

func (r Request) normalized() Request {
	r.MIMEType = strings.ToLower(strings.TrimSpace(r.MIMEType))
	return r
}

func (o Options) logger() *log.Logger {
	if o.Logger != nil {
		return o.Logger
	}
	return log.Default()
}
