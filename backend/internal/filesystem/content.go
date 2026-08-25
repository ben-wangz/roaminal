package filesystem

import (
	"context"
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
	reader, openErr := s.remote.OpenContent(ctx, id, root.AbsolutePath, clean, start, length)
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
	raw, runErr := s.remote.Stat(ctx, id, root.AbsolutePath, clean)
	if runErr != nil {
		return Entry{}, mapStatError(runErr)
	}
	return makeEntry(root, clean, raw), nil
}
