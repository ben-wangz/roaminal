import { RemoteMonitorBand } from '../status/remote-monitor-band';
import { TerminalRuntime } from '../terminal/terminal-runtime';
import { TerminalViewport } from '../terminal/terminal-viewport';
import { TerminalSearch } from '../terminal/terminal-search';
import { MobileTerminalComposer } from '../input/mobile-terminal-composer';
import { useMobileKeyboard } from '../input/use-mobile-keyboard';
import { VirtualKeyboardDock } from '../input/virtual-keyboard-dock';
import type { ContextualMode } from '../input/contextual-keyboard-model';
import type { ConnectionInstanceSummary } from '../terminal/terminal-protocol';
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
  onToast: (message: string, kind?: 'info' | 'success' | 'error') => void;
  virtualKeyboardOpen: boolean;
  contextualMode: ContextualMode;
  onToggleVirtualKeyboard: () => void;
  onContextualModeChange: (mode: ContextualMode) => void;
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
  onToast,
  virtualKeyboardOpen,
  contextualMode,
  onToggleVirtualKeyboard,
  onContextualModeChange,
}: Props) {
  const filesystemInstance = connections.find(
    (connection) => connection.connectionInstanceId === activeInstance?.connectionInstanceId,
  ) || null;
  const mobileKeyboard = useMobileKeyboard(activeRuntime, mode === 'terminal' && Boolean(activeRuntime));
  return (
    <>
      <RemoteMonitorBand instance={activeInstance} />
      <div className="workspace-body">
        <div
          className={`workspace-mode-view terminal-mode-view ${mode === 'terminal' ? 'active' : 'inactive'}`}
          aria-hidden={mode !== 'terminal'}
          inert={mode !== 'terminal' || undefined}
        >
          {search && activeRuntime && <TerminalSearch runtime={activeRuntime} onClose={onCloseSearch} />}
          <section className="terminal-stage">
            {activeRuntime ? (
              <TerminalViewport
                key={activeRuntime.connectionInstanceId}
                runtime={activeRuntime}
                active={mode === 'terminal'}
              />
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
            {activeRuntime && <MobileTerminalComposer runtime={activeRuntime} active={mode === 'terminal'} keyboardOpen={mobileKeyboard.keyboardOpen} />}
          </section>
          <footer className="statusbar">
            <span>{currentConnection?.cwd || 'No connection'}</span>
            <span className="execution-status" aria-live="polite">
              {executionStatus || (currentConnection ? `${currentConnection.cols}x${currentConnection.rows}` : '')}
            </span>
          </footer>
        </div>
        <div
          className={`workspace-mode-view filesystem-mode-view ${mode === 'filesystem' ? 'active' : 'inactive'}`}
          aria-hidden={mode !== 'filesystem'}
          inert={mode !== 'filesystem' || undefined}
        >
          {connections.length > 0 ? connections.map((connection) => (
            <FileSystemWorkspace
              key={connection.connectionInstanceId}
              instance={connection}
              active={mode === 'filesystem' && connection.connectionInstanceId === filesystemInstance?.connectionInstanceId}
              onToast={onToast}
            />
          )) : <FileSystemWorkspace instance={null} active={mode === 'filesystem'} onToast={onToast} />}
        </div>
      </div>
      {mode === 'terminal' && <VirtualKeyboardDock
        open={virtualKeyboardOpen}
        instance={activeInstance}
        runtime={activeRuntime}
        mode={contextualMode}
        nativeKeyboardOpen={mobileKeyboard.keyboardOpen}
        onToggle={onToggleVirtualKeyboard}
        onModeChange={onContextualModeChange}
      />}
    </>
  );
}
