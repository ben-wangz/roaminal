package filesystem

import (
	"io"
	"time"
)

type RootContext struct {
	ConnectionInstanceID string    `json:"connectionInstanceId"`
	AbsolutePath         string    `json:"absolutePath"`
	RelativePath         string    `json:"relativePath"`
	Source               string    `json:"source"`
	Status               string    `json:"status"`
	Revision             string    `json:"revision"`
	ResolvedAt           time.Time `json:"resolvedAt"`
}

type Entry struct {
	Name         string `json:"name"`
	RelativePath string `json:"relativePath"`
	AbsolutePath string `json:"absolutePath"`
	Type         string `json:"type"`
	// MIMEType is adapter metadata used by the server to select a viewer. Keep
	// it out of directory JSON; stat already exposes the value as mimeType.
	MIMEType   string     `json:"-"`
	Size       *int64     `json:"size"`
	ModifiedAt *time.Time `json:"modifiedAt"`
	Mode       uint32     `json:"mode"`
	Symlink    bool       `json:"symlink"`
}

type DirectoryResult struct {
	ConnectionInstanceID string  `json:"connectionInstanceId"`
	RootRevision         string  `json:"rootRevision"`
	Path                 string  `json:"path"`
	Entries              []Entry `json:"entries"`
	NextCursor           *string `json:"nextCursor"`
}

type DirectorySnapshot struct {
	ID        string
	Key       string
	Entries   []Entry
	ExpiresAt time.Time
}

type ContentStream struct {
	Reader        io.ReadCloser
	Entry         Entry
	Root          RootContext
	Start         int64
	End           int64
	TotalSize     int64
	ContentLength int64
}

type UploadManifest struct {
	RootRevision   string               `json:"rootRevision"`
	TargetPath     string               `json:"targetPath"`
	ConflictPolicy string               `json:"conflictPolicy"`
	Files          []UploadManifestFile `json:"files"`
}

type UploadManifestFile struct {
	Part         string    `json:"part"`
	RelativePath string    `json:"relativePath"`
	Size         int64     `json:"size"`
	ModifiedAt   time.Time `json:"modifiedAt"`
}

type UploadFailure struct {
	Path  string `json:"path"`
	Code  string `json:"code"`
	Error string `json:"error,omitempty"`
}

type UploadStatus struct {
	UploadID    string          `json:"uploadId"`
	Status      string          `json:"status"`
	Transport   string          `json:"transport"`
	TargetPath  string          `json:"targetPath"`
	BytesSent   int64           `json:"bytesSent"`
	BytesTotal  int64           `json:"bytesTotal"`
	CurrentPath string          `json:"currentPath"`
	Failures    []UploadFailure `json:"failures"`
}
