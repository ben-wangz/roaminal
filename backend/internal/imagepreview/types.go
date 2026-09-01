package imagepreview

import (
	"context"
	"errors"
	"io"
	"log"
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

func (o Options) logger() *log.Logger {
	if o.Logger != nil {
		return o.Logger
	}
	return log.Default()
}
