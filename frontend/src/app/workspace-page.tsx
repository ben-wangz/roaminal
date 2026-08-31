import { useMonitorDisclosure } from '../status/use-monitor-disclosure';
import { ChevronDown, ChevronUp, Terminal } from 'lucide-react';
import { RemoteMonitorBand } from '../status/remote-monitor-band';
import { TerminalRuntime } from '../terminal/terminal-runtime';
import { TerminalViewport } from '../terminal/terminal-viewport';
import { TerminalSearch } from '../terminal/terminal-search';
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
}: Props) {
  const filesystemInstance = connections.find(
    (connection) => connection.connectionInstanceId === activeInstance?.connectionInstanceId,
  ) || null;
  const monitorDisclosure = useMonitorDisclosure(activeInstance?.connectionInstanceId || null);
  return (
    <>
      {activeInstance && <WorkspaceContextBar instance={activeInstance} expanded={monitorDisclosure.expanded} onToggleMonitor={() => monitorDisclosure.setExpanded((value) => !value)} />}
      <RemoteMonitorBand
        instance={activeInstance}
        expanded={monitorDisclosure.expanded}
      />
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
    </>
  );
}

function WorkspaceContextBar({
  instance,
  expanded,
  onToggleMonitor,
}: {
  instance: ConnectionInstanceSummary;
  expanded: boolean;
  onToggleMonitor: () => void;
}) {
  const identity = instance.type === 'ssh' ? instance.sourceHostAlias || instance.title : instance.title || 'Local';
  const typeLabel = instance.type === 'ssh' ? 'SSH' : 'Local';
  return (
    <section className="workspace-context-bar" aria-label="Active connection">
      <div className="workspace-context-identity">
        <span className="workspace-context-icon" aria-hidden="true"><Terminal size={17} /></span>
        <div>
          <strong title={identity}>{identity}</strong>
          <span>{typeLabel}{instance.cwd ? ` | ${instance.cwd}` : ''}</span>
        </div>
      </div>
      {instance.type === 'ssh' && instance.lifecycle === 'live' && (
        <button
          className="monitor-disclosure workspace-context-monitor-toggle"
          type="button"
          onClick={onToggleMonitor}
          aria-label={expanded ? 'Collapse remote monitor' : 'Expand remote monitor'}
          title={expanded ? 'Collapse remote monitor' : 'Expand remote monitor'}
          aria-expanded={expanded}
          aria-controls="remote-monitor-metrics"
        >
          {expanded ? <ChevronUp size={14} aria-hidden="true" /> : <ChevronDown size={14} aria-hidden="true" />}
        </button>
      )}
    </section>
  );
}
