package server

import (
	"errors"
	"mime"
	"net/http"
	"path"
	"strconv"
	"strings"

	"github.com/ben-wangz/roaminal/backend/internal/filesystem"
)

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
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, filesystemRootResponse{ConnectionInstanceID: root.ConnectionInstanceID, Root: root})
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
	w.Header().Set("Cache-Control", "no-store")
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
	writeJSON(w, http.StatusOK, filesystemStatResponse{
		ConnectionInstanceID: root.ConnectionInstanceID,
		RootRevision:         root.Revision,
		Entry:                entry,
		MimeType:             mimeTypeForEntry(entry.Name, entry.Type),
		Encoding:             "utf-8",
		Capabilities:         filesystemCapabilities{Read: entry.Type == "file", Range: entry.Type == "file", Stream: entry.Type == "file", Download: entry.Type == "file"},
		ConsistencyToken:     consistencyToken(entry),
	})
}

func writeFilesystemError(w http.ResponseWriter, err error) {
	w.Header().Set("Cache-Control", "no-store")
	status := http.StatusInternalServerError
	code := "filesystem_internal_error"
	var rootChanged *filesystem.RootChangedError
	switch {
	case errors.As(err, &rootChanged):
		status, code = http.StatusConflict, "filesystem_root_changed"
		writeCodedError(w, status, "filesystem root changed", code, filesystemRootChangedDetails{Root: rootChanged.Root})
		return
	case errors.Is(err, filesystem.ErrUnsupported):
		status, code = http.StatusConflict, "filesystem_unsupported"
	case errors.Is(err, filesystem.ErrInstanceNotFound):
		status, code = http.StatusNotFound, "filesystem_instance_not_found"
	case errors.Is(err, filesystem.ErrNoTransport):
		status, code = http.StatusConflict, "filesystem_no_transport"
	case errors.Is(err, filesystem.ErrTransportUnavailable):
		status, code = http.StatusServiceUnavailable, "filesystem_transport_unavailable"
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
	case errors.Is(err, filesystem.ErrInvalidRange):
		status, code = http.StatusRequestedRangeNotSatisfiable, "filesystem_range_invalid"
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
	if errors.Is(err, filesystem.ErrTransportUnavailable) {
		retryable := true
		writeCodedErrorWithRetry(w, status, "filesystem transport temporarily unavailable", code, nil, &retryable)
		return
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
