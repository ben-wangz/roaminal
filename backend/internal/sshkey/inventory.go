package sshkey

import (
	"bufio"
	"encoding/base64"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"github.com/ben-wangz/roaminal/backend/internal/sshfs"
)

type Key struct {
	KeyID              string `json:"keyId"`
	FileName           string `json:"fileName"`
	Algorithm          string `json:"algorithm"`
	Bits               int    `json:"bits"`
	Fingerprint        string `json:"fingerprint"`
	PublicKeyAvailable bool   `json:"publicKeyAvailable"`
	ReadOnly           bool   `json:"readOnly"`
	Status             string `json:"status"`
}

type Inventory struct {
	Root       *sshfs.Root
	KeygenPath string
}

func New(root *sshfs.Root) *Inventory {
	return &Inventory{Root: root, KeygenPath: discover("ssh-keygen")}
}

func (i *Inventory) List() []Key {
	if i == nil || i.Root == nil || !i.Root.Available() {
		return []Key{}
	}
	entries, err := i.Root.ReadDir()
	if err != nil {
		return []Key{}
	}
	result := make([]Key, 0)
	for _, entry := range entries {
		name := entry.Name()
		algorithm, ok := allowedName(name)
		if !ok || entry.IsDir() {
			continue
		}
		linkInfo, err := i.Root.Lstat(name)
		if err != nil || linkInfo.IsDir() {
			continue
		}
		info := linkInfo
		readOnly := linkInfo.Mode()&os.ModeSymlink != 0
		if readOnly {
			info, err = i.Root.Stat(name)
		}
		if err != nil || !info.Mode().IsRegular() || info.Size() > sshfs.PrivateKeyMaxBytes {
			continue
		}
		key := Key{KeyID: KeyID(name), FileName: name, Algorithm: algorithm, ReadOnly: readOnly || !writableByRuntime(info), Status: "available"}
		key.Bits, key.Fingerprint, err = i.fingerprint(name)
		if err != nil {
			key.Status = "invalid"
		}
		if pub, pubErr := i.Root.Lstat(name + ".pub"); pubErr == nil && !pub.IsDir() {
			if pub.Mode()&os.ModeSymlink != 0 {
				pub, pubErr = i.Root.Stat(name + ".pub")
			}
			if pubErr == nil && pub.Mode().IsRegular() && pub.Size() <= sshfs.PublicKeyMaxBytes {
				key.PublicKeyAvailable = i.validPublic(name + ".pub")
			}
		}
		result = append(result, key)
	}
	return result
}

func (i *Inventory) Public(keyID string) (string, error) {
	name, err := DecodeKeyID(keyID)
	if err != nil {
		return "", err
	}
	if _, ok := allowedName(name); !ok {
		return "", errors.New("unsupported key filename")
	}
	data, _, err := i.Root.ReadFile(name+".pub", sshfs.PublicKeyMaxBytes)
	if err != nil {
		return "", err
	}
	if !validPublic(data) {
		return "", errors.New("invalid public key")
	}
	return strings.TrimSpace(string(data)), nil
}

func (i *Inventory) fingerprint(name string) (int, string, error) {
	if i.KeygenPath == "" {
		return 0, "", errors.New("ssh-keygen unavailable")
	}
	file, err := i.Root.Open(name)
	if err != nil {
		return 0, "", err
	}
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return 0, "", err
	}
	if !info.Mode().IsRegular() || info.Size() > sshfs.PrivateKeyMaxBytes {
		_ = file.Close()
		return 0, "", errors.New("private key is not a regular file")
	}
	cmd := exec.Command(i.KeygenPath, "-lf", "/dev/stdin", "-E", "sha256")
	cmd.Stdin = file
	cmd.Env = append(os.Environ(), "LC_ALL=C")
	out, runErr := cmd.Output()
	_ = file.Close()
	if runErr != nil {
		return 0, "", runErr
	}
	fields := strings.Fields(string(out))
	if len(fields) < 2 {
		return 0, "", errors.New("invalid ssh-keygen output")
	}
	bits, err := strconv.Atoi(fields[0])
	if err != nil {
		return 0, "", err
	}
	fingerprint := fields[1]
	if !strings.HasPrefix(fingerprint, "SHA256:") {
		return 0, "", errors.New("invalid fingerprint")
	}
	return bits, fingerprint, nil
}

func (i *Inventory) validPublic(name string) bool {
	data, _, err := i.Root.ReadFile(name, sshfs.PublicKeyMaxBytes)
	return err == nil && validPublic(data)
}
func validPublic(data []byte) bool {
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	if !scanner.Scan() {
		return false
	}
	fields := strings.Fields(scanner.Text())
	return len(fields) >= 2 && (fields[0] == "ssh-ed25519" || fields[0] == "ssh-rsa")
}
func allowedName(name string) (string, bool) {
	if name == "id_ed25519" || strings.HasSuffix(name, "_ed25519") {
		return "ed25519", true
	}
	if name == "id_rsa" || strings.HasSuffix(name, "_rsa") {
		return "rsa", true
	}
	return "", false
}
func AlgorithmForName(name string) (string, bool) { return allowedName(name) }
func writableByRuntime(info os.FileInfo) bool {
	if info == nil {
		return false
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	return ok && int(stat.Uid) == os.Getuid() && info.Mode().Perm()&0o077 == 0
}
func KeyID(name string) string { return base64.RawURLEncoding.EncodeToString([]byte(name)) }
func DecodeKeyID(id string) (string, error) {
	data, err := base64.RawURLEncoding.DecodeString(id)
	if err != nil || len(data) == 0 || strings.ContainsAny(string(data), "/\\") {
		return "", errors.New("invalid key id")
	}
	return string(data), nil
}
func discover(name string) string {
	path, err := exec.LookPath(name)
	if err != nil {
		return ""
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return path
	}
	return absolute
}
