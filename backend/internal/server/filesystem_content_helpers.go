package server

import (
	"fmt"
	"io"
	"mime"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/ben-wangz/roaminal/backend/internal/filesystem"
)

func setContentHeaders(w http.ResponseWriter, contentType string, length int64, etag string, originalVariant bool, modifiedAt *time.Time) {
	w.Header().Set("Accept-Ranges", "bytes")
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Length", strconv.FormatInt(length, 10))
	w.Header().Set("ETag", etag)
	if modifiedAt != nil {
		w.Header().Set("Last-Modified", modifiedAt.UTC().Format(http.TimeFormat))
	}
	if originalVariant {
		w.Header().Set("X-Roaminal-Image-Variant", "original")
	}
}

func writeContent(w http.ResponseWriter, r *http.Request, reader io.Reader, start, length, total int64) {
	status := http.StatusOK
	if r.Header.Get("Range") != "" {
		status = http.StatusPartialContent
		w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, start+length-1, total))
	}
	w.WriteHeader(status)
	_, _ = io.CopyN(w, reader, length)
}

func setAttachment(w http.ResponseWriter, name string) {
	filename := strings.ReplaceAll(strings.ReplaceAll(name, `"`, ""), "\r", "")
	filename = strings.ReplaceAll(filename, "\n", "")
	if filename == "" {
		filename = "download"
	}
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": filename}))
}

func ifNoneMatch(header, etag string) bool {
	for _, candidate := range strings.Split(header, ",") {
		candidate = strings.TrimSpace(candidate)
		if candidate == "*" || candidate == etag || strings.TrimPrefix(candidate, "W/") == etag {
			return true
		}
	}
	return false
}

func contentRange(value string, size int64) (int64, int64, bool, error) {
	if value == "" {
		return 0, size, false, nil
	}
	if !strings.HasPrefix(value, "bytes=") || strings.Contains(value[6:], ",") {
		return 0, 0, false, filesystem.ErrInvalidRange
	}
	value = strings.TrimSpace(strings.TrimPrefix(value, "bytes="))
	parts := strings.Split(value, "-")
	if len(parts) != 2 || size == 0 {
		return 0, 0, false, filesystem.ErrInvalidRange
	}
	if parts[0] == "" {
		suffix, err := strconv.ParseInt(parts[1], 10, 64)
		if err != nil || suffix <= 0 {
			return 0, 0, false, filesystem.ErrInvalidRange
		}
		if suffix > size {
			suffix = size
		}
		return size - suffix, suffix, true, nil
	}
	start, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || start < 0 || start >= size {
		return 0, 0, false, filesystem.ErrInvalidRange
	}
	end := size - 1
	if parts[1] != "" {
		end, err = strconv.ParseInt(parts[1], 10, 64)
		if err != nil || end < start {
			return 0, 0, false, filesystem.ErrInvalidRange
		}
		if end >= size {
			end = size - 1
		}
	}
	length := end - start + 1
	if length > 8<<20 {
		return 0, 0, false, filesystem.ErrContentTooLarge
	}
	return start, length, true, nil
}

func contentWindowLength(contentType string, size int64) int64 {
	limit := int64(8 << 20)
	if strings.HasPrefix(contentType, "text/") || contentType == "application/json" || contentType == "application/xml" {
		limit = 1 << 20
	}
	if size < limit {
		return size
	}
	return limit
}
