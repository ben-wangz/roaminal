package domain

import "time"

// UploadJobRecord is the durable, non-secret portion of a FileSystem upload
// operation. Staged file contents remain in the staging adapter; this record
// only keeps enough metadata to reconcile status after a process restart.
type UploadJobRecord struct {
	ID                   string                `json:"id"`
	ConnectionInstanceID string                `json:"connectionInstanceId"`
	RootRevision         string                `json:"rootRevision"`
	TargetPath           string                `json:"targetPath"`
	ConflictPolicy       string                `json:"conflictPolicy"`
	Status               string                `json:"status"`
	Transport            string                `json:"transport"`
	BytesSent            int64                 `json:"bytesSent"`
	BytesTotal           int64                 `json:"bytesTotal"`
	CurrentPath          string                `json:"currentPath"`
	Failures             []UploadFailureRecord `json:"failures"`
	StagingPath          string                `json:"stagingPath,omitempty"`
	Files                []UploadFileRecord    `json:"files"`
	CreatedAt            time.Time             `json:"createdAt"`
	UpdatedAt            time.Time             `json:"updatedAt"`
}

// InterruptUpload marks persisted in-flight metadata as terminal without
// replaying files or inventing a completion timestamp.
func InterruptUpload(record *UploadJobRecord) bool {
	if record.Status != "queued" && record.Status != "running" {
		return false
	}
	record.Status = "failed"
	record.Failures = append(record.Failures, UploadFailureRecord{Code: "filesystem_upload_failed", Error: "Upload interrupted by restart"})
	return true
}

type UploadFailureRecord struct {
	Path  string `json:"path"`
	Code  string `json:"code"`
	Error string `json:"error,omitempty"`
}

type UploadFileRecord struct {
	Part         string    `json:"part"`
	RelativePath string    `json:"relativePath"`
	Size         int64     `json:"size"`
	ModifiedAt   time.Time `json:"modifiedAt"`
	StagedPath   string    `json:"stagedPath"`
}
