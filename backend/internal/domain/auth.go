package domain

import "time"

// AuthSessionRecord contains only authentication data. Workspace layout is a
// separate aggregate owned by the workspace repository. EnrollmentID binds the
// session to a TOTP enrollment; records with an empty or different binding are
// legacy or stale credentials and must be rejected.
type AuthSessionRecord struct {
	ID                  string    `json:"id"`
	PasswordFingerprint string    `json:"passwordFingerprint"`
	EnrollmentID        string    `json:"enrollmentId"`
	RefreshTokenHash    string    `json:"refreshTokenHash"`
	CreatedAt           time.Time `json:"createdAt"`
	LastSeenAt          time.Time `json:"lastSeenAt"`
	RefreshExpiresAt    time.Time `json:"refreshExpiresAt"`
	RotatedAt           time.Time `json:"rotatedAt"`
	UserAgent           string    `json:"userAgent"`
}
