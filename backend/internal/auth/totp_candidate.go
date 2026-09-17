package auth

import (
	"bytes"
	"crypto/hmac"
	"encoding/base32"
	"encoding/base64"
	"fmt"
	"image"
	"image/png"
	"net/url"
	"strings"
	"time"

	"github.com/pquerna/otp"
	"github.com/pquerna/otp/totp"
)

var totpSecretEncoding = base32.StdEncoding.WithPadding(base32.NoPadding)

var totpValidateOpts = totp.ValidateOpts{Period: TOTPPeriod, Skew: 0, Digits: otp.DigitsSix, Algorithm: otp.AlgorithmSHA1}

// newCandidateLocked generates the in-memory enrollment candidate. The
// secret comes from the injected random source; the QR image is rendered
// locally by the OTP library.
func (m *Manager) newCandidateLocked() (setupCandidate, error) {
	raw := make([]byte, TOTPSecretSize)
	if _, err := m.random.Read(raw); err != nil {
		return setupCandidate{}, err
	}
	secret := totpSecretEncoding.EncodeToString(raw)
	uri := provisioningURI(secret)
	key, err := otp.NewKeyFromURL(uri)
	if err != nil {
		return setupCandidate{}, fmt.Errorf("build TOTP provisioning key: %w", err)
	}
	qr, err := key.Image(256, 256)
	if err != nil {
		return setupCandidate{}, fmt.Errorf("render TOTP QR image: %w", err)
	}
	pngData, err := encodeQRPNG(qr)
	if err != nil {
		return setupCandidate{}, err
	}
	return setupCandidate{secret: secret, uri: uri, qrPNG: "data:image/png;base64," + base64.StdEncoding.EncodeToString(pngData)}, nil
}

func provisioningURI(secret string) string {
	values := url.Values{}
	values.Set("algorithm", "SHA1")
	values.Set("digits", "6")
	values.Set("issuer", TOTPIssuer)
	values.Set("period", "30")
	values.Set("secret", secret)
	return "otpauth://totp/" + TOTPIssuer + ":" + TOTPAccount + "?" + values.Encode()
}

func encodeQRPNG(qr image.Image) ([]byte, error) {
	var buffer bytes.Buffer
	if err := png.Encode(&buffer, qr); err != nil {
		return nil, fmt.Errorf("encode TOTP QR image: %w", err)
	}
	return buffer.Bytes(), nil
}

// validateTOTPCode checks a six-digit code against a Base32 secret with at
// most one adjacent time step of tolerance in each direction and returns the
// matched step for replay accounting. All OTP math is delegated to the
// maintained TOTP library.
func validateTOTPCode(secret, code string, now time.Time) (int64, bool) {
	code = strings.TrimSpace(code)
	if len(code) != TOTPDigits {
		return 0, false
	}
	for _, r := range code {
		if r < '0' || r > '9' {
			return 0, false
		}
	}
	step := now.UTC().Unix() / TOTPPeriod
	for _, delta := range [3]int64{0, 1, -1} {
		candidate, err := totp.GenerateCodeCustom(secret, time.Unix((step+delta)*TOTPPeriod, 0).UTC(), totpValidateOpts)
		if err != nil {
			return 0, false
		}
		if hmac.Equal([]byte(candidate), []byte(code)) {
			return step + delta, true
		}
	}
	return 0, false
}
