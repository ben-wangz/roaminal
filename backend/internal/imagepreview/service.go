package imagepreview

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/ydylla/fcache"
)

type Service struct {
	options    Options
	logger     logger
	cache      fcache.Cache
	cacheDir   string
	stagingDir string
	slots      chan struct{}
	done       chan struct{}
	wait       sync.WaitGroup
	closeOnce  sync.Once
	trackedMu  sync.Mutex
	tracked    map[uint64]time.Time
	enabled    bool
}

type logger interface {
	Printf(string, ...any)
}

func New(options Options) *Service {
	s := &Service{
		options: options,
		logger:  options.logger(),
		done:    make(chan struct{}),
		tracked: make(map[uint64]time.Time),
	}
	if err := options.validate(); err != nil {
		s.logger.Printf("level=INFO event=filesystem_image_preview_disabled reason=invalid_configuration error_type=%T", err)
		return s
	}
	if err := startVips(options.MaxConversions); err != nil {
		s.logger.Printf("level=INFO event=filesystem_image_preview_disabled reason=libvips_start_failed error_type=%T", err)
		return s
	}
	cacheDir, stagingDir, err := prepareManagedDirectory(options.CacheDir)
	if err != nil {
		s.logger.Printf("level=INFO event=filesystem_image_preview_disabled reason=cache_directory_unavailable error_type=%T", err)
		return s
	}
	cache, err := buildCache(cacheDir, options.CacheTargetBytes)
	if err != nil {
		s.logger.Printf("level=INFO event=filesystem_image_preview_cache_init_failed error_type=%T", err)
		return s
	}
	if err := cache.Clear(false); err != nil {
		if resetErr := replaceCacheData(cacheDir); resetErr == nil {
			cache, err = buildCache(cacheDir, options.CacheTargetBytes)
			if err == nil {
				err = cache.Clear(false)
			}
		}
		if err != nil {
			s.logger.Printf("level=INFO event=filesystem_image_preview_cache_clear_failed error_type=%T", err)
			return s
		}
	}
	if err := clearStaging(stagingDir); err != nil {
		s.logger.Printf("level=INFO event=filesystem_image_preview_staging_cleanup_failed error_type=%T", err)
	}
	s.cache = cache
	s.cacheDir = cacheDir
	s.stagingDir = stagingDir
	s.slots = make(chan struct{}, options.MaxConversions)
	s.enabled = true
	s.logger.Printf("level=INFO event=filesystem_image_preview_started cache_target_bytes=%d cache_max_age=%s cleanup_interval=%s max_conversions=%d max_source_bytes=%d max_output_bytes=%d max_static_pixels=%d max_frames=%d max_animated_pixels=%d conversion_timeout=%s libvips_version=%s", options.CacheTargetBytes, options.CacheMaxAge, options.CleanupInterval, options.MaxConversions, options.MaxSourceBytes, options.MaxOutputBytes, options.MaxStaticPixels, options.MaxFrames, options.MaxAnimatedPixels, options.ConversionTimeout, vipsVersion())
	s.wait.Add(1)
	go s.janitor()
	return s
}

func (o Options) validate() error {
	if !validCachePath(o.CacheDir) {
		return errors.New("cache directory must be an absolute private path outside reserved directories")
	}
	if o.CacheTargetBytes <= 0 || o.CacheMaxAge < time.Minute || o.CleanupInterval < time.Minute || o.MaxConversions <= 0 || o.MaxSourceBytes <= 0 || o.MaxOutputBytes <= 0 || o.MaxStaticPixels == 0 || o.MaxFrames <= 0 || o.MaxAnimatedPixels == 0 || o.ConversionTimeout <= 0 {
		return errors.New("image preview limits are invalid")
	}
	return nil
}

func (s *Service) Available() bool { return s != nil && s.enabled }

func (s *Service) Open(ctx context.Context, request Request) (Result, error) {
	if s == nil || !s.enabled || s.cache == nil {
		return Result{}, ErrUnavailable
	}
	if ctx == nil {
		ctx = context.Background()
	}
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
		return Result{Reader: reader, Size: info.Size, ETag: etag, Hit: true}, nil
	}
	if !errors.Is(err, fcache.ErrNotFound) {
		s.logger.Printf("level=INFO event=filesystem_image_preview_cache_read_failed cache_key=%s error_type=%T", cacheKeyPrefix(digest), err)
		if deleteErr := s.cache.Delete(key); deleteErr != nil {
			s.logger.Printf("level=INFO event=filesystem_image_preview_cache_corruption_cleanup_failed cache_key=%s error_type=%T", cacheKeyPrefix(digest), deleteErr)
		}
	}

	reader, info, hit, err := s.cache.GetReaderOrPut(key, s.options.CacheMaxAge, fcache.FillerFunc(func(_ uint64, sink io.Writer) (int64, error) {
		return s.fill(requestCtx, request, sink, digest)
	}))
	if err != nil {
		return Result{}, err
	}
	if !hit {
		s.track(key, info.Expires)
	}
	return Result{Reader: reader, Size: info.Size, ETag: etag, Hit: hit}, nil
}

func (s *Service) fill(ctx context.Context, request Request, sink io.Writer, digest [sha256.Size]byte) (int64, error) {
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

	if request.SourceSize > s.options.MaxSourceBytes {
		return 0, fmt.Errorf("%w: source exceeds byte limit", ErrInvalid)
	}
	if request.Open == nil {
		return 0, fmt.Errorf("%w: source opener is missing", ErrUnavailable)
	}
	remote, err := request.Open(ctx)
	if err != nil {
		return 0, err
	}
	written, copyErr := copyAtMost(file, remote, s.options.MaxSourceBytes)
	closeErr := remote.Close()
	if copyErr != nil {
		return 0, copyErr
	}
	if closeErr != nil {
		return 0, closeErr
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
	data, details, err := convertFile(ctx, pathValue, s.options)
	if err != nil {
		return 0, err
	}
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	if int64(len(data)) > s.options.MaxOutputBytes {
		return 0, fmt.Errorf("%w: output exceeds byte limit", ErrInvalid)
	}
	written, err = sink.Write(data)
	if err != nil {
		return int64(written), err
	}
	if written != len(data) {
		return int64(written), io.ErrShortWrite
	}
	s.logger.Printf("level=INFO event=filesystem_image_preview_converted cache_key=%s source_bytes=%d output_bytes=%d width=%d height=%d frames=%d duration_ms=%d", cacheKeyPrefix(digest), request.SourceSize, len(data), details.Width, details.Height, details.Frames, time.Since(started).Milliseconds())
	return int64(written), nil
}

func copyAtMost(dst io.Writer, src io.Reader, limit int64) (int64, error) {
	if limit < 0 {
		return 0, ErrInvalid
	}
	return io.Copy(dst, io.LimitReader(src, limit+1))
}

func validateRequest(request Request) error {
	if strings.TrimSpace(request.ConnectionInstanceID) == "" || strings.TrimSpace(request.RootAbsolutePath) == "" || strings.TrimSpace(request.RootRevision) == "" || strings.TrimSpace(request.SourceToken) == "" || request.SourceSize < 0 || request.Open == nil {
		return fmt.Errorf("%w: source descriptor is incomplete", ErrUnavailable)
	}
	clean := path.Clean(request.RelativePath)
	if clean == "." || path.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, "../") {
		return fmt.Errorf("%w: relative path is invalid", ErrUnavailable)
	}
	return nil
}

func identityDigest(request Request) [sha256.Size]byte {
	fields := []string{
		request.ConnectionInstanceID,
		request.RootAbsolutePath,
		request.RootRevision,
		path.Clean(request.RelativePath),
		request.SourceToken,
		outputFormat,
		fmt.Sprintf("%d", request.SourceSize),
		fmt.Sprintf("%d", outputQuality),
		fmt.Sprintf("%d", outputEffort),
		PipelineVersion,
	}
	var input strings.Builder
	for _, field := range fields {
		var length [8]byte
		binary.BigEndian.PutUint64(length[:], uint64(len(field)))
		input.Write(length[:])
		input.WriteString(field)
	}
	return sha256.Sum256([]byte(input.String()))
}

func cacheKeyPrefix(digest [sha256.Size]byte) string { return hex.EncodeToString(digest[:6]) }

func (s *Service) track(key uint64, expires time.Time) {
	if expires.IsZero() {
		return
	}
	s.trackedMu.Lock()
	s.tracked[key] = expires
	s.trackedMu.Unlock()
}

func (s *Service) janitor() {
	defer s.wait.Done()
	ticker := time.NewTicker(s.options.CleanupInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			s.cleanupExpired()
		case <-s.done:
			return
		}
	}
}

func (s *Service) cleanupExpired() {
	now := time.Now()
	keys := make([]uint64, 0)
	s.trackedMu.Lock()
	for key, expires := range s.tracked {
		if !now.Before(expires) {
			keys = append(keys, key)
		}
	}
	s.trackedMu.Unlock()
	for _, key := range keys {
		if _, err := s.cache.Delete(key); err != nil {
			s.logger.Printf("level=INFO event=filesystem_image_preview_janitor_delete_failed error_type=%T", err)
			continue
		}
		s.trackedMu.Lock()
		delete(s.tracked, key)
		s.trackedMu.Unlock()
	}
}

func (s *Service) Close() {
	if s == nil {
		return
	}
	s.closeOnce.Do(func() {
		close(s.done)
		s.wait.Wait()
	})
}
