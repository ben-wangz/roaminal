import { RemoteMonitorBand } from '../status/remote-monitor-band';
import { TerminalRuntime } from '../terminal/terminal-runtime';
import { TerminalViewport } from '../terminal/terminal-viewport';
import { TerminalSearch } from '../terminal/terminal-search';
import { TouchKeyboard } from '../input/touch-keyboard';
import type { ConnectionInstanceSummary } from '../terminal/terminal-protocol';
import { TerminalSquare, FolderTree } from 'lucide-react';
import { FileSystemWorkspace } from '../filesystem/filesystem-workspace';

export type WorkspaceMode = 'terminal' | 'filesystem';

type Props = {
  connections: ConnectionInstanceSummary[];
  activeInstance: ConnectionInstanceSummary | null;
  activeRuntime: TerminalRuntime | null;
  currentConnection: ConnectionInstanceSummary | undefined;
  search: boolean;
  executionStatus: string | null;
  onCloseSearch: () => void;
  onOpenManager: () => void;
  mode: WorkspaceMode;
  onModeChange: (mode: WorkspaceMode) => void;
  onToast: (message: string, kind?: 'info' | 'success' | 'error') => void;
};

export function WorkspacePage({
  connections,
  activeInstance,
  activeRuntime,
  currentConnection,
  search,
  executionStatus,
  onCloseSearch,
  onOpenManager,
  mode,
  onModeChange,
  onToast,
}: Props) {
  return (
    <>
      <RemoteMonitorBand instance={activeInstance} />
      <nav className="workspace-mode-bar" aria-label="Workspace mode">
        <button className={`workspace-mode-button ${mode === 'terminal' ? 'active' : ''}`} type="button" aria-pressed={mode === 'terminal'} onClick={() => onModeChange('terminal')}><TerminalSquare size={15} aria-hidden="true" /> Terminal</button>
        <button className={`workspace-mode-button ${mode === 'filesystem' ? 'active' : ''}`} type="button" aria-pressed={mode === 'filesystem'} onClick={() => onModeChange('filesystem')}><FolderTree size={15} aria-hidden="true" /> FileSystem</button>
      </nav>
      <div className="workspace-mode-view terminal-mode-view" hidden={mode !== 'terminal'}>
        <>
          {search && activeRuntime && <TerminalSearch runtime={activeRuntime} onClose={onCloseSearch} />}
          <section className="terminal-stage">
            {activeRuntime ? (
              <TerminalViewport key={activeRuntime.connectionInstanceId} runtime={activeRuntime} />
            ) : (
              <div className="empty-state">
                <div className="brand-mark">
                  r<span>&gt;</span>
                </div>
                <button className="primary" onClick={onOpenManager}>
                  Open connection manager
                </button>
              </div>
            )}
          </section>
          {activeRuntime && <TouchKeyboard onInput={(value) => activeRuntime.input(value)} />}
          <footer className="statusbar">
            <span>{currentConnection?.cwd || 'No connection'}</span>
            <span className="execution-status" aria-live="polite">
              {executionStatus || (currentConnection ? `${currentConnection.cols}x${currentConnection.rows}` : '')}
            </span>
          </footer>
        </>
      </div>
      <div className="workspace-mode-view filesystem-mode-view" hidden={mode !== 'filesystem'}>
        {connections.some((connection) => connection.connectionInstanceId === activeInstance?.connectionInstanceId) ? connections.map((connection) => (
          <div className="filesystem-instance-view" hidden={connection.connectionInstanceId !== activeInstance?.connectionInstanceId} key={connection.connectionInstanceId}>
            <FileSystemWorkspace
              instance={connection}
              active={mode === 'filesystem' && connection.connectionInstanceId === activeInstance?.connectionInstanceId}
              onToast={onToast}
            />
          </div>
        )) : <FileSystemWorkspace instance={null} active onToast={onToast} />}
      </div>
    </>
  );
}
