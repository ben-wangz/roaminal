package filesystem

import (
	"context"
	"fmt"
	"io"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/ben-wangz/roaminal/backend/internal/ports"
)

// remoteFileSystemAdapter is the only FileSystem component that knows the
// remote shell protocol. The service above it receives structured adapter
// results and owns policy and consistency decisions.
type remoteFileSystemAdapter struct {
	executor ports.RemoteExecutor
}

func newRemoteFileSystemAdapter(executor ports.RemoteExecutor) ports.RemoteFileSystem {
	if executor == nil {
		return nil
	}
	return &remoteFileSystemAdapter{executor: executor}
}

func (a *remoteFileSystemAdapter) ConnectionInstance(id string) (ports.ConnectionInstanceView, error) {
	return a.executor.ConnectionInstance(id)
}

func (a *remoteFileSystemAdapter) ProbeTmuxRoot(ctx context.Context, id, session string) (ports.RemoteRoot, error) {
	result, err := a.executor.RunRemote(ctx, id, ports.RemoteCommand{Script: tmuxRootScript, Args: []string{session}, OutputLimit: 16 << 10, Timeout: rootProbeTimeout})
	if err != nil {
		return ports.RemoteRoot{}, err
	}
	absolute, err := parseRootOutput(result.Output)
	if err != nil {
		return ports.RemoteRoot{}, err
	}
	return ports.RemoteRoot{AbsolutePath: absolute, Source: "tmux", Status: "current"}, nil
}

func (a *remoteFileSystemAdapter) ProbeConfiguredRoot(ctx context.Context, id, pwd string) (ports.RemoteRoot, error) {
	result, err := a.executor.RunRemote(ctx, id, ports.RemoteCommand{Script: configuredRootScript, Args: []string{pwd}, OutputLimit: 16 << 10, Timeout: rootProbeTimeout})
	if err != nil {
		return ports.RemoteRoot{}, err
	}
	absolute, err := parseRootOutput(result.Output)
	if err != nil {
		return ports.RemoteRoot{}, err
	}
	return ports.RemoteRoot{AbsolutePath: absolute, Source: "configured", Status: "fallback"}, nil
}

func (a *remoteFileSystemAdapter) List(ctx context.Context, id, root, relative string) ([]ports.RemoteFileEntry, error) {
	result, err := a.executor.RunRemote(ctx, id, ports.RemoteCommand{Script: directoryScript, Args: []string{root, relative}, OutputLimit: maxDirectoryOutput, Timeout: rootRequestTimeout})
	if err != nil {
		return nil, err
	}
	raw, err := parseDirectory(result.Output)
	if err != nil {
		return nil, err
	}
	return convertEntries(raw), nil
}

func (a *remoteFileSystemAdapter) Stat(ctx context.Context, id, root, relative string) (ports.RemoteFileEntry, error) {
	result, err := a.executor.RunRemote(ctx, id, ports.RemoteCommand{Script: statScript, Args: []string{root, relative}, OutputLimit: 32 << 10, Timeout: rootRequestTimeout})
	if err != nil {
		return ports.RemoteFileEntry{}, err
	}
	raw, err := parseDirectory(result.Output)
	if err != nil {
		return ports.RemoteFileEntry{}, err
	}
	if len(raw) != 1 {
		return ports.RemoteFileEntry{}, ErrProtocol
	}
	return convertEntry(raw[0]), nil
}

func (a *remoteFileSystemAdapter) OpenContent(ctx context.Context, id, root, relative string, start, length int64) (io.ReadCloser, error) {
	return a.executor.OpenRemote(ctx, id, ports.RemoteCommand{Script: contentScript, Args: []string{root, relative, strconv.FormatInt(start, 10), strconv.FormatInt(length, 10)}, Timeout: 15 * time.Minute})
}

func (a *remoteFileSystemAdapter) AcquireRemoteTransfer(ctx context.Context, id string) (ports.RemoteTransferLease, error) {
	return a.executor.AcquireRemoteTransfer(ctx, id)
}

func (a *remoteFileSystemAdapter) ResolveUploadTarget(ctx context.Context, id, root, relative string) (string, error) {
	result, err := a.executor.RunRemote(ctx, id, ports.RemoteCommand{Script: uploadTargetScript, Args: []string{root, relative}, OutputLimit: 16 << 10, Timeout: rootRequestTimeout})
	if err != nil {
		return "", err
	}
	parts := strings.Split(string(result.Output), "\x00")
	if len(parts) != 3 || parts[0] != uploadTargetMarker || parts[1] == "" || parts[2] != "" || !strings.HasPrefix(parts[1], "/") {
		return "", ErrProtocol
	}
	return path.Clean(parts[1]), nil
}

func (a *remoteFileSystemAdapter) UploadConflicts(ctx context.Context, id, root, target string, files []string) ([]string, error) {
	args := []string{root, target}
	args = append(args, files...)
	result, err := a.executor.RunRemote(ctx, id, ports.RemoteCommand{Script: uploadConflictScript, Args: args, OutputLimit: 1 << 20, Timeout: rootRequestTimeout})
	if err != nil {
		return nil, err
	}
	parts := strings.Split(string(result.Output), "\x00")
	if len(parts) < 3 || parts[0] != uploadConflictMarker || parts[len(parts)-2] != uploadConflictEnd || parts[len(parts)-1] != "" {
		return nil, ErrProtocol
	}
	return append([]string(nil), parts[1:len(parts)-2]...), nil
}

func (a *remoteFileSystemAdapter) RsyncAvailable(ctx context.Context, id string) (bool, error) {
	result, err := a.executor.RunRemote(ctx, id, ports.RemoteCommand{Script: rsyncProbeScript, OutputLimit: 64, Timeout: rootProbeTimeout})
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(string(result.Output)) == rsyncAvailableMarker, nil
}

func (a *remoteFileSystemAdapter) CreateDirectories(ctx context.Context, id, root, target string, directories []string) error {
	if len(directories) == 0 {
		return nil
	}
	args := []string{root, target}
	args = append(args, directories...)
	_, err := a.executor.RunRemote(ctx, id, ports.RemoteCommand{Script: remoteMkdirScript, Args: args, OutputLimit: 1024, Timeout: 10 * time.Second})
	return err
}

func (a *remoteFileSystemAdapter) ShouldUploadWithScp(ctx context.Context, id, target, relative string, modifiedAt int64) (bool, error) {
	result, err := a.executor.RunRemote(ctx, id, ports.RemoteCommand{Script: remoteMtimeScript, Args: []string{target, relative, strconv.FormatInt(modifiedAt, 10)}, OutputLimit: 64, Timeout: rootProbeTimeout})
	if err != nil {
		return false, err
	}
	value := strings.TrimSpace(string(result.Output))
	if value != "upload" && value != "skip" {
		return false, fmt.Errorf("%w: invalid upload decision", ErrProtocol)
	}
	return value == "upload", nil
}

func convertEntries(raw []rawEntry) []ports.RemoteFileEntry {
	entries := make([]ports.RemoteFileEntry, 0, len(raw))
	for _, value := range raw {
		entries = append(entries, convertEntry(value))
	}
	return entries
}

func convertEntry(value rawEntry) ports.RemoteFileEntry {
	return ports.RemoteFileEntry{Name: value.Name, Type: value.Type, Size: value.Size, ModifiedAt: value.ModifiedAt, Mode: value.Mode, Symlink: value.Symlink}
}

var _ ports.RemoteFileSystem = (*remoteFileSystemAdapter)(nil)
