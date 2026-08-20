package server

import (
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"strings"

	"github.com/ben-wangz/roaminal/backend/internal/filesystem"
)

const filesystemUploadBodyLimit = int64(10<<30) + 16<<20

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
