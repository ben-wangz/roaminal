import { AlertTriangle, ChevronRight, Eye, EyeOff, LoaderCircle, RefreshCw, Upload } from 'lucide-react';
import { useCallback } from 'react';
import type { CSSProperties } from 'react';
import type { ConnectionInstanceSummary } from '../terminal/terminal-protocol';
import { AutoRefreshMenu } from './auto-refresh-menu';
import { FileContextMenu } from './file-context-menu';
import { FilePreview } from './file-preview';
import { FileTree } from './file-tree';
import { UploadConfirmDialog } from './upload-confirm-dialog';
import { useFilesystemWorkspace } from './use-filesystem-workspace';

type Props = {
  instance: ConnectionInstanceSummary | null;
  active: boolean;
  onToast: (message: string, kind?: 'info' | 'success' | 'error') => void;
};

export function FileSystemWorkspace({ instance, active, onToast }: Props) {
  const instanceId = instance?.connectionInstanceId || '';
  const workspace = useFilesystemWorkspace({ instanceId, active, onToast });
  const {
    root, rootError, entries, expanded, loading, errorPaths, selected, previewEntry, menu, uploadTarget, uploadStatus,
    showHidden, treeWidth, refreshing, autoRefreshDegraded, autoRefreshSeconds, resizeStart, rootEntry, breadcrumb,
    setSelected, setPreviewEntry, setMenu, setUploadTarget, setUploadStatus, setShowHidden, setTreeWidth,
    reloadRoot, refreshFileTree, refreshDirectory, toggle, openEntry, navigate, confirmUpload, cancelCurrentUpload,
    openMenu, openMenuAt, changeAutoRefresh,
  } = workspace;
  const handlePreviewRootChanged = useCallback(() => {
    void reloadRoot(true);
  }, [reloadRoot]);

  if (!active) return null;
  if (!instance) return <div className="filesystem-unavailable"><AlertTriangle size={20} aria-hidden="true" /><span>Select an SSH connection instance to browse files.</span></div>;
  if (rootError) return <div className="filesystem-unavailable"><AlertTriangle size={20} aria-hidden="true" /><div><strong>FileSystem unavailable</strong><small>{rootError.code || rootError.message}</small><button className="secondary" type="button" onClick={() => void reloadRoot()}><RefreshCw size={14} aria-hidden="true" /> Retry</button></div></div>;
  if (!root) return <div className="filesystem-unavailable"><LoaderCircle className="spin" size={20} aria-hidden="true" /> Probing remote root...</div>;

  return (
    <section className={`filesystem-workspace ${previewEntry ? 'previewing' : ''}`} style={{ '--filesystem-tree-width': `${treeWidth}px` } as CSSProperties} onContextMenu={(event) => event.preventDefault()}>
      <header className="filesystem-toolbar">
        <div className="filesystem-breadcrumb" aria-label="Current directory">
          {breadcrumb.map((part, index) => <span key={`${part}-${index}`}>{index > 0 && <ChevronRight size={13} aria-hidden="true" />}<button type="button" onClick={() => void navigate(index === 0 ? '.' : breadcrumb.slice(1, index + 1).join('/'))}>{index === 0 ? root.absolutePath : part}</button></span>)}
        </div>
        <div className="filesystem-root-status" data-status={root.status}><span className="connection-indicator" />{root.source === 'tmux' ? 'Active pane' : 'Configured fallback'}</div>
      </header>
      <div className="filesystem-columns">
        <aside className="filesystem-tree-panel">
          <div className="filesystem-panel-heading">
            <span>Files</span>
            <div className="filesystem-panel-actions">
              <button className="icon-button" type="button" onClick={() => setShowHidden((value) => !value)} title={showHidden ? 'Hide hidden files' : 'Show hidden files'} aria-label={showHidden ? 'Hide hidden files' : 'Show hidden files'}>{showHidden ? <EyeOff size={14} aria-hidden="true" /> : <Eye size={14} aria-hidden="true" />}</button>
              <button className="icon-button" type="button" disabled={refreshing} onClick={() => void refreshFileTree()} title="Refresh file tree" aria-label="Refresh file tree">{refreshing ? <LoaderCircle className="spin" size={14} aria-hidden="true" /> : <RefreshCw size={14} aria-hidden="true" />}</button>
              <AutoRefreshMenu value={autoRefreshSeconds} degraded={autoRefreshDegraded} disabled={refreshing} onChange={changeAutoRefresh} />
            </div>
          </div>
          {rootEntry && <FileTree rootEntry={rootEntry} entries={entries} showHidden={showHidden} expanded={expanded} selected={selected} loading={loading} errorPaths={errorPaths} onToggle={(entry) => void toggle(entry)} onSelect={(entry) => setSelected(entry.relativePath)} onOpen={(entry) => void openEntry(entry)} onContextMenu={openMenu} onRootContextMenu={openMenu} onOpenMenuAt={openMenuAt} />}
        </aside>
        <div className="filesystem-resizer" role="separator" aria-label="Resize file tree" aria-orientation="vertical" aria-valuemin={240} aria-valuemax={420} aria-valuenow={treeWidth} tabIndex={0} onPointerDown={(event) => { resizeStart.current = { x: event.clientX, width: treeWidth }; document.body.style.cursor = 'col-resize'; }} onKeyDown={(event) => { if (event.key === 'ArrowLeft' || event.key === 'ArrowRight') { event.preventDefault(); setTreeWidth((value) => Math.min(420, Math.max(240, value + (event.key === 'ArrowRight' ? 16 : -16)))); } }} />
        <div className="filesystem-preview-panel">
          <FilePreview instanceId={instanceId} root={root} entry={previewEntry} onClose={() => setPreviewEntry(null)} onToast={onToast} onRootChanged={handlePreviewRootChanged} />
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
