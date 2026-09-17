package domain

// TOTPFormatVersion is owned by the mandatory-TOTP enrollment record stored in
// the private `2fa-secret` state file. It is independent of the auth-session
// storage schema.
const TOTPFormatVersion = 1

// TOTPEnrollmentRecord is the single shared identity's TOTP enrollment. The
// secret is Base32 credential material: it must never be logged, serialized
// through generic APIs, or exposed to the frontend outside the setup flow.
type TOTPEnrollmentRecord struct {
	FormatVersion        int    `json:"formatVersion"`
	EnrollmentID         string `json:"enrollmentId"`
	Secret               string `json:"secret"`
	LastAcceptedTimeStep int64  `json:"lastAcceptedTimeStep"`
}
