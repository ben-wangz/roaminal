package imagepreview

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"sync"
	"time"

	"github.com/ydylla/fcache"
)

type Service struct {
	options              Options
	logger               logger
	cache                fcache.Cache
	cacheDir             string
	stagingDir           string
	slots                chan struct{}
	done                 chan struct{}
	wait                 sync.WaitGroup
	closeOnce            sync.Once
	trackedMu            sync.Mutex
	tracked              map[uint64]time.Time
	cleanupErrors        map[uint64]bool
	evictionMu           sync.Mutex
	evictionErrorsLogged int
	enabled              bool
}

type logger interface {
	Printf(string, ...any)
}

func (s *Service) Open(ctx context.Context, request Request) (Result, error) {
	if s == nil || !s.enabled || s.cache == nil {
		return Result{}, ErrUnavailable
	}
	if ctx == nil {
		ctx = context.Background()
	}
	defer s.observeEvictionErrors()
	request = request.normalized()
	if err := validateRequest(request); err != nil {
		return Result{}, err
	}
	if !eligibleMIME(request.MIMEType) {
		return Result{}, ErrUnsupported
	}
	digest := identityDigest(request)
	key := binary.BigEndian.Uint64(digest[:8])
	etag := `"` + hex.EncodeToString(digest[:]) + `"`
	requestCtx, cancel := context.WithTimeout(ctx, s.options.ConversionTimeout)
	defer cancel()

	reader, info, err := s.cache.GetReader(key)
	if err == nil {
		if validateCachedReader(reader, info.Size) == nil {
			return Result{Reader: reader, Size: info.Size, ETag: etag, Hit: true}, nil
		}
		_ = reader.Close()
		if _, deleteErr := s.cache.Delete(key); deleteErr != nil {
			s.logger.Printf("level=INFO event=filesystem_image_preview_cache_corruption_cleanup_failed cache_key=%s error_type=%T", cacheKeyPrefix(digest), deleteErr)
		}
		err = fcache.ErrNotFound
	}
	if !errors.Is(err, fcache.ErrNotFound) {
		s.logger.Printf("level=INFO event=filesystem_image_preview_cache_read_failed cache_key=%s error_type=%T", cacheKeyPrefix(digest), err)
		if _, deleteErr := s.cache.Delete(key); deleteErr != nil {
			s.logger.Printf("level=INFO event=filesystem_image_preview_cache_corruption_cleanup_failed cache_key=%s error_type=%T", cacheKeyPrefix(digest), deleteErr)
		}
	}

	reader, info, hit, err := s.cache.GetReaderOrPut(key, s.options.CacheMaxAge, fcache.FillerFunc(func(_ uint64, sink io.Writer) (int64, error) {
		return s.fill(requestCtx, request, sink, digest)
	}))
	if err != nil {
		s.logger.Printf("level=INFO event=filesystem_image_preview_conversion_failed cache_key=%s error_type=%T", cacheKeyPrefix(digest), err)
		return Result{}, err
	}
	if hit && validateCachedReader(reader, info.Size) != nil {
		_ = reader.Close()
		if _, deleteErr := s.cache.Delete(key); deleteErr != nil {
			s.logger.Printf("level=INFO event=filesystem_image_preview_cache_corruption_cleanup_failed cache_key=%s error_type=%T", cacheKeyPrefix(digest), deleteErr)
			return Result{}, fmt.Errorf("%w: corrupt cache entry", ErrUnavailable)
		}
		reader, info, hit, err = s.cache.GetReaderOrPut(key, s.options.CacheMaxAge, fcache.FillerFunc(func(_ uint64, sink io.Writer) (int64, error) {
			return s.fill(requestCtx, request, sink, digest)
		}))
		if err != nil {
			s.logger.Printf("level=INFO event=filesystem_image_preview_conversion_failed cache_key=%s error_type=%T", cacheKeyPrefix(digest), err)
			return Result{}, err
		}
		if hit && validateCachedReader(reader, info.Size) != nil {
			_ = reader.Close()
			_, _ = s.cache.Delete(key)
			return Result{}, fmt.Errorf("%w: corrupt cache entry after regeneration", ErrUnavailable)
		}
	}
	if !hit {
		s.track(key, info.Expires)
	}
	return Result{Reader: reader, Size: info.Size, ETag: etag, Hit: hit}, nil
}

func (s *Service) fill(ctx context.Context, request Request, sink io.Writer, digest [sha256.Size]byte) (int64, error) {
	if request.SourceSize > s.options.MaxSourceBytes {
		return 0, fmt.Errorf("%w: source exceeds byte limit", ErrInvalid)
	}
	select {
	case s.slots <- struct{}{}:
		defer func() { <-s.slots }()
	case <-ctx.Done():
		return 0, ctx.Err()
	}
	started := time.Now()
	file, err := os.CreateTemp(s.stagingDir, "source-")
	if err != nil {
		return 0, fmt.Errorf("%w: create staging file", ErrUnavailable)
	}
	pathValue := file.Name()
	_ = file.Chmod(0o600)
	defer os.Remove(pathValue)
	defer file.Close()

	if request.Open == nil {
		return 0, fmt.Errorf("%w: source opener is missing", ErrUnavailable)
	}
	remote, err := request.Open(ctx)
	if err != nil {
		return 0, err
	}
	written, copyErr := copyAtMost(ctx, file, remote, s.options.MaxSourceBytes)
	closeErr := remote.Close()
	if copyErr != nil {
		return 0, copyErr
	}
	if closeErr != nil {
		return 0, closeErr
	}
	if written > s.options.MaxSourceBytes {
		return 0, fmt.Errorf("%w: source exceeds byte limit", ErrInvalid)
	}
	if written != request.SourceSize {
		return 0, fmt.Errorf("%w: source changed while staging", ErrInvalid)
	}
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	if request.Validate != nil {
		if err := request.Validate(ctx); err != nil {
			return 0, err
		}
	}
	if err := file.Close(); err != nil {
		return 0, err
	}
	data, details, err := convertFile(ctx, pathValue, request.MIMEType, s.options)
	if err != nil {
		return 0, err
	}
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	if int64(len(data)) > s.options.MaxOutputBytes {
		return 0, fmt.Errorf("%w: output exceeds byte limit", ErrInvalid)
	}
	written, err = io.Copy(sink, bytes.NewReader(data))
	if err != nil {
		return int64(written), err
	}
	if written != int64(len(data)) {
		return int64(written), io.ErrShortWrite
	}
	s.logger.Printf("level=INFO event=filesystem_image_preview_converted cache_key=%s source_bytes=%d output_bytes=%d width=%d height=%d frames=%d duration_ms=%d", cacheKeyPrefix(digest), request.SourceSize, len(data), details.Width, details.Height, details.Frames, time.Since(started).Milliseconds())
	return int64(written), nil
}

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r contextReader) Read(buffer []byte) (int, error) {
	select {
	case <-r.ctx.Done():
		return 0, r.ctx.Err()
	default:
	}
	return r.reader.Read(buffer)
}

func copyAtMost(ctx context.Context, dst io.Writer, src io.Reader, limit int64) (int64, error) {
	if limit < 0 || limit >= math.MaxInt64 {
		return 0, ErrInvalid
	}
	return io.Copy(dst, io.LimitReader(contextReader{ctx: ctx, reader: src}, limit+1))
}
