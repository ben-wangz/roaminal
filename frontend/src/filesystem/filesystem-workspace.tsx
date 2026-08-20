import { AlertTriangle, ChevronRight, Eye, EyeOff, LoaderCircle, RefreshCw, Upload } from 'lucide-react';
import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import type { ConnectionInstanceSummary } from '../terminal/terminal-protocol';
import { cancelUpload, createUpload, loadEntries, loadRoot as loadRootContext, loadUpload } from './filesystem-api';
import { FileContextMenu } from './file-context-menu';
import { FilePreview } from './file-preview';
import { FileTree } from './file-tree';
import type { FileEntry, FileSystemError, LocalUploadFile, RootContext, UploadStatus } from './filesystem-types';
import { UploadConfirmDialog } from './upload-confirm-dialog';

type Props = {
  instance: ConnectionInstanceSummary | null;
  active: boolean;
  onToast: (message: string, kind?: 'info' | 'success' | 'error') => void;
};

type ContextState = { entry: FileEntry; x: number; y: number } | null;

export function FileSystemWorkspace({ instance, active, onToast }: Props) {
  const instanceId = instance?.connectionInstanceId || '';
  const [root, setRoot] = useState<RootContext | null>(null);
  const [rootError, setRootError] = useState<FileSystemError | null>(null);
  const [entries, setEntries] = useState<Map<string, FileEntry[]>>(new Map());
  const [expanded, setExpanded] = useState<Set<string>>(new Set());
  const [loading, setLoading] = useState<Set<string>>(new Set());
  const [errorPaths, setErrorPaths] = useState<Set<string>>(new Set());
  const [selected, setSelected] = useState<string | null>(null);
  const [previewEntry, setPreviewEntry] = useState<FileEntry | null>(null);
  const [currentPath, setCurrentPath] = useState('.');
  const [menu, setMenu] = useState<ContextState>(null);
  const [uploadTarget, setUploadTarget] = useState<FileEntry | null>(null);
  const [uploadStatus, setUploadStatus] = useState<UploadStatus | null>(null);
  const [showHidden, setShowHidden] = useState(true);
  const [treeWidth, setTreeWidth] = useState(290);
  const resizeStart = useRef<{ x: number; width: number } | null>(null);
  const loaded = useRef(false);

  const handleError = useCallback((reason: unknown, pathValue?: string) => {
    const error = (reason instanceof Error ? reason : new Error('FileSystem request failed')) as FileSystemError;
    if (error.code === 'filesystem_root_changed' && error.root) {
      setRoot(error.root);
      setEntries(new Map());
      setExpanded(new Set());
      setSelected(null);
      setPreviewEntry(null);
      setCurrentPath('.');
      onToast('Root changed. The file tree was refreshed.', 'info');
      return;
    }
    if (pathValue) setErrorPaths((current) => new Set(current).add(pathValue));
    else setRootError(error);
  }, [onToast]);

  const loadDirectory = useCallback(async (pathValue: string, revision: string) => {
    setLoading((current) => new Set(current).add(pathValue));
    setErrorPaths((current) => { const next = new Set(current); next.delete(pathValue); return next; });
    try {
      const all: FileEntry[] = [];
      let cursor: string | undefined;
      do {
        const result = await loadEntries(instanceId, pathValue, revision, cursor);
        all.push(...result.entries);
        cursor = result.nextCursor || undefined;
      } while (cursor);
      setEntries((current) => new Map(current).set(pathValue, all));
    } catch (error) {
      const filesystemError = (error instanceof Error ? error : new Error('FileSystem request failed')) as FileSystemError;
      handleError(filesystemError, pathValue);
      if (filesystemError.code === 'filesystem_root_changed' && filesystemError.root) {
        await loadDirectory('.', filesystemError.root.revision);
      }
    } finally {
      setLoading((current) => { const next = new Set(current); next.delete(pathValue); return next; });
    }
  }, [handleError, instanceId]);

  const reloadRoot = useCallback(async () => {
    if (!instanceId) return;
    setRootError(null);
    setRoot(null);
    setEntries(new Map());
    setExpanded(new Set());
    setSelected(null);
    setPreviewEntry(null);
    setCurrentPath('.');
    try {
      const nextRoot = await loadRootContext(instanceId);
      setRoot(nextRoot);
      await loadDirectory('.', nextRoot.revision);
    } catch (error) {
      handleError(error);
    }
  }, [handleError, instanceId, loadDirectory]);

  useEffect(() => {
    if (!active || !instanceId || loaded.current) return;
    loaded.current = true;
    void reloadRoot();
  }, [active, instanceId, reloadRoot]);
  useEffect(() => {
    if (!menu) return undefined;
    const close = () => setMenu(null);
    document.addEventListener('mousedown', close);
    return () => document.removeEventListener('mousedown', close);
  }, [menu]);
  useEffect(() => {
    const move = (event: PointerEvent) => {
      if (!resizeStart.current) return;
      setTreeWidth(Math.min(420, Math.max(240, resizeStart.current.width + event.clientX - resizeStart.current.x)));
    };
    const end = () => { resizeStart.current = null; document.body.style.cursor = ''; };
    document.addEventListener('pointermove', move);
    document.addEventListener('pointerup', end);
    return () => {
      document.removeEventListener('pointermove', move);
      document.removeEventListener('pointerup', end);
    };
  }, []);

  const toggle = async (entry: FileEntry) => {
    setSelected(entry.relativePath);
    if (expanded.has(entry.relativePath)) {
      setExpanded((current) => { const next = new Set(current); next.delete(entry.relativePath); return next; });
      return;
    }
    setExpanded((current) => new Set(current).add(entry.relativePath));
    if (!entries.has(entry.relativePath) && root) await loadDirectory(entry.relativePath, root.revision);
  };

  const openEntry = async (entry: FileEntry) => {
    setSelected(entry.relativePath);
    if (entry.type === 'directory') {
      setCurrentPath(entry.relativePath);
      if (!expanded.has(entry.relativePath)) await toggle(entry);
      return;
    }
    if (entry.type === 'file') setPreviewEntry(entry);
  };

  const refresh = async (pathValue: string) => {
    if (!root) return;
    setEntries((current) => {
      const next = new Map(current);
      next.delete(pathValue);
      return next;
    });
    await loadDirectory(pathValue, root.revision);
  };

  const navigate = async (pathValue: string) => {
    setCurrentPath(pathValue);
    if (root && !entries.has(pathValue)) await loadDirectory(pathValue, root.revision);
  };

  const handlePreviewRootChanged = useCallback(() => {
    void reloadRoot();
  }, [reloadRoot]);

  const confirmUpload = async (files: LocalUploadFile[], policy: 'refuse' | 'overwrite' | 'update-if-newer') => {
    if (!root || !uploadTarget) return;
    const manifest = {
      rootRevision: root.revision,
      targetPath: uploadTarget.relativePath,
      conflictPolicy: policy,
      files: files.map((item, index) => ({ part: `file-${index}`, relativePath: item.relativePath, size: item.file.size, modifiedAt: new Date(item.file.lastModified).toISOString() })),
    } as const;
    setUploadTarget(null);
    try {
      const status = await createUpload(instanceId, manifest, files);
      setUploadStatus(status);
      onToast('Upload queued.', 'info');
      void pollUpload(status.uploadId, manifest.targetPath);
    } catch (error) {
      handleError(error);
      onToast(error instanceof Error ? error.message : 'Unable to start upload.', 'error');
    }
  };

  const pollUpload = async (uploadId: string, targetPath: string) => {
    try {
      const status = await loadUpload(instanceId, uploadId);
      setUploadStatus(status);
      if (status.status === 'completed' || status.status === 'failed' || status.status === 'partial-failure' || status.status === 'cancelled') {
        if (status.status === 'completed') onToast('Upload completed.', 'success');
        else if (status.status !== 'cancelled') onToast('Upload finished with errors.', 'error');
        if (root) await refresh(targetPath);
        return;
      }
      window.setTimeout(() => void pollUpload(uploadId, targetPath), 650);
    } catch (error) {
      onToast(error instanceof Error ? error.message : 'Unable to read upload status.', 'error');
    }
  };

  const cancelCurrentUpload = async () => {
    if (!uploadStatus) return;
    try {
      const next = await cancelUpload(instanceId, uploadStatus.uploadId);
      setUploadStatus(next);
    } catch (error) {
      onToast(error instanceof Error ? error.message : 'Unable to cancel upload.', 'error');
    }
  };

  const breadcrumb = useMemo(() => currentPath === '.' ? ['.'] : ['.', ...currentPath.split('/')], [currentPath]);
  const rootEntry = useMemo<FileEntry | null>(() => root ? {
    name: root.absolutePath,
    relativePath: '.',
    absolutePath: root.absolutePath,
    type: 'directory',
    size: null,
    modifiedAt: null,
    mode: 0,
    symlink: false,
  } : null, [root]);
  const openMenu = useCallback((event: React.MouseEvent, entry: FileEntry) => {
    event.preventDefault();
    setSelected(entry.relativePath);
    setMenu({
      entry,
      x: Math.max(8, Math.min(event.clientX, window.innerWidth - 258)),
      y: Math.max(8, Math.min(event.clientY, window.innerHeight - 218)),
    });
  }, []);

  if (!active) return null;
  if (!instance) return <div className="filesystem-unavailable"><AlertTriangle size={20} aria-hidden="true" /><span>Select an SSH connection instance to browse files.</span></div>;
  if (rootError) return <div className="filesystem-unavailable"><AlertTriangle size={20} aria-hidden="true" /><div><strong>FileSystem unavailable</strong><small>{rootError.code || rootError.message}</small><button className="secondary" type="button" onClick={() => void reloadRoot()}><RefreshCw size={14} aria-hidden="true" /> Retry</button></div></div>;
  if (!root) return <div className="filesystem-unavailable"><LoaderCircle className="spin" size={20} aria-hidden="true" /> Probing remote root...</div>;

  return (
    <section className={`filesystem-workspace ${previewEntry ? 'previewing' : ''}`} style={{ '--filesystem-tree-width': `${treeWidth}px` } as React.CSSProperties} onContextMenu={(event) => event.preventDefault()}>
      <header className="filesystem-toolbar">
        <div className="filesystem-breadcrumb" aria-label="Current directory">
          {breadcrumb.map((part, index) => <span key={`${part}-${index}`}>{index > 0 && <ChevronRight size={13} aria-hidden="true" />}<button type="button" onClick={() => void navigate(index === 0 ? '.' : breadcrumb.slice(1, index + 1).join('/'))}>{index === 0 ? root.absolutePath : part}</button></span>)}
        </div>
        <div className="filesystem-root-status" data-status={root.status}><span className="connection-indicator" />{root.source === 'tmux' ? 'Active pane' : 'Configured fallback'}</div>
      </header>
      <div className="filesystem-columns">
        <aside className="filesystem-tree-panel">
          <div className="filesystem-panel-heading"><span>Files</span><div className="filesystem-panel-actions"><button className="icon-button" type="button" onClick={() => setShowHidden((value) => !value)} title={showHidden ? 'Hide hidden files' : 'Show hidden files'} aria-label={showHidden ? 'Hide hidden files' : 'Show hidden files'}>{showHidden ? <Eye size={14} aria-hidden="true" /> : <EyeOff size={14} aria-hidden="true" />}</button><button className="icon-button" type="button" onClick={() => void refresh('.')} title="Refresh files" aria-label="Refresh files"><RefreshCw size={14} aria-hidden="true" /></button></div></div>
          {rootEntry && <FileTree rootEntry={rootEntry} entries={entries} showHidden={showHidden} expanded={expanded} selected={selected} loading={loading} errorPaths={errorPaths} onToggle={(entry) => void toggle(entry)} onSelect={(entry) => setSelected(entry.relativePath)} onOpen={(entry) => void openEntry(entry)} onContextMenu={openMenu} onRootContextMenu={openMenu} onRefresh={(pathValue) => void refresh(pathValue)} />}
        </aside>
        <div className="filesystem-resizer" role="separator" aria-label="Resize file tree" aria-orientation="vertical" aria-valuemin={240} aria-valuemax={420} aria-valuenow={treeWidth} tabIndex={0} onPointerDown={(event) => { resizeStart.current = { x: event.clientX, width: treeWidth }; document.body.style.cursor = 'col-resize'; }} onKeyDown={(event) => { if (event.key === 'ArrowLeft' || event.key === 'ArrowRight') { event.preventDefault(); setTreeWidth((value) => Math.min(420, Math.max(240, value + (event.key === 'ArrowRight' ? 16 : -16)))); } }} />
        <div className="filesystem-preview-panel">
          <FilePreview instanceId={instanceId} root={root} entry={previewEntry} onClose={() => setPreviewEntry(null)} onToast={onToast} onRootChanged={handlePreviewRootChanged} />
        </div>
      </div>
      {menu && <FileContextMenu entry={menu.entry} x={menu.x} y={menu.y} onClose={() => setMenu(null)} onUpload={setUploadTarget} onRefresh={(pathValue) => void refresh(pathValue)} onToast={onToast} />}
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
