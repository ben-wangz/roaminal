import { memo, type RefObject } from 'react';
import { ChevronsLeft, CircleHelp, Keyboard, PanelLeft } from 'lucide-react';
import type { WorkspaceMode } from '../app/workspace-page';
import type { WorkspaceTool } from '../app/workspace-tool';

type Props = {
  workspaceTool: WorkspaceTool;
  workspaceToolOpen: boolean;
  workspaceMode: WorkspaceMode;
  connectionToolButton: RefObject<HTMLButtonElement | null>;
  keyboardToolButton: RefObject<HTMLButtonElement | null>;
  onSelectWorkspaceTool: (tool: WorkspaceTool) => void;
  onCollapseWorkspaceTool: () => void;
  onHelp: () => void;
};

export const WorkspaceToolRail = memo(function WorkspaceToolRail({
  workspaceTool,
  workspaceToolOpen,
  workspaceMode,
  connectionToolButton,
  keyboardToolButton,
  onSelectWorkspaceTool,
  onCollapseWorkspaceTool,
  onHelp,
}: Props) {
  const connectionActive = workspaceTool === 'connections';
  const keyboardActive = workspaceTool === 'keyboard';
  return (
    <nav className="workspace-tool-rail" aria-label="Workspace tools">
      <div className="workspace-tool-rail-buttons">
        <button
          ref={connectionToolButton}
          className={`workspace-tool-button ${connectionActive ? 'active' : ''}`}
          type="button"
          onClick={() => onSelectWorkspaceTool('connections')}
          aria-label="Connections"
          title="Connections"
          aria-pressed={connectionActive}
          aria-expanded={connectionActive && workspaceToolOpen}
          aria-controls="workspace-tool-surface"
          data-testid="workspace-tool-connections"
        >
          <PanelLeft aria-hidden="true" size={18} />
        </button>
        <button
          ref={keyboardToolButton}
          className={`workspace-tool-button ${keyboardActive ? 'active' : ''}`}
          type="button"
          disabled={workspaceMode !== 'terminal'}
          onClick={() => onSelectWorkspaceTool('keyboard')}
          aria-label="Virtual keyboard"
          title={workspaceMode === 'terminal' ? 'Virtual keyboard' : 'Virtual keyboard is available in Terminal'}
          aria-pressed={keyboardActive}
          aria-expanded={keyboardActive && workspaceToolOpen}
          aria-controls="workspace-tool-surface"
          data-testid="workspace-tool-keyboard"
        >
          <Keyboard aria-hidden="true" size={18} />
        </button>
        <button
          className="workspace-tool-button workspace-tool-help"
          type="button"
          onClick={onHelp}
          aria-label="Help"
          title="Help"
          data-testid="workspace-tool-help"
        >
          <CircleHelp aria-hidden="true" size={18} />
        </button>
      </div>
      <button
        className="workspace-tool-button workspace-tool-rail-collapse"
        type="button"
        onClick={onCollapseWorkspaceTool}
        aria-label={`Collapse ${workspaceTool === 'connections' ? 'Connections' : 'Virtual keyboard'}`}
        title="Collapse workspace tool"
        aria-expanded={workspaceToolOpen}
        aria-controls="workspace-tool-surface"
        data-testid="workspace-tool-collapse"
      >
        <ChevronsLeft aria-hidden="true" size={18} />
      </button>
    </nav>
  );
});
