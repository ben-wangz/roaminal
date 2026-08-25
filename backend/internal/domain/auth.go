package domain

import "time"

// AuthSessionRecord contains only authentication data. Workspace layout is a
// separate aggregate owned by the workspace repository.
type AuthSessionRecord struct {
	ID                  string    `json:"id"`
	PasswordFingerprint string    `json:"passwordFingerprint"`
	RefreshTokenHash    string    `json:"refreshTokenHash"`
	CreatedAt           time.Time `json:"createdAt"`
	LastSeenAt          time.Time `json:"lastSeenAt"`
	RefreshExpiresAt    time.Time `json:"refreshExpiresAt"`
	RotatedAt           time.Time `json:"rotatedAt"`
	UserAgent           string    `json:"userAgent"`
}
