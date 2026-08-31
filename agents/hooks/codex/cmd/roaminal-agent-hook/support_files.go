package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

func processStart(pid int) string {
	if runtime.GOOS == "linux" {
		data, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "stat"))
		if err != nil {
			return ""
		}
		closeName := strings.LastIndex(string(data), ")")
		if closeName < 0 {
			return ""
		}
		fields := strings.Fields(string(data)[closeName+1:])
		if len(fields) < 20 {
			return ""
		}
		return fields[19]
	}
	output, err := exec.Command("ps", "-o", "lstart=", "-p", strconv.Itoa(pid)).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(output))
}

func readPrivateFile(path string, limit int64) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm() != 0600 {
		return nil, errors.New("private file permissions are unsafe")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, errors.New("private file is too large")
	}
	return data, nil
}

func syncDirectory(path string) error {
	directory, err := os.Open(path)
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Sync()
}

func componentVersionGreater(current, requested string) bool {
	currentNumber, currentErr := strconv.Atoi(current)
	requestedNumber, requestedErr := strconv.Atoi(requested)
	if currentErr == nil && requestedErr == nil {
		return currentNumber > requestedNumber
	}
	return current > requested
}

func validChecksum(value string) bool {
	if len(value) != sha256.Size*2 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func executableChecksum(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func fileExists(path string) bool {
	info, err := os.Lstat(path)
	return err == nil && info.Mode().IsRegular() && info.Mode().Perm()&0o077 == 0 && info.Mode().Perm()&0400 != 0
}

func output(value any) error { return json.NewEncoder(os.Stdout).Encode(value) }

func safeError(err error) string {
	message := strings.TrimSpace(err.Error())
	for _, marker := range []string{"token", "Bearer", "credential"} {
		if strings.Contains(strings.ToLower(message), strings.ToLower(marker)) {
			return "agent operation failed"
		}
	}
	return message
}
