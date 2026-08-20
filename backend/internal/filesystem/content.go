package filesystem

import (
	"context"
	"fmt"
	"time"

	"github.com/ben-wangz/roaminal/backend/internal/connection"
)

func (s *Service) OpenContent(ctx context.Context, id, relative, revision string, start, length int64) (ContentStream, error) {
	root, err := s.rootForRevision(ctx, id, revision)
	if err != nil {
		return ContentStream{}, err
	}
	clean, err := ValidateRelativePath(relative)
	if err != nil {
		return ContentStream{}, err
	}
	entry, err := s.statAtRoot(ctx, id, root, clean)
	if err != nil {
		return ContentStream{}, err
	}
	if entry.Type != "file" || entry.Symlink || entry.Size == nil {
		return ContentStream{}, ErrContentUnavailable
	}
	total := *entry.Size
	if start < 0 || length < 0 || start > total || length > total-start {
		return ContentStream{}, ErrContentUnavailable
	}
	if length > maxContentStream {
		return ContentStream{}, ErrContentTooLarge
	}
	reader, openErr := s.executor.OpenRemote(ctx, id, connection.RemoteCommand{
		Script:  contentScript,
		Args:    []string{root.AbsolutePath, clean, fmt.Sprintf("%d", start), fmt.Sprintf("%d", length)},
		Timeout: 15 * time.Minute,
	})
	if openErr != nil {
		return ContentStream{}, mapRemoteError(openErr)
	}
	end := start
	if length > 0 {
		end += length - 1
	}
	return ContentStream{Reader: reader, Entry: entry, Root: root, Start: start, End: end, TotalSize: total, ContentLength: length}, nil
}

func (s *Service) statAtRoot(ctx context.Context, id string, root RootContext, clean string) (Entry, error) {
	result, runErr := s.executor.RunRemote(ctx, id, connection.RemoteCommand{
		Script:      statScript,
		Args:        []string{root.AbsolutePath, clean},
		OutputLimit: 32 << 10,
		Timeout:     5 * time.Second,
	})
	if runErr != nil {
		return Entry{}, mapStatError(runErr)
	}
	raw, parseErr := parseDirectory(result.Output)
	if parseErr != nil || len(raw) != 1 {
		return Entry{}, ErrProtocol
	}
	return makeEntry(root, clean, raw[0]), nil
}
