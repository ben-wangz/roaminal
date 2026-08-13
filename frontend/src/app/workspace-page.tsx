import { RemoteMonitorBand } from '../status/remote-monitor-band';
import { TerminalRuntime } from '../terminal/terminal-runtime';
import { TerminalViewport } from '../terminal/terminal-viewport';
import { TerminalSearch } from '../terminal/terminal-search';
import { TouchKeyboard } from '../input/touch-keyboard';
import type { ConnectionInstanceSummary } from '../terminal/terminal-protocol';

type Props = {
  activeInstance: ConnectionInstanceSummary | null;
  activeRuntime: TerminalRuntime | null;
  currentConnection: ConnectionInstanceSummary | undefined;
  search: boolean;
  executionStatus: string | null;
  onCloseSearch: () => void;
  onOpenManager: () => void;
};

export function WorkspacePage({
  activeInstance,
  activeRuntime,
  currentConnection,
  search,
  executionStatus,
  onCloseSearch,
  onOpenManager,
}: Props) {
  return (
    <>
      <RemoteMonitorBand instance={activeInstance} />
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
  );
}
