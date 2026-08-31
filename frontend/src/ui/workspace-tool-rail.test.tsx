import { renderToStaticMarkup } from 'react-dom/server';
import { describe, expect, it, vi } from 'vitest';
import { WorkspaceToolRail } from './workspace-tool-rail';

describe('workspace tool rail', () => {
  it('renders one icon-only switcher with active and disabled semantics', () => {
    const html = renderToStaticMarkup(
      <WorkspaceToolRail
        workspaceTool="connections"
        workspaceToolOpen={true}
        workspaceMode="filesystem"
        connectionToolButton={{ current: null }}
        keyboardToolButton={{ current: null }}
        onSelectWorkspaceTool={vi.fn()}
        onCollapseWorkspaceTool={vi.fn()}
        onHelp={vi.fn()}
      />,
    );

    expect(html).toContain('aria-label="Workspace tools"');
    expect(html).toContain('data-testid="workspace-tool-connections"');
    expect(html).toContain('data-testid="workspace-tool-keyboard"');
    expect(html).toContain('data-testid="workspace-tool-help"');
    expect(html).toContain('data-testid="workspace-tool-collapse"');
    expect(html).toContain('aria-pressed="true"');
    expect(html).toContain('disabled=""');
    expect(html).not.toContain('Connections</button>');
  });
});
