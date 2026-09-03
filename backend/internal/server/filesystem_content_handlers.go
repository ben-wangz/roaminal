package server

import (
	"context"
	"errors"
	"io"
	"net/http"

	"github.com/ben-wangz/roaminal/backend/internal/filesystem"
	"github.com/ben-wangz/roaminal/backend/internal/imagepreview"
)

func (s *Server) filesystemContent(w http.ResponseWriter, r *http.Request, _ string) {
	variant := r.URL.Query().Get("variant")
	if variant != "" && variant != "preview" && variant != "original" {
		writeCodedError(w, http.StatusBadRequest, "invalid filesystem content variant", "filesystem_variant_invalid", nil, "variant")
		return
	}
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
	if entry.Type != "file" || entry.Size == nil || entry.Symlink {
		writeFilesystemError(w, filesystem.ErrContentUnavailable)
		return
	}
	contentType := mimeTypeForEntry(entry.Name, entry.Type, entry.MIMEType)
	download := r.URL.Query().Get("download") == "1"
	if variant == "preview" && !download {
		s.filesystemImagePreview(w, r, id, pathValue, entry, root, contentType)
		return
	}

	start, length, partial, rangeErr := contentRange(r.Header.Get("Range"), *entry.Size)
	if rangeErr != nil {
		writeFilesystemError(w, rangeErr)
		return
	}
	if r.Header.Get("Range") == "" {
		if variant == "original" || download {
			length = *entry.Size
		} else {
			length = contentWindowLength(contentType, *entry.Size)
		}
		partial = length < *entry.Size
	}
	etag := `"` + consistencyToken(entry) + `"`
	setContentHeaders(w, contentType, length, etag, variant == "original" && !download, entry.ModifiedAt)
	if variant != "" && ifNoneMatch(r.Header.Get("If-None-Match"), etag) {
		w.WriteHeader(http.StatusNotModified)
		return
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
	if partial {
		w.Header().Set("X-Roaminal-Content-Truncated", "true")
	}
	if download {
		setAttachment(w, entry.Name)
	}
	writeContent(w, r, stream.Reader, stream.Start, stream.ContentLength, stream.TotalSize)
}

func (s *Server) filesystemImagePreview(w http.ResponseWriter, r *http.Request, id, pathValue string, entry filesystem.Entry, root filesystem.RootContext, contentType string) {
	if s.imagePreview == nil {
		writeImagePreviewError(w, imagepreview.ErrUnavailable)
		return
	}
	sourceToken := consistencyToken(entry)
	request := imagepreview.Request{
		ConnectionInstanceID: id,
		RootAbsolutePath:     root.AbsolutePath,
		RootRevision:         root.Revision,
		RelativePath:         entry.RelativePath,
		MIMEType:             contentType,
		SourceSize:           *entry.Size,
		SourceToken:          sourceToken,
		Open: func(ctx context.Context) (io.ReadCloser, error) {
			stream, err := s.filesystem.OpenContent(ctx, id, pathValue, root.Revision, 0, *entry.Size)
			if err != nil {
				return nil, err
			}
			if consistencyToken(stream.Entry) != sourceToken {
				_ = stream.Reader.Close()
				return nil, filesystem.ErrContentUnavailable
			}
			return stream.Reader, nil
		},
		Validate: func(ctx context.Context) error {
			next, nextRoot, err := s.filesystem.Stat(ctx, id, pathValue, root.Revision)
			if err != nil {
				return err
			}
			if nextRoot.Revision != root.Revision || consistencyToken(next) != sourceToken {
				return filesystem.ErrContentUnavailable
			}
			return nil
		},
	}
	result, err := s.imagePreview.Open(r.Context(), request)
	if err != nil {
		writeImagePreviewError(w, err)
		return
	}
	defer result.Reader.Close()
	start, length, _, rangeErr := contentRange(r.Header.Get("Range"), result.Size)
	if rangeErr != nil {
		writeFilesystemError(w, rangeErr)
		return
	}
	if _, err := result.Reader.Seek(start, io.SeekStart); err != nil {
		writeImagePreviewError(w, err)
		return
	}
	setContentHeaders(w, imagepreview.OutputContentType, length, result.ETag, false, entry.ModifiedAt)
	w.Header().Set("X-Roaminal-Image-Variant", "preview")
	if ifNoneMatch(r.Header.Get("If-None-Match"), result.ETag) {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	writeContent(w, r, result.Reader, start, length, result.Size)
}

func writeImagePreviewError(w http.ResponseWriter, err error) {
	var rootChanged *filesystem.RootChangedError
	if errors.As(err, &rootChanged) || errors.Is(err, filesystem.ErrContentUnavailable) || errors.Is(err, filesystem.ErrNoTransport) || errors.Is(err, filesystem.ErrTransportUnavailable) {
		writeFilesystemError(w, err)
		return
	}
	switch {
	case errors.Is(err, imagepreview.ErrUnsupported):
		writeCodedError(w, http.StatusUnsupportedMediaType, "filesystem image preview is unsupported", "filesystem_image_preview_unsupported", nil)
	case errors.Is(err, imagepreview.ErrInvalid):
		writeCodedError(w, http.StatusUnprocessableEntity, "filesystem image preview is unavailable", "filesystem_image_preview_unavailable", nil)
	case errors.Is(err, imagepreview.ErrUnavailable), errors.Is(err, context.DeadlineExceeded):
		retryable := true
		writeCodedErrorWithRetry(w, http.StatusServiceUnavailable, "filesystem image preview is temporarily unavailable", "filesystem_image_preview_unavailable", nil, &retryable)
	default:
		writeCodedErrorWithRetry(w, http.StatusServiceUnavailable, "filesystem image preview is temporarily unavailable", "filesystem_image_preview_unavailable", nil, nil)
	}
}
