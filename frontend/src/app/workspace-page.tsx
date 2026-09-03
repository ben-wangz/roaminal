import { useMonitorDisclosure } from '../status/use-monitor-disclosure';
import { RemoteMonitorBand } from '../status/remote-monitor-band';
import { TerminalRuntime } from '../terminal/terminal-runtime';
import { TerminalViewport } from '../terminal/terminal-viewport';
import type { ConnectionInstanceSummary } from '../terminal/terminal-protocol';
import { FilePreviewWorkspace } from '../filesystem/file-preview-workspace';
import type { FileSystemWorkspaceState } from '../filesystem/use-filesystem-workspace';
import { TerminalFooter } from '../terminal/terminal-footer';
import type { WorkspaceContent } from './workspace-content';

type Props = {
  connections: ConnectionInstanceSummary[];
  activeInstance: ConnectionInstanceSummary | null;
  activeRuntime: TerminalRuntime | null;
  currentConnection: ConnectionInstanceSummary | undefined;
  executionStatus: string | null;
  onOpenManager: () => void;
  content: WorkspaceContent;
  filesystem: FileSystemWorkspaceState;
  onBackToTerminal: () => void;
  onToast: (message: string, kind?: 'info' | 'success' | 'error') => void;
};

export function WorkspacePage({
  connections,
  activeInstance,
  activeRuntime,
  currentConnection,
  executionStatus,
  onOpenManager,
  content,
  filesystem,
  onBackToTerminal,
  onToast,
}: Props) {
  const monitorDisclosure = useMonitorDisclosure(activeInstance?.connectionInstanceId || null);
  const previewEntry = filesystem.instanceReady && filesystem.instanceId === activeInstance?.connectionInstanceId ? filesystem.previewEntry : null;
  const previewActive = content === 'file-preview';
  const handlePreviewRootChanged = () => {
    void filesystem.reloadRoot(true);
  };
  const handleBackToTerminal = () => {
    onBackToTerminal();
    window.requestAnimationFrame(() => activeRuntime?.focus());
  };
  return (
    <>
      <RemoteMonitorBand
        instance={activeInstance}
        expanded={monitorDisclosure.expanded}
        onToggle={() => monitorDisclosure.setExpanded((value) => !value)}
      />
      <div className="workspace-body">
        <div
          className={`workspace-content-view terminal-content-view ${content === 'terminal' ? 'active' : 'inactive'}`}
          aria-hidden={content !== 'terminal'}
          inert={content !== 'terminal' || undefined}
        >
          <section className="terminal-stage">
            {activeRuntime ? (
              <TerminalViewport
                key={activeRuntime.connectionInstanceId}
                runtime={activeRuntime}
                active={content === 'terminal'}
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
            {content === 'terminal' && (
              <TerminalFooter
                connections={connections}
                currentConnection={currentConnection}
                activeRuntime={activeRuntime}
                executionStatus={executionStatus}
              />
            )}
          </section>
        </div>
        <div
          className={`workspace-content-view file-preview-content-view ${previewActive ? 'active' : 'inactive'}`}
          aria-hidden={!previewActive}
          inert={!previewActive || undefined}
        >
          <FilePreviewWorkspace
            instanceId={filesystem.instanceId}
            root={previewEntry ? filesystem.root : null}
            entry={previewEntry}
            onBackToTerminal={handleBackToTerminal}
            onToast={onToast}
            onRootChanged={handlePreviewRootChanged}
          />
        </div>
      </div>
    </>
  );
}
