import { renderToStaticMarkup } from 'react-dom/server';
import { describe, expect, it, vi } from 'vitest';
import { act, create, type ReactTestRenderer } from 'react-test-renderer';
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
        settingsToolButton={{ current: null }}
        settingsActive={false}
        connectionCount={12}
        agentRelaxCount={4}
        onSelectWorkspaceTool={vi.fn()}
        onCollapseWorkspaceTool={vi.fn()}
        onHelp={vi.fn()}
        onOpenSettings={vi.fn()}
      />,
    );

    expect(html).toContain('aria-label="Application tools"');
    expect(html).toContain('data-testid="workspace-tool-connections"');
    expect(html).toContain('data-testid="workspace-tool-keyboard"');
    expect(html).toContain('data-testid="workspace-tool-files"');
    expect(html).toContain('data-testid="workspace-tool-settings"');
    expect(html).toContain('data-testid="workspace-tool-help"');
    expect(html).toContain('data-testid="workspace-tool-collapse"');
    expect(html).toContain('aria-pressed="true"');
    expect(html).toContain('aria-label="Connections, 12 connections, 4 relaxed agents"');
    expect(html).toContain('data-testid="workspace-tool-connections-count">12</span>');
    expect(html).toContain('data-testid="workspace-tool-connections-agent-relax-count">4</span>');
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
        settingsToolButton={{ current: null }}
        settingsActive={false}
        connectionCount={0}
        agentRelaxCount={0}
        onSelectWorkspaceTool={vi.fn()}
        onCollapseWorkspaceTool={vi.fn()}
        onHelp={vi.fn()}
        onOpenSettings={vi.fn()}
      />,
    );

    expect(html).toContain('data-testid="workspace-tool-files"');
    expect(html).toContain('aria-label="Files"');
    expect(html).toContain('aria-expanded="true"');
    expect(html).toContain('aria-label="Collapse Files"');
  });

  it('opens Settings without forwarding the browser click event', () => {
    const onOpenSettings = vi.fn();
    let renderer: ReactTestRenderer | undefined;
    act(() => {
      renderer = create(
        <WorkspaceToolRail
          workspaceTool="connections"
          workspaceToolOpen={false}
          connectionToolButton={{ current: null }}
          keyboardToolButton={{ current: null }}
          filesToolButton={{ current: null }}
          settingsToolButton={{ current: null }}
          settingsActive={false}
          connectionCount={0}
          agentRelaxCount={0}
          onSelectWorkspaceTool={vi.fn()}
          onCollapseWorkspaceTool={vi.fn()}
          onHelp={vi.fn()}
          onOpenSettings={onOpenSettings}
        />,
      );
    });

    const settingsButton = renderer?.root.findByProps({ 'data-testid': 'workspace-tool-settings' });
    act(() => {
      settingsButton?.props.onClick({ type: 'click' });
    });

    expect(onOpenSettings).toHaveBeenCalledOnce();
    expect(onOpenSettings).toHaveBeenCalledWith();
    renderer?.unmount();
  });
});
