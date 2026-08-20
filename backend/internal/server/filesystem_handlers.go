package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"path"
	"strconv"
	"strings"

	"github.com/ben-wangz/roaminal/backend/internal/filesystem"
)

const filesystemUploadBodyLimit = int64(10<<30) + 16<<20

func (s *Server) filesystemRoot(w http.ResponseWriter, r *http.Request, _ string) {
	if s.filesystem == nil {
		writeFilesystemError(w, filesystem.ErrUnsupported)
		return
	}
	root, err := s.filesystem.Root(r.Context(), r.PathValue("connectionInstanceId"))
	if err != nil {
		writeFilesystemError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"connectionInstanceId": root.ConnectionInstanceID, "root": root})
}

func (s *Server) filesystemEntries(w http.ResponseWriter, r *http.Request, _ string) {
	if s.filesystem == nil {
		writeFilesystemError(w, filesystem.ErrUnsupported)
		return
	}
	limit := 0
	if value := r.URL.Query().Get("limit"); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil {
			writeFilesystemError(w, filesystem.ErrInvalidPath)
			return
		}
		limit = parsed
	}
	result, err := s.filesystem.Entries(r.Context(), r.PathValue("connectionInstanceId"), r.URL.Query().Get("path"), r.URL.Query().Get("rootRevision"), r.URL.Query().Get("cursor"), limit)
	if err != nil {
		writeFilesystemError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) filesystemStat(w http.ResponseWriter, r *http.Request, _ string) {
	if s.filesystem == nil {
		writeFilesystemError(w, filesystem.ErrUnsupported)
		return
	}
	entry, root, err := s.filesystem.Stat(r.Context(), r.PathValue("connectionInstanceId"), r.URL.Query().Get("path"), r.URL.Query().Get("rootRevision"))
	if err != nil {
		writeFilesystemError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"connectionInstanceId": root.ConnectionInstanceID,
		"rootRevision":         root.Revision,
		"entry":                entry,
		"mimeType":             mimeTypeForEntry(entry.Name, entry.Type),
		"encoding":             "utf-8",
		"capabilities":         map[string]bool{"read": entry.Type == "file", "range": entry.Type == "file", "stream": entry.Type == "file", "download": entry.Type == "file"},
		"consistencyToken":     consistencyToken(entry),
	})
}

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
		length = contentWindowLength(contentType, *entry.Size)
		partial = length < *entry.Size
	}
	stream, err := s.filesystem.OpenContent(r.Context(), id, pathValue, root.Revision, start, length)
	if err != nil {
		writeFilesystemError(w, err)
		return
	}
	defer stream.Reader.Close()
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
		filename := strings.ReplaceAll(strings.ReplaceAll(entry.Name, "\"", ""), "\r", "")
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

func (s *Server) filesystemCreateUpload(w http.ResponseWriter, r *http.Request, _ string) {
	if s.filesystem == nil {
		writeFilesystemError(w, filesystem.ErrUnsupported)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, filesystemUploadBodyLimit)
	if err := r.ParseMultipartForm(8 << 20); err != nil {
		if strings.Contains(err.Error(), "request body too large") || strings.Contains(err.Error(), "http: request body too large") {
			writeFilesystemError(w, filesystem.ErrContentTooLarge)
		} else {
			writeError(w, http.StatusBadRequest, "filesystem_upload_manifest_invalid")
		}
		return
	}
	values := r.MultipartForm.Value["manifest"]
	if len(values) != 1 {
		writeError(w, http.StatusBadRequest, "filesystem_upload_manifest_invalid")
		return
	}
	var manifest filesystem.UploadManifest
	decoder := json.NewDecoder(strings.NewReader(values[0]))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&manifest); err != nil {
		writeError(w, http.StatusBadRequest, "filesystem_upload_manifest_invalid")
		return
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		writeError(w, http.StatusBadRequest, "filesystem_upload_manifest_invalid")
		return
	}
	parts := make(map[string]*multipart.FileHeader)
	for name, headers := range r.MultipartForm.File {
		if len(headers) != 1 {
			writeError(w, http.StatusBadRequest, "filesystem_upload_manifest_invalid")
			return
		}
		parts[name] = headers[0]
	}
	status, err := s.filesystem.CreateUpload(r.Context(), r.PathValue("connectionInstanceId"), manifest, parts)
	if err != nil {
		writeFilesystemError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, status)
}

func (s *Server) filesystemGetUpload(w http.ResponseWriter, r *http.Request, _ string) {
	if s.filesystem == nil {
		writeFilesystemError(w, filesystem.ErrUnsupported)
		return
	}
	status, err := s.filesystem.UploadStatus(r.PathValue("connectionInstanceId"), r.PathValue("uploadId"))
	if err != nil {
		writeFilesystemError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, status)
}

func (s *Server) filesystemCancelUpload(w http.ResponseWriter, r *http.Request, _ string) {
	if s.filesystem == nil {
		writeFilesystemError(w, filesystem.ErrUnsupported)
		return
	}
	status, err := s.filesystem.CancelUpload(r.PathValue("connectionInstanceId"), r.PathValue("uploadId"))
	if err != nil {
		writeFilesystemError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, status)
}

func contentRange(value string, size int64) (int64, int64, bool, error) {
	if value == "" {
		return 0, size, false, nil
	}
	if !strings.HasPrefix(value, "bytes=") || strings.Contains(value[6:], ",") {
		return 0, 0, false, filesystem.ErrContentUnavailable
	}
	value = strings.TrimSpace(strings.TrimPrefix(value, "bytes="))
	parts := strings.Split(value, "-")
	if len(parts) != 2 || size == 0 {
		return 0, 0, false, filesystem.ErrContentUnavailable
	}
	if parts[0] == "" {
		suffix, err := strconv.ParseInt(parts[1], 10, 64)
		if err != nil || suffix <= 0 {
			return 0, 0, false, filesystem.ErrContentUnavailable
		}
		if suffix > size {
			suffix = size
		}
		return size - suffix, suffix, true, nil
	}
	start, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || start < 0 || start >= size {
		return 0, 0, false, filesystem.ErrContentUnavailable
	}
	end := size - 1
	if parts[1] != "" {
		end, err = strconv.ParseInt(parts[1], 10, 64)
		if err != nil || end < start {
			return 0, 0, false, filesystem.ErrContentUnavailable
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

func writeFilesystemError(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	code := "filesystem_internal_error"
	var rootChanged *filesystem.RootChangedError
	switch {
	case errors.As(err, &rootChanged):
		status, code = http.StatusConflict, "filesystem_root_changed"
		writeJSON(w, status, map[string]any{"error": "filesystem root changed", "code": code, "root": rootChanged.Root})
		return
	case errors.Is(err, filesystem.ErrUnsupported):
		status, code = http.StatusConflict, "filesystem_unsupported"
	case errors.Is(err, filesystem.ErrInstanceNotFound):
		status, code = http.StatusNotFound, "filesystem_instance_not_found"
	case errors.Is(err, filesystem.ErrNoTransport):
		status, code = http.StatusConflict, "filesystem_no_transport"
	case errors.Is(err, filesystem.ErrRootUnavailable):
		status, code = http.StatusConflict, "filesystem_root_unavailable"
	case errors.Is(err, filesystem.ErrInvalidPath):
		status, code = http.StatusBadRequest, "filesystem_path_invalid"
	case errors.Is(err, filesystem.ErrPathOutsideRoot):
		status, code = http.StatusForbidden, "filesystem_path_outside_root"
	case errors.Is(err, filesystem.ErrNotFound):
		status, code = http.StatusNotFound, "filesystem_not_found"
	case errors.Is(err, filesystem.ErrPermissionDenied):
		status, code = http.StatusForbidden, "filesystem_permission_denied"
	case errors.Is(err, filesystem.ErrFilenameEncoding):
		status, code = http.StatusInternalServerError, "filesystem_filename_encoding"
	case errors.Is(err, filesystem.ErrDirectoryTooLarge):
		status, code = http.StatusRequestEntityTooLarge, "filesystem_directory_too_large"
	case errors.Is(err, filesystem.ErrProtocol):
		status, code = http.StatusInternalServerError, "filesystem_protocol_error"
	case errors.Is(err, filesystem.ErrInvalidCursor):
		status, code = http.StatusBadRequest, "filesystem_path_invalid"
	case errors.Is(err, filesystem.ErrTimeout):
		status, code = http.StatusGatewayTimeout, "filesystem_timeout"
	case errors.Is(err, filesystem.ErrContentTooLarge):
		status, code = http.StatusRequestEntityTooLarge, "filesystem_content_too_large"
	case errors.Is(err, filesystem.ErrContentUnavailable):
		status, code = http.StatusConflict, "filesystem_content_unavailable"
	case errors.Is(err, filesystem.ErrUploadNotFound):
		status, code = http.StatusNotFound, "filesystem_upload_not_found"
	case errors.Is(err, filesystem.ErrUploadConflict):
		status, code = http.StatusConflict, "filesystem_upload_conflict"
	case errors.Is(err, filesystem.ErrUploadCancelled):
		status, code = http.StatusConflict, "filesystem_upload_cancelled"
	case errors.Is(err, filesystem.ErrUploadFailed):
		status, code = http.StatusInternalServerError, "filesystem_upload_failed"
	case errors.Is(err, filesystem.ErrListingFailed):
		status, code = http.StatusInternalServerError, "filesystem_listing_failed"
	}
	writeError(w, status, code)
}

func consistencyToken(entry filesystem.Entry) string {
	modified := ""
	if entry.ModifiedAt != nil {
		modified = entry.ModifiedAt.UTC().Format("20060102150405")
	}
	size := ""
	if entry.Size != nil {
		size = strconv.FormatInt(*entry.Size, 10)
	}
	return size + "-" + modified + "-" + strconv.FormatUint(uint64(entry.Mode), 10)
}

func mimeTypeForEntry(name, entryType string) string {
	if entryType != "file" {
		return "application/octet-stream"
	}
	value := mime.TypeByExtension(strings.ToLower(path.Ext(name)))
	if value == "" {
		return "application/octet-stream"
	}
	return value
}
