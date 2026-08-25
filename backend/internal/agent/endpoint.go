package agent

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"strings"
	"unicode"

	"github.com/ben-wangz/roaminal/backend/internal/ports"
)

func NormalizeEndpoint(value ports.EffectiveEndpoint) (Endpoint, error) {
	user := strings.TrimSpace(value.User)
	host := strings.TrimSpace(value.Host)
	if user == "" || host == "" || value.Port < 1 || value.Port > 65535 {
		return Endpoint{}, errors.New("incomplete endpoint")
	}
	for _, runeValue := range user + host {
		if unicode.IsControl(runeValue) || unicode.IsSpace(runeValue) {
			return Endpoint{}, errors.New("endpoint contains control character")
		}
	}
	if strings.HasPrefix(host, "[") || strings.HasSuffix(host, "]") {
		if !strings.HasPrefix(host, "[") || !strings.HasSuffix(host, "]") {
			return Endpoint{}, errors.New("endpoint host brackets are invalid")
		}
		host = host[1 : len(host)-1]
	}
	if ip := net.ParseIP(host); ip != nil {
		host = ip.String()
	} else {
		host = strings.ToLower(strings.TrimSuffix(host, "."))
	}
	if host == "" {
		return Endpoint{}, errors.New("endpoint host is empty")
	}
	key := endpointKey(user, host, value.Port)
	return Endpoint{Key: key, User: user, Host: host, Port: value.Port, Display: endpointDisplay(user, host, value.Port)}, nil
}

func endpointDisplay(user, host string, port int) string {
	if strings.Contains(host, ":") {
		host = "[" + host + "]"
	}
	return fmt.Sprintf("%s@%s:%d", user, host, port)
}

func endpointKey(user, host string, port int) string {
	digest := sha256.New()
	write := func(value string) {
		var length [8]byte
		binary.BigEndian.PutUint64(length[:], uint64(len(value)))
		_, _ = digest.Write(length[:])
		_, _ = digest.Write([]byte(value))
	}
	write("roaminal-ssh-endpoint-v1")
	write(user)
	write(host)
	write(fmt.Sprintf("%d", port))
	return base64.RawURLEncoding.EncodeToString(digest.Sum(nil))
}
