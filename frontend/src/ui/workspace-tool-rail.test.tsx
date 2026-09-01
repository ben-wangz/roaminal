import { renderToStaticMarkup } from 'react-dom/server';
import { describe, expect, it, vi } from 'vitest';
import { WorkspaceToolRail } from './workspace-tool-rail';

describe('workspace tool rail', () => {
  it('renders the icon-only three-tool switcher with active semantics', () => {
    const html = renderToStaticMarkup(
      <WorkspaceToolRail
        workspaceTool="connections"
        workspaceToolOpen={true}
        connectionToolButton={{ current: null }}
        keyboardToolButton={{ current: null }}
        filesToolButton={{ current: null }}
        connectionCount={4}
        onSelectWorkspaceTool={vi.fn()}
        onCollapseWorkspaceTool={vi.fn()}
        onHelp={vi.fn()}
      />,
    );

    expect(html).toContain('aria-label="Workspace tools"');
    expect(html).toContain('data-testid="workspace-tool-connections"');
    expect(html).toContain('data-testid="workspace-tool-keyboard"');
    expect(html).toContain('data-testid="workspace-tool-files"');
    expect(html).toContain('data-testid="workspace-tool-help"');
    expect(html).toContain('data-testid="workspace-tool-collapse"');
    expect(html).toContain('aria-pressed="true"');
    expect(html).toContain('aria-label="Connections, 4"');
    expect(html).toContain('data-testid="workspace-tool-connections-count">4</span>');
    expect(html).not.toContain('disabled=""');
    expect(html).not.toContain('Connections</button>');
  });

  it('marks Files as the selected tool and names its shared collapse action', () => {
    const html = renderToStaticMarkup(
      <WorkspaceToolRail
        workspaceTool="files"
        workspaceToolOpen={true}
        connectionToolButton={{ current: null }}
        keyboardToolButton={{ current: null }}
        filesToolButton={{ current: null }}
        connectionCount={0}
        onSelectWorkspaceTool={vi.fn()}
        onCollapseWorkspaceTool={vi.fn()}
        onHelp={vi.fn()}
      />,
    );

    expect(html).toContain('data-testid="workspace-tool-files"');
    expect(html).toContain('aria-label="Files"');
    expect(html).toContain('aria-expanded="true"');
    expect(html).toContain('aria-label="Collapse Files"');
  });
});
