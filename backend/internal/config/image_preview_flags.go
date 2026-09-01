package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

func applyFilesystemImagePreviewArg(c *Config, key, value string) error {
	switch key {
	case "--filesystem-image-preview-cache-dir":
		c.FilesystemImagePreviewCacheDir = value
	case "--filesystem-image-preview-cache-target-mib":
		n, err := strconv.Atoi(value)
		if err != nil {
			return err
		}
		c.FilesystemImagePreviewCacheTargetMiB = n
	case "--filesystem-image-preview-cache-max-age":
		d, err := time.ParseDuration(value)
		if err != nil {
			return err
		}
		c.FilesystemImagePreviewCacheMaxAge = d
	case "--filesystem-image-preview-cache-cleanup-interval":
		d, err := time.ParseDuration(value)
		if err != nil {
			return err
		}
		c.FilesystemImagePreviewCacheCleanupInterval = d
	case "--filesystem-image-preview-max-conversions":
		n, err := strconv.Atoi(value)
		if err != nil {
			return err
		}
		c.FilesystemImagePreviewMaxConversions = n
	case "--filesystem-image-preview-max-source-mib":
		n, err := strconv.Atoi(value)
		if err != nil {
			return err
		}
		c.FilesystemImagePreviewMaxSourceMiB = n
	case "--filesystem-image-preview-max-output-mib":
		n, err := strconv.Atoi(value)
		if err != nil {
			return err
		}
		c.FilesystemImagePreviewMaxOutputMiB = n
	case "--filesystem-image-preview-max-static-pixels":
		n, err := strconv.ParseUint(value, 10, 64)
		if err != nil {
			return err
		}
		c.FilesystemImagePreviewMaxStaticPixels = n
	case "--filesystem-image-preview-max-frames":
		n, err := strconv.Atoi(value)
		if err != nil {
			return err
		}
		c.FilesystemImagePreviewMaxFrames = n
	case "--filesystem-image-preview-max-animated-pixels":
		n, err := strconv.ParseUint(value, 10, 64)
		if err != nil {
			return err
		}
		c.FilesystemImagePreviewMaxAnimatedPixels = n
	case "--filesystem-image-preview-conversion-timeout":
		d, err := time.ParseDuration(value)
		if err != nil {
			return err
		}
		c.FilesystemImagePreviewConversionTimeout = d
	default:
		return fmt.Errorf("unknown image preview argument %s", key)
	}
	return nil
}

func applyFilesystemImagePreviewEnv(c *Config) error {
	if value, ok := os.LookupEnv("ROAMINAL_FILESYSTEM_IMAGE_PREVIEW_CACHE_DIR"); ok {
		c.FilesystemImagePreviewCacheDir = value
	}
	if err := setIntEnv("ROAMINAL_FILESYSTEM_IMAGE_PREVIEW_CACHE_TARGET_MIB", &c.FilesystemImagePreviewCacheTargetMiB); err != nil {
		return err
	}
	if err := setDurationEnv("ROAMINAL_FILESYSTEM_IMAGE_PREVIEW_CACHE_MAX_AGE", &c.FilesystemImagePreviewCacheMaxAge); err != nil {
		return err
	}
	if err := setDurationEnv("ROAMINAL_FILESYSTEM_IMAGE_PREVIEW_CACHE_CLEANUP_INTERVAL", &c.FilesystemImagePreviewCacheCleanupInterval); err != nil {
		return err
	}
	if err := setIntEnv("ROAMINAL_FILESYSTEM_IMAGE_PREVIEW_MAX_CONVERSIONS", &c.FilesystemImagePreviewMaxConversions); err != nil {
		return err
	}
	if err := setIntEnv("ROAMINAL_FILESYSTEM_IMAGE_PREVIEW_MAX_SOURCE_MIB", &c.FilesystemImagePreviewMaxSourceMiB); err != nil {
		return err
	}
	if err := setIntEnv("ROAMINAL_FILESYSTEM_IMAGE_PREVIEW_MAX_OUTPUT_MIB", &c.FilesystemImagePreviewMaxOutputMiB); err != nil {
		return err
	}
	if err := setUint64Env("ROAMINAL_FILESYSTEM_IMAGE_PREVIEW_MAX_STATIC_PIXELS", &c.FilesystemImagePreviewMaxStaticPixels); err != nil {
		return err
	}
	if err := setIntEnv("ROAMINAL_FILESYSTEM_IMAGE_PREVIEW_MAX_FRAMES", &c.FilesystemImagePreviewMaxFrames); err != nil {
		return err
	}
	if err := setUint64Env("ROAMINAL_FILESYSTEM_IMAGE_PREVIEW_MAX_ANIMATED_PIXELS", &c.FilesystemImagePreviewMaxAnimatedPixels); err != nil {
		return err
	}
	return setDurationEnv("ROAMINAL_FILESYSTEM_IMAGE_PREVIEW_CONVERSION_TIMEOUT", &c.FilesystemImagePreviewConversionTimeout)
}

func setIntEnv(name string, target *int) error {
	value, ok := os.LookupEnv(name)
	if !ok {
		return nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	*target = parsed
	return nil
}

func setUint64Env(name string, target *uint64) error {
	value, ok := os.LookupEnv(name)
	if !ok {
		return nil
	}
	parsed, err := strconv.ParseUint(value, 10, 64)
	if err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	*target = parsed
	return nil
}

func setDurationEnv(name string, target *time.Duration) error {
	value, ok := os.LookupEnv(name)
	if !ok {
		return nil
	}
	parsed, err := time.ParseDuration(value)
	if err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	*target = parsed
	return nil
}
