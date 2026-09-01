import { renderToStaticMarkup } from 'react-dom/server';
import { describe, expect, it, vi } from 'vitest';
import { FilePreviewWorkspace } from './file-preview-workspace';
import type { FileEntry, RootContext } from './filesystem-types';

const root: RootContext = {
  connectionInstanceId: 'instance-1',
  absolutePath: '/workspace',
  relativePath: '.',
  source: 'tmux',
  status: 'current',
  revision: 'revision-1',
  resolvedAt: '2026-09-01T00:00:00Z',
};

const entry: FileEntry = {
  name: 'README.md',
  relativePath: 'README.md',
  absolutePath: '/workspace/README.md',
  type: 'file',
  size: 10,
  modifiedAt: null,
  mode: 0,
  symlink: false,
};

describe('file preview workspace', () => {
  it('renders an icon-only Back to Terminal control for an active preview', () => {
    const html = renderToStaticMarkup(
      <FilePreviewWorkspace
        instanceId="instance-1"
        root={root}
        entry={entry}
        onBackToTerminal={vi.fn()}
        onToast={vi.fn()}
        onRootChanged={vi.fn()}
      />,
    );

    expect(html).toContain('aria-label="File preview"');
    expect(html).toContain('data-testid="file-preview-back-terminal"');
    expect(html).toContain('aria-label="Back to Terminal"');
    expect(html).not.toContain('Close preview');
  });

  it('keeps a back action available while the root is loading', () => {
    const html = renderToStaticMarkup(
      <FilePreviewWorkspace
        instanceId="instance-1"
        root={null}
        entry={entry}
        onBackToTerminal={vi.fn()}
        onToast={vi.fn()}
        onRootChanged={vi.fn()}
      />,
    );

    expect(html).toContain('Loading file preview');
    expect(html).toContain('data-testid="file-preview-back-terminal"');
  });
});
