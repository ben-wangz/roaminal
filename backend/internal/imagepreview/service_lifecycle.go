package imagepreview

import (
	"time"
)

func New(options Options) *Service {
	s := &Service{
		options:       options,
		logger:        options.logger(),
		done:          make(chan struct{}),
		tracked:       make(map[uint64]time.Time),
		cleanupErrors: make(map[uint64]bool),
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

func (s *Service) Available() bool { return s != nil && s.enabled }

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
			s.observeEvictionErrors()
			s.cleanupExpired()
		case <-s.done:
			return
		}
	}
}

func (s *Service) observeEvictionErrors() {
	if s == nil || s.cache == nil {
		return
	}
	errorsFound := s.cache.Stats().EvictionErrors
	s.evictionMu.Lock()
	defer s.evictionMu.Unlock()
	if len(errorsFound) < s.evictionErrorsLogged {
		s.evictionErrorsLogged = 0
	}
	for _, eviction := range errorsFound[s.evictionErrorsLogged:] {
		s.logger.Printf("level=INFO event=filesystem_image_preview_cache_eviction_failed error_type=%T", eviction.Error)
	}
	s.evictionErrorsLogged = len(errorsFound)
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
			s.trackedMu.Lock()
			firstFailure := !s.cleanupErrors[key]
			s.cleanupErrors[key] = true
			s.trackedMu.Unlock()
			if firstFailure {
				s.logger.Printf("level=INFO event=filesystem_image_preview_janitor_delete_failed cache_key=%d error_type=%T", key, err)
			}
			continue
		}
		s.trackedMu.Lock()
		delete(s.tracked, key)
		delete(s.cleanupErrors, key)
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
