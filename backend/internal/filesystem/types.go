package filesystem

import "time"

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
	Name         string     `json:"name"`
	RelativePath string     `json:"relativePath"`
	AbsolutePath string     `json:"absolutePath"`
	Type         string     `json:"type"`
	Size         *int64     `json:"size"`
	ModifiedAt   *time.Time `json:"modifiedAt"`
	Mode         uint32     `json:"mode"`
	Symlink      bool       `json:"symlink"`
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
