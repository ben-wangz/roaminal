package report

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ben-wangz/roaminal/agents/hooks/codex/internal/model"
	"github.com/ben-wangz/roaminal/agents/hooks/codex/internal/tmux"
)

func SpoolPath(home string, info tmux.Info) string {
	hash := sha256.Sum256([]byte(info.SessionID + "|" + fmt.Sprint(info.SessionCreated) + "|" + info.SocketFingerprint))
	return filepath.Join(home, ".roaminal", "spool", base64.RawURLEncoding.EncodeToString(hash[:])[:24])
}

func WriteSpool(home string, event model.Event, info tmux.Info) error {
	dir := SpoolPath(home, info)
	root := filepath.Join(home, ".roaminal")
	for _, path := range []string{root, filepath.Join(root, "spool"), dir} {
		if err := ensurePrivateDir(path); err != nil {
			return err
		}
	}
	data, err := json.Marshal(event)
	if err != nil {
		return err
	}
	path := filepath.Join(dir, fmt.Sprintf("%020d-%s.json", event.Sequence, event.EventID))
	tmp, err := os.CreateTemp(dir, ".event-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(0600); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		return err
	}
	if err := syncDirectory(dir); err != nil {
		return err
	}
	return trimSpool(dir)
}

type HTTPError struct {
	Status  int
	Message string
}

func (e *HTTPError) Error() string { return e.Message }

func Send(ctx context.Context, cfg model.Config, event model.Event) (bool, error) {
	data, err := json.Marshal(event)
	if err != nil {
		return false, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, cfg.WebhookURL, bytes.NewReader(data))
	if err != nil {
		return false, err
	}
	request.Header.Set("Authorization", "Bearer "+cfg.Token)
	request.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: time.Second}
	response, err := client.Do(request)
	if err != nil {
		return false, err
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
	if response.StatusCode >= 200 && response.StatusCode < 300 {
		return true, nil
	}
	return false, &HTTPError{Status: response.StatusCode, Message: response.Status}
}

func Drain(ctx context.Context, cfg model.Config, home string, info tmux.Info) {
	dir := SpoolPath(home, info)
	release, locked := AcquireSenderLock(home, info)
	if !locked {
		return
	}
	defer release()
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	sort.Slice(entries, func(left, right int) bool { return entries[left].Name() < entries[right].Name() })
	sent := 0
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") || sent >= 16 {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var event model.Event
		if json.Unmarshal(data, &event) != nil {
			_ = os.Remove(path)
			continue
		}
		accepted, err := Send(ctx, cfg, event)
		if err != nil && retryableHTTPError(err) {
			if waitErr := waitRetry(ctx); waitErr == nil {
				accepted, err = Send(ctx, cfg, event)
			}
		}
		if err != nil {
			var responseErr *HTTPError
			if errors.As(err, &responseErr) && (responseErr.Status == http.StatusBadRequest || responseErr.Status == http.StatusRequestEntityTooLarge) {
				rejectSpool(path, responseErr.Status)
			}
			return
		}
		if accepted {
			_ = os.Remove(path)
			sent++
		}
	}
}

func rejectSpool(path string, status int) {
	code := "agent_event_invalid"
	if status == http.StatusRequestEntityTooLarge {
		code = "agent_event_too_large"
	}
	data, _ := json.Marshal(map[string]string{"code": code})
	_ = os.WriteFile(path+".rejected", append(data, '\n'), 0600)
	_ = os.Remove(path)
}

func retryableHTTPError(err error) bool {
	var responseErr *HTTPError
	if errors.As(err, &responseErr) {
		return responseErr.Status == http.StatusTooManyRequests || responseErr.Status >= 500
	}
	return true
}

func AcquireSenderLock(home string, info tmux.Info) (func(), bool) {
	digest := sha256.Sum256([]byte(info.SessionID + "|" + fmt.Sprint(info.SessionCreated) + "|" + info.SocketFingerprint))
	path := filepath.Join(home, ".roaminal", "locks", "send-"+hex.EncodeToString(digest[:8])+".lock")
	root := filepath.Join(home, ".roaminal")
	if err := ensurePrivateDir(root); err != nil {
		return func() {}, false
	}
	if err := ensurePrivateDir(filepath.Join(root, "locks")); err != nil {
		return func() {}, false
	}
	if err := os.Mkdir(path, 0700); err != nil {
		if !os.IsExist(err) || !staleSenderLock(path) {
			return func() {}, false
		}
		_ = os.Remove(filepath.Join(path, "owner"))
		_ = os.Remove(path)
		if err := os.Mkdir(path, 0700); err != nil {
			return func() {}, false
		}
	}
	ownerValue := senderLockOwner{PID: os.Getpid(), ProcessStart: tmux.ProcessStart(os.Getpid()), CreatedAt: time.Now().UTC().Format(time.RFC3339Nano)}
	owner := filepath.Join(path, "owner")
	data, err := json.Marshal(ownerValue)
	if err != nil {
		_ = os.Remove(path)
		return func() {}, false
	}
	if err := os.WriteFile(owner, data, 0600); err != nil {
		_ = os.Remove(path)
		return func() {}, false
	}
	return func() {
		data, readErr := os.ReadFile(owner)
		var current senderLockOwner
		if readErr != nil || json.Unmarshal(data, &current) != nil || current.PID != ownerValue.PID || current.ProcessStart != ownerValue.ProcessStart {
			return
		}
		_ = os.Remove(owner)
		_ = os.Remove(path)
	}, true
}

func ensurePrivateDir(path string) error {
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return os.Mkdir(path, 0700)
	}
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode().Perm()&0o077 != 0 {
		return errors.New("private spool directory permissions are unsafe")
	}
	return nil
}

type senderLockOwner struct {
	PID          int    `json:"pid"`
	ProcessStart string `json:"processStart"`
	CreatedAt    string `json:"createdAt"`
}

func staleSenderLock(path string) bool {
	info, err := os.Lstat(path)
	if err != nil || !info.IsDir() || info.Mode().Perm()&0o077 != 0 || time.Since(info.ModTime()) < 2*time.Minute {
		return false
	}
	data, err := os.ReadFile(filepath.Join(path, "owner"))
	if err != nil {
		return true
	}
	var owner senderLockOwner
	if json.Unmarshal(data, &owner) != nil {
		pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
		if err != nil {
			return true
		}
		owner.PID = pid
	}
	if owner.PID < 1 {
		return true
	}
	if !tmux.ProcessAlive(owner.PID) {
		return true
	}
	return owner.ProcessStart != "" && tmux.ProcessStart(owner.PID) != "" && owner.ProcessStart != tmux.ProcessStart(owner.PID)
}

func waitRetry(ctx context.Context) error {
	timer := time.NewTimer(250 * time.Millisecond)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
