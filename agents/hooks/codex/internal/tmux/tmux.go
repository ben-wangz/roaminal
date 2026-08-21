package tmux

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"
)

type Info struct {
	SessionName       string `json:"sessionName"`
	SessionID         string `json:"sessionId"`
	SessionCreated    int64  `json:"sessionCreated"`
	PaneID            string `json:"paneId"`
	SocketFingerprint string `json:"socketFingerprint"`
}

type Client struct{ binary string }

func New() (*Client, error) {
	binary, err := exec.LookPath("tmux")
	if err != nil {
		return nil, err
	}
	return &Client{binary: binary}, nil
}

func (c *Client) run(ctx context.Context, args ...string) (string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	command := exec.CommandContext(ctx, c.binary, args...)
	output, err := command.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

func (c *Client) Discover(ctx context.Context) (Info, error) {
	pane := strings.TrimSpace(os.Getenv("TMUX_PANE"))
	if pane != "" {
		if info, err := c.discoverPane(ctx, pane); err == nil {
			return info, nil
		}
	}
	if strings.TrimSpace(os.Getenv("TMUX")) == "" {
		return Info{}, errors.New("tmux environment is not available")
	}
	return c.discoverAncestorPane(ctx)
}

func (c *Client) discoverPane(ctx context.Context, pane string) (Info, error) {
	value, err := c.run(ctx, "display-message", "-p", "-t", pane, "#{session_name}\t#{session_id}\t#{session_created}\t#{pane_id}\t#{socket_path}")
	if err != nil {
		return Info{}, err
	}
	return parseInfo(strings.Split(value, "\t"))
}

func (c *Client) discoverAncestorPane(ctx context.Context) (Info, error) {
	value, err := c.run(ctx, "list-panes", "-a", "-F", "#{session_name}\t#{session_id}\t#{session_created}\t#{pane_id}\t#{socket_path}\t#{pane_pid}")
	if err != nil {
		return Info{}, err
	}
	ancestors := processAncestors(os.Getpid())
	var matches []Info
	for _, line := range strings.Split(value, "\n") {
		parts := strings.Split(line, "\t")
		if len(parts) != 6 {
			continue
		}
		pid, parseErr := strconv.Atoi(parts[5])
		if parseErr != nil || pid < 1 {
			continue
		}
		if _, ok := ancestors[pid]; !ok {
			continue
		}
		info, parseErr := parseInfo(parts[:5])
		if parseErr == nil {
			matches = append(matches, info)
		}
	}
	if len(matches) != 1 {
		return Info{}, errors.New("tmux pane could not be identified")
	}
	return matches[0], nil
}

func parseInfo(parts []string) (Info, error) {
	if len(parts) != 5 || !validIdentity(parts[0], 128) || !validIdentity(parts[1], 128) || !validIdentity(parts[3], 128) || !validIdentity(parts[4], 4096) {
		return Info{}, errors.New("invalid tmux identity")
	}
	created, err := strconv.ParseInt(parts[2], 10, 64)
	if err != nil || created < 0 {
		return Info{}, errors.New("invalid tmux session creation time")
	}
	digest := sha256.Sum256([]byte(parts[4]))
	return Info{SessionName: parts[0], SessionID: parts[1], SessionCreated: created, PaneID: parts[3], SocketFingerprint: hex.EncodeToString(digest[:])[:16]}, nil
}

func validIdentity(value string, maxBytes int) bool {
	if value == "" || len(value) > maxBytes {
		return false
	}
	for _, character := range value {
		if character < 0x20 || character == 0x7f {
			return false
		}
	}
	return true
}

func (c *Client) withLock(ctx context.Context, info Info, fn func() error) error {
	channel := "roaminal-agent-" + fmt.Sprintf("%x", sha256.Sum256([]byte(info.SessionID)))[:16]
	if _, err := c.run(ctx, "wait-for", "-L", channel); err != nil {
		return err
	}
	defer func() { _, _ = c.run(context.Background(), "wait-for", "-U", channel) }()
	return fn()
}

func (c *Client) option(ctx context.Context, info Info, name string) (string, bool) {
	value, err := c.run(ctx, "show-options", "-t", "="+info.SessionName, "-v", name)
	return value, err == nil
}

func (c *Client) setOption(ctx context.Context, info Info, name, value string) error {
	_, err := c.run(ctx, "set-option", "-q", "-t", "="+info.SessionName, name, value)
	return err
}

func (c *Client) Claim(ctx context.Context, info Info, codexSessionID string) (bool, error) {
	if strings.TrimSpace(codexSessionID) == "" {
		return false, errors.New("codex session id is empty")
	}
	var claimed bool
	err := c.withLock(ctx, info, func() error {
		owner, exists := c.option(ctx, info, "@roaminal_agent_owner_v1")
		if !exists || owner == "" {
			claimed = true
			return c.setOption(ctx, info, "@roaminal_agent_owner_v1", codexSessionID+"|"+ownerIdentity())
		}
		claimed = owner == codexSessionID+"|"+ownerIdentity()
		if !claimed {
			if !ownerAlive(owner) {
				claimed = true
				return c.setOption(ctx, info, "@roaminal_agent_owner_v1", codexSessionID+"|"+ownerIdentity())
			}
		}
		return nil
	})
	return claimed, err
}

func (c *Client) NextSequence(ctx context.Context, info Info) (uint64, error) {
	var result uint64
	err := c.withLock(ctx, info, func() error {
		value, ok := c.option(ctx, info, "@roaminal_agent_sequence_v1")
		if ok {
			parsed, err := strconv.ParseUint(value, 10, 64)
			if err != nil || parsed == ^uint64(0) {
				if err == nil {
					err = errors.New("tmux sequence overflow")
				}
				return err
			}
			result = parsed + 1
		} else {
			result = 1
		}
		return c.setOption(ctx, info, "@roaminal_agent_sequence_v1", strconv.FormatUint(result, 10))
	})
	return result, err
}

func (c *Client) Release(ctx context.Context, info Info, codexSessionID string) error {
	if codexSessionID == "" {
		return nil
	}
	return c.withLock(ctx, info, func() error {
		owner, exists := c.option(ctx, info, "@roaminal_agent_owner_v1")
		if !exists || !strings.HasPrefix(owner, codexSessionID+"|") {
			return nil
		}
		_, err := c.run(ctx, "set-option", "-q", "-u", "-t", "="+info.SessionName, "@roaminal_agent_owner_v1")
		return err
	})
}

func (c *Client) Available(ctx context.Context) bool {
	_, err := c.run(ctx, "-V")
	return err == nil
}

func (c *Client) Deadline() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 2*time.Second)
}

func processAlive(pid int) bool {
	if pid < 1 {
		return false
	}
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	err = process.Signal(syscall.Signal(0))
	return err == nil || errors.Is(err, syscall.EPERM)
}
