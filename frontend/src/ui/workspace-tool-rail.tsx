import { memo, type RefObject } from 'react';
import { ChevronsLeft, CircleHelp, FolderTree, Keyboard, PanelLeft, Settings } from 'lucide-react';
import type { WorkspaceTool } from '../app/workspace-tool';

type Props = {
  workspaceTool: WorkspaceTool;
  workspaceToolOpen: boolean;
  connectionToolButton: RefObject<HTMLButtonElement | null>;
  keyboardToolButton: RefObject<HTMLButtonElement | null>;
  filesToolButton: RefObject<HTMLButtonElement | null>;
  settingsToolButton: RefObject<HTMLButtonElement | null>;
  settingsActive: boolean;
  connectionCount: number;
  agentRelaxCount: number;
  onSelectWorkspaceTool: (tool: WorkspaceTool) => void;
  onCollapseWorkspaceTool: () => void;
  onHelp: () => void;
  onOpenSettings: () => void;
};

export const WorkspaceToolRail = memo(function WorkspaceToolRail({
  workspaceTool,
  workspaceToolOpen,
  connectionToolButton,
  keyboardToolButton,
  filesToolButton,
  settingsToolButton,
  settingsActive,
  connectionCount,
  agentRelaxCount,
  onSelectWorkspaceTool,
  onCollapseWorkspaceTool,
  onHelp,
  onOpenSettings,
}: Props) {
  const connectionActive = workspaceTool === 'connections';
  const keyboardActive = workspaceTool === 'keyboard';
  const filesActive = workspaceTool === 'files';
  return (
    <nav className="workspace-tool-rail" aria-label="Application tools">
      <div className="workspace-tool-rail-buttons">
        <button
          ref={connectionToolButton}
          className={`workspace-tool-button ${connectionActive ? 'active' : ''}`}
          type="button"
          onClick={() => onSelectWorkspaceTool('connections')}
          aria-label={`Connections, ${connectionCount} connections, ${agentRelaxCount} relaxed agents`}
          title={`Connections (${connectionCount}; ${agentRelaxCount} relaxed agents)`}
          aria-pressed={connectionActive}
          aria-expanded={connectionActive && workspaceToolOpen}
          aria-controls="workspace-tool-surface"
          data-testid="workspace-tool-connections"
        >
          <PanelLeft aria-hidden="true" size={18} />
          <span className="workspace-tool-count-badge" aria-hidden="true" data-testid="workspace-tool-connections-count">{connectionCount}</span>
          <span className="workspace-tool-agent-relax-badge" aria-hidden="true" data-testid="workspace-tool-connections-agent-relax-count">{agentRelaxCount}</span>
        </button>
        <button
          ref={keyboardToolButton}
          className={`workspace-tool-button ${keyboardActive ? 'active' : ''}`}
          type="button"
          onClick={() => onSelectWorkspaceTool('keyboard')}
          aria-label="Virtual keyboard"
          title="Virtual keyboard"
          aria-pressed={keyboardActive}
          aria-expanded={keyboardActive && workspaceToolOpen}
          aria-controls="workspace-tool-surface"
          data-testid="workspace-tool-keyboard"
        >
          <Keyboard aria-hidden="true" size={18} />
        </button>
        <button
          ref={filesToolButton}
          className={`workspace-tool-button ${filesActive ? 'active' : ''}`}
          type="button"
          onClick={() => onSelectWorkspaceTool('files')}
          aria-label="Files"
          title="Files"
          aria-pressed={filesActive}
          aria-expanded={filesActive && workspaceToolOpen}
          aria-controls="workspace-tool-surface"
          data-testid="workspace-tool-files"
        >
          <FolderTree aria-hidden="true" size={18} />
        </button>
        <button
          ref={settingsToolButton}
          className={`workspace-tool-button workspace-tool-settings ${settingsActive ? 'active' : ''}`}
          type="button"
          onClick={() => onOpenSettings()}
          aria-label="Settings"
          title="Settings"
          aria-pressed={settingsActive}
          data-testid="workspace-tool-settings"
        >
          <Settings aria-hidden="true" size={18} />
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
        aria-label={`Collapse ${workspaceTool === 'connections' ? 'Connections' : workspaceTool === 'keyboard' ? 'Virtual keyboard' : 'Files'}`}
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
