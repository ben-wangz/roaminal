package server

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"mime/multipart"
	"net/http"

	"github.com/ben-wangz/roaminal/backend/internal/filesystem"
)

const (
	uploadManifestLimit       = 4 << 20
	uploadMultipartOverhead   = 16 << 20
	filesystemUploadBodyLimit = filesystem.MaxUploadBytes + uploadMultipartOverhead
)

func (s *Server) filesystemCreateUpload(w http.ResponseWriter, r *http.Request, _ string) {
	if s.filesystem == nil {
		writeFilesystemError(w, filesystem.ErrUnsupported)
		return
	}
	if r.ContentLength > filesystemUploadBodyLimit {
		writeFilesystemError(w, filesystem.ErrContentTooLarge)
		return
	}
	mediaType, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	boundary := params["boundary"]
	if err != nil || mediaType != "multipart/form-data" || len(boundary) == 0 || len(boundary) > 70 {
		writeFilesystemError(w, filesystem.ErrUploadManifestInvalid)
		return
	}
	body := &io.LimitedReader{R: http.MaxBytesReader(w, r.Body, filesystemUploadBodyLimit), N: uploadMultipartOverhead + 1}
	source := multipartUpload{parts: multipart.NewReader(body, boundary), body: body, contentLength: r.ContentLength}
	status, err := s.filesystem.CreateUpload(r.Context(), r.PathValue("connectionInstanceId"), source)
	if err != nil {
		writeFilesystemError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, status)
}

type multipartUpload struct {
	parts         *multipart.Reader
	body          *io.LimitedReader
	contentLength int64
}

func (m multipartUpload) ReadManifest() (filesystem.UploadManifest, error) {
	part, err := m.parts.NextPart()
	if err != nil {
		if err == io.EOF {
			err = filesystem.ErrUploadManifestInvalid
		}
		return filesystem.UploadManifest{}, m.readError(err)
	}
	if part.FormName() != "manifest" || part.FileName() != "" {
		return filesystem.UploadManifest{}, filesystem.ErrUploadManifestInvalid
	}
	data, err := io.ReadAll(io.LimitReader(part, uploadManifestLimit+1))
	if err = m.readError(err); err != nil {
		return filesystem.UploadManifest{}, err
	}
	if len(data) > uploadManifestLimit {
		return filesystem.UploadManifest{}, filesystem.ErrContentTooLarge
	}
	return decodeUploadManifest(data)
}

func decodeUploadManifest(data []byte) (filesystem.UploadManifest, error) {
	// Limit the number of decoded entries as well as the JSON size; a small
	// untrusted array of tiny objects can otherwise allocate a large slice.
	wire := struct {
		filesystem.UploadManifest
		Files json.RawMessage `json:"files"`
	}{}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&wire); err != nil || decoder.Decode(new(json.RawMessage)) != io.EOF {
		return filesystem.UploadManifest{}, filesystem.ErrUploadManifestInvalid
	}
	manifest := wire.UploadManifest
	if len(wire.Files) == 0 || bytes.Equal(wire.Files, []byte("null")) {
		return manifest, nil
	}
	decoder = json.NewDecoder(bytes.NewReader(wire.Files))
	decoder.DisallowUnknownFields()
	if token, err := decoder.Token(); err != nil || token != json.Delim('[') {
		return filesystem.UploadManifest{}, filesystem.ErrUploadManifestInvalid
	}
	for decoder.More() {
		if len(manifest.Files) == filesystem.MaxUploadFiles {
			return filesystem.UploadManifest{}, filesystem.ErrContentTooLarge
		}
		var file filesystem.UploadManifestFile
		if err := decoder.Decode(&file); err != nil {
			return filesystem.UploadManifest{}, filesystem.ErrUploadManifestInvalid
		}
		manifest.Files = append(manifest.Files, file)
	}
	return manifest, nil
}

func (m multipartUpload) ReadFiles(total int64, stage func(name string, content io.Reader) error) error {
	if m.contentLength > total+uploadMultipartOverhead {
		return filesystem.ErrContentTooLarge
	}
	// The rest of the wire allowance comes from the validated manifest. No
	// untrusted header, preamble, or epilogue can extend the body limit.
	m.body.N += total
	pending := total
	for {
		part, err := m.parts.NextPart()
		if err == io.EOF {
			// Bytes buffered by the parser have already been counted. Do not
			// allow an epilogue to use the wire allowance for missing files.
			m.body.N -= pending
			_, err := io.Copy(io.Discard, m.body)
			return m.readError(err)
		}
		if err = m.readError(err); err != nil {
			return err
		}
		name := part.FormName()
		if name == "" || name == "manifest" || part.FileName() == "" {
			return filesystem.ErrUploadManifestInvalid
		}
		input := uploadPartReader{part: part, source: m}
		if err := stage(name, &input); err != nil {
			// Do not close an invalid part: multipart.Part.Close drains it.
			return err
		}
		pending -= input.read
	}
}

func (m multipartUpload) readError(err error) error {
	var tooLarge *http.MaxBytesError
	switch {
	case m.body.N <= 0 || errors.As(err, &tooLarge) || errors.Is(err, multipart.ErrMessageTooLarge):
		return filesystem.ErrContentTooLarge
	case err == nil || err == io.EOF:
		return err
	default:
		return filesystem.ErrUploadManifestInvalid
	}
}

type uploadPartReader struct {
	part   *multipart.Part
	source multipartUpload
	read   int64
}

func (r *uploadPartReader) Read(p []byte) (int, error) {
	n, err := r.part.Read(p)
	r.read += int64(n)
	return n, r.source.readError(err)
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
