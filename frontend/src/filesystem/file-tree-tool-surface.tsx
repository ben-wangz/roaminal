import { AlertTriangle, ChevronRight, Eye, EyeOff, LoaderCircle, RefreshCw, Upload } from 'lucide-react';
import type { ConnectionInstanceSummary } from '../terminal/terminal-protocol';
import { AutoRefreshMenu } from './auto-refresh-menu';
import { FileContextMenu } from './file-context-menu';
import { FileTree } from './file-tree';
import { UploadConfirmDialog } from './upload-confirm-dialog';
import type { FileSystemWorkspaceState } from './use-filesystem-workspace';

type Props = {
  instance: ConnectionInstanceSummary | null;
  workspace: FileSystemWorkspaceState;
  onToast: (message: string, kind?: 'info' | 'success' | 'error') => void;
};

export function FileTreeToolSurface({ instance, workspace, onToast }: Props) {
  const {
    root, rootError, entries, expanded, loading, errorPaths, selected, menu, uploadTarget, uploadStatus,
    showHidden, refreshing, autoRefreshDegraded, autoRefreshSeconds, rootEntry, breadcrumb,
    setSelected, setMenu, setUploadTarget, setUploadStatus, setShowHidden,
    reloadRoot, refreshFileTree, refreshDirectory, toggle, openEntry, confirmUpload, cancelCurrentUpload,
    openMenu, openMenuAt, changeAutoRefresh,
  } = workspace;

  if (!instance) {
    return <div className="filesystem-unavailable"><AlertTriangle size={20} aria-hidden="true" /><span>Select an SSH connection instance to browse files.</span></div>;
  }
  const filesystemEligible = instance.type === 'ssh' && instance.lifecycle === 'live' && instance.purpose === 'interactive';
  if (!filesystemEligible) {
    return <div className="filesystem-unavailable"><AlertTriangle size={20} aria-hidden="true" /><span>File browsing requires a live interactive SSH connection instance.</span></div>;
  }
  if (!workspace.instanceReady) {
    return <div className="filesystem-unavailable"><LoaderCircle className="spin" size={20} aria-hidden="true" /> Probing remote root...</div>;
  }
  if (rootError) {
    const retryable = rootError.retryable !== false && rootError.code !== 'filesystem_no_transport' && rootError.code !== 'filesystem_unsupported';
    return <div className="filesystem-unavailable"><AlertTriangle size={20} aria-hidden="true" /><div><strong>FileSystem unavailable</strong><small>{rootError.code || rootError.message}</small>{retryable ? <button className="secondary" type="button" onClick={() => void reloadRoot()}><RefreshCw size={14} aria-hidden="true" /> Retry</button> : <small>Open a fresh SSH connection instance to restore remote file access.</small>}</div></div>;
  }
  if (!root) return <div className="filesystem-unavailable"><LoaderCircle className="spin" size={20} aria-hidden="true" /> Probing remote root...</div>;

  return (
    <section className="filesystem-tree-tool-surface" onContextMenu={(event) => event.preventDefault()}>
      <header className="filesystem-toolbar">
        <div className="filesystem-breadcrumb" aria-label="Current directory">
          {breadcrumb.map((part, index) => <span key={`${part}-${index}`}>{index > 0 && <ChevronRight size={13} aria-hidden="true" />}<button type="button" onClick={() => void workspace.navigate(index === 0 ? '.' : breadcrumb.slice(1, index + 1).join('/'))}>{index === 0 ? root.absolutePath : part}</button></span>)}
        </div>
        <div className="filesystem-root-status" data-status={root.status}><span className="connection-indicator" />{root.source === 'tmux' ? 'Active pane' : 'Configured fallback'}</div>
      </header>
      <div className="filesystem-tree-tool-body">
        <div className="filesystem-panel-heading">
          <span>Files</span>
          <div className="filesystem-panel-actions">
            <button className="icon-button" type="button" onClick={() => setShowHidden((value) => !value)} title={showHidden ? 'Hide hidden files' : 'Show hidden files'} aria-label={showHidden ? 'Hide hidden files' : 'Show hidden files'}>{showHidden ? <EyeOff size={14} aria-hidden="true" /> : <Eye size={14} aria-hidden="true" />}</button>
            <button className="icon-button" type="button" disabled={refreshing} onClick={() => void refreshFileTree()} title="Refresh file tree" aria-label="Refresh file tree" data-testid="filesystem-refresh">{refreshing ? <LoaderCircle className="spin" size={14} aria-hidden="true" /> : <RefreshCw size={14} aria-hidden="true" />}</button>
            <AutoRefreshMenu value={autoRefreshSeconds} degraded={autoRefreshDegraded} disabled={refreshing} onChange={changeAutoRefresh} />
          </div>
        </div>
        <div className="filesystem-tree-panel">
          {rootEntry && <FileTree rootEntry={rootEntry} entries={entries} showHidden={showHidden} expanded={expanded} selected={selected} loading={loading} errorPaths={errorPaths} onToggle={(entry) => void toggle(entry)} onSelect={(entry) => setSelected(entry.relativePath)} onOpen={(entry) => void openEntry(entry)} onContextMenu={openMenu} onRootContextMenu={openMenu} onOpenMenuAt={openMenuAt} />}
        </div>
      </div>
      {menu && <FileContextMenu entry={menu.entry} x={menu.x} y={menu.y} onClose={() => setMenu(null)} onUpload={setUploadTarget} onRefresh={(pathValue) => void refreshDirectory(pathValue)} onToast={onToast} />}
      {uploadTarget && <UploadConfirmDialog target={uploadTarget} onClose={() => setUploadTarget(null)} onConfirm={(files, policy) => void confirmUpload(files, policy)} />}
      {uploadStatus && !['completed', 'failed', 'partial-failure', 'cancelled'].includes(uploadStatus.status) && <div className="filesystem-upload-progress"><div><Upload size={15} aria-hidden="true" /><strong>Uploading {uploadStatus.currentPath || 'files'}</strong><span>{uploadStatus.transport} · {formatProgress(uploadStatus.bytesSent, uploadStatus.bytesTotal)}</span></div><button className="text-button" type="button" onClick={() => void cancelCurrentUpload()}>Cancel</button></div>}
      {uploadStatus && (uploadStatus.status === 'failed' || uploadStatus.status === 'partial-failure') && <div className="filesystem-upload-result error"><strong>Upload failed</strong><span>{uploadStatus.failures.map((failure) => failure.path || failure.code).join(', ')}</span><button className="text-button" type="button" onClick={() => setUploadStatus(null)}>Dismiss</button></div>}
    </section>
  );
}

function formatProgress(sent: number, total: number): string {
  if (!total) return 'preparing';
  return `${Math.round((sent / total) * 100)}%`;
}
