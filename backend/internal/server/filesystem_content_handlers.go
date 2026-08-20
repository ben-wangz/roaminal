package server

import (
	"fmt"
	"io"
	"mime"
	"net/http"
	"strconv"
	"strings"

	"github.com/ben-wangz/roaminal/backend/internal/filesystem"
)

func (s *Server) filesystemContent(w http.ResponseWriter, r *http.Request, _ string) {
	if s.filesystem == nil {
		writeFilesystemError(w, filesystem.ErrUnsupported)
		return
	}
	id := r.PathValue("connectionInstanceId")
	pathValue := r.URL.Query().Get("path")
	revision := r.URL.Query().Get("rootRevision")
	entry, root, err := s.filesystem.Stat(r.Context(), id, pathValue, revision)
	if err != nil {
		writeFilesystemError(w, err)
		return
	}
	if entry.Type != "file" || entry.Size == nil {
		writeFilesystemError(w, filesystem.ErrContentUnavailable)
		return
	}
	start, length, partial, rangeErr := contentRange(r.Header.Get("Range"), *entry.Size)
	if rangeErr != nil {
		writeFilesystemError(w, rangeErr)
		return
	}
	contentType := mimeTypeForEntry(entry.Name, entry.Type)
	if r.Header.Get("Range") == "" {
		if r.URL.Query().Get("download") == "1" {
			length = *entry.Size
		} else {
			length = contentWindowLength(contentType, *entry.Size)
		}
		partial = length < *entry.Size
	}
	stream, err := s.filesystem.OpenContent(r.Context(), id, pathValue, root.Revision, start, length)
	if err != nil {
		writeFilesystemError(w, err)
		return
	}
	defer stream.Reader.Close()
	if consistencyToken(stream.Entry) != consistencyToken(entry) {
		writeFilesystemError(w, filesystem.ErrContentUnavailable)
		return
	}
	w.Header().Set("Accept-Ranges", "bytes")
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Length", strconv.FormatInt(stream.ContentLength, 10))
	w.Header().Set("ETag", `"`+consistencyToken(entry)+`"`)
	if entry.ModifiedAt != nil {
		w.Header().Set("Last-Modified", entry.ModifiedAt.UTC().Format(http.TimeFormat))
	}
	if partial {
		w.Header().Set("X-Roaminal-Content-Truncated", "true")
	}
	if r.URL.Query().Get("download") == "1" {
		filename := strings.ReplaceAll(strings.ReplaceAll(entry.Name, `"`, ""), "\r", "")
		filename = strings.ReplaceAll(filename, "\n", "")
		if filename == "" {
			filename = "download"
		}
		w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": filename}))
	}
	status := http.StatusOK
	if r.Header.Get("Range") != "" {
		status = http.StatusPartialContent
		w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", stream.Start, stream.End, stream.TotalSize))
	}
	w.WriteHeader(status)
	_, _ = io.CopyN(w, stream.Reader, stream.ContentLength)
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
