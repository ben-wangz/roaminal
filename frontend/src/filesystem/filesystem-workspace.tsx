import { AlertTriangle, ChevronRight, Eye, EyeOff, LoaderCircle, RefreshCw, Upload } from 'lucide-react';
import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import type { ConnectionInstanceSummary } from '../terminal/terminal-protocol';
import { AutoRefreshMenu } from './auto-refresh-menu';
import { readAutoRefreshSeconds, writeAutoRefreshSeconds } from './auto-refresh-settings';
import { cancelUpload, createUpload, loadEntries, loadRoot as loadRootContext, loadUpload } from './filesystem-api';
import { FileContextMenu } from './file-context-menu';
import { FilePreview } from './file-preview';
import { FileTree } from './file-tree';
import type { FileEntry, FileSystemError, LocalUploadFile, RootContext, UploadStatus } from './filesystem-types';
import { UploadConfirmDialog } from './upload-confirm-dialog';

const MAX_REFRESH_CONCURRENCY = 3;

type Props = {
  instance: ConnectionInstanceSummary | null;
  active: boolean;
  onToast: (message: string, kind?: 'info' | 'success' | 'error') => void;
};

type ContextState = { entry: FileEntry; x: number; y: number } | null;

async function fetchAllEntries(instanceId: string, pathValue: string, revision: string, signal?: AbortSignal): Promise<FileEntry[]> {
  const all: FileEntry[] = [];
  let cursor: string | undefined;
  do {
    const result = await loadEntries(instanceId, pathValue, revision, cursor, signal);
    all.push(...result.entries);
    cursor = result.nextCursor || undefined;
  } while (cursor);
  return all;
}

async function runWithConcurrency<T>(items: string[], limit: number, worker: (item: string) => Promise<T>): Promise<T[]> {
  const results = new Array<T>(items.length);
  let nextIndex = 0;
  const run = async () => {
    while (nextIndex < items.length) {
      const index = nextIndex;
      nextIndex += 1;
      results[index] = await worker(items[index]);
    }
  };
  await Promise.all(Array.from({ length: Math.min(limit, items.length) }, () => run()));
  return results;
}

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
  const [refreshing, setRefreshing] = useState(false);
  const [autoRefreshDegraded, setAutoRefreshDegraded] = useState(false);
  const [autoRefreshSeconds, setAutoRefreshSeconds] = useState(readAutoRefreshSeconds);
  const resizeStart = useRef<{ x: number; width: number } | null>(null);
  const rootRef = useRef<RootContext | null>(null);
  const entriesRef = useRef<Map<string, FileEntry[]>>(new Map());
  const expandedRef = useRef<Set<string>>(new Set());
  const requestGeneration = useRef(new Map<string, number>());
  const requestControllers = useRef(new Map<string, AbortController>());
  const globalController = useRef<AbortController | null>(null);
  const globalRefreshInFlight = useRef(false);
  const lastGlobalRefreshAt = useRef<number | null>(null);
  const autoRefreshDegradedRef = useRef(false);
  const activeRef = useRef(active);
  const loadedInstanceId = useRef('');
  const previousInstanceId = useRef(instanceId);

  const updateRoot = useCallback((next: RootContext | null) => {
    rootRef.current = next;
    setRoot(next);
  }, []);

  const updateEntries = useCallback((next: Map<string, FileEntry[]>) => {
    entriesRef.current = next;
    setEntries(next);
  }, []);

  const updateExpanded = useCallback((next: Set<string>) => {
    expandedRef.current = next;
    setExpanded(next);
  }, []);

  const pruneDirectory = useCallback((pathValue: string) => {
    const prefix = pathValue === '.' ? '' : `${pathValue}/`;
    const matchesPath = (value: string) => pathValue === '.' || value === pathValue || value.startsWith(prefix);
    const nextEntries = new Map(entriesRef.current);
    for (const key of nextEntries.keys()) {
      if (matchesPath(key)) nextEntries.delete(key);
    }
    updateEntries(nextEntries);
    const nextExpanded = new Set([...expandedRef.current].filter((key) => !matchesPath(key)));
    updateExpanded(nextExpanded);
    setLoading((current) => new Set([...current].filter((key) => !matchesPath(key))));
    setErrorPaths((current) => new Set([...current].filter((key) => !matchesPath(key))));
    for (const [key, controller] of requestControllers.current) {
      if (!matchesPath(key)) continue;
      controller.abort();
      requestControllers.current.delete(key);
      requestGeneration.current.set(key, (requestGeneration.current.get(key) || 0) + 1);
    }
    if ((selected && matchesPath(selected)) || (previewEntry && matchesPath(previewEntry.relativePath))) {
      setSelected(null);
      setPreviewEntry(null);
    }
  }, [previewEntry, selected, updateEntries, updateExpanded]);

  const pruneMissingDescendants = useCallback((pathValue: string) => {
    const prefix = pathValue === '.' ? '' : `${pathValue}/`;
    const nextEntries = new Map(entriesRef.current);
    const removed = new Set<string>();
    const candidatePaths = new Set([
      ...nextEntries.keys(),
      ...expandedRef.current,
      ...requestControllers.current.keys(),
    ]);
    const candidates = [...candidatePaths]
      .filter((key) => key !== pathValue && (pathValue === '.' || key.startsWith(prefix)))
      .sort((left, right) => left.split('/').length - right.split('/').length);
    for (const key of candidates) {
      const slash = key.lastIndexOf('/');
      const parent = slash < 0 ? '.' : key.slice(0, slash);
      if (removed.has(parent)) {
        removed.add(key);
        continue;
      }
      const stillDirectory = nextEntries.get(parent)?.some((entry) => entry.relativePath === key && entry.type === 'directory');
      if (!stillDirectory) removed.add(key);
    }
    if (!removed.size) return;
    for (const key of removed) nextEntries.delete(key);
    updateEntries(nextEntries);
    const isRemovedPath = (value: string) => [...removed].some((path) => value === path || value.startsWith(`${path}/`));
    for (const key of removed) {
      requestControllers.current.get(key)?.abort();
      requestControllers.current.delete(key);
      requestGeneration.current.set(key, (requestGeneration.current.get(key) || 0) + 1);
    }
    const shouldRemove = (key: string) => isRemovedPath(key);
    updateExpanded(new Set([...expandedRef.current].filter((key) => !shouldRemove(key))));
    setLoading((current) => new Set([...current].filter((key) => !shouldRemove(key))));
    setErrorPaths((current) => new Set([...current].filter((key) => !shouldRemove(key))));
    if ((selected && isRemovedPath(selected)) || (previewEntry && isRemovedPath(previewEntry.relativePath))) {
      setSelected(null);
      setPreviewEntry(null);
    }
  }, [previewEntry, selected, updateEntries, updateExpanded]);

  const resetTree = useCallback((clearRoot: boolean) => {
    const nextEntries = new Map<string, FileEntry[]>();
    const nextExpanded = new Set<string>();
    entriesRef.current = nextEntries;
    expandedRef.current = nextExpanded;
    setEntries(nextEntries);
    setExpanded(nextExpanded);
    setLoading(new Set());
    setErrorPaths(new Set());
    setSelected(null);
    setPreviewEntry(null);
    setCurrentPath('.');
    if (clearRoot) updateRoot(null);
  }, [updateRoot]);

  const beginDirectoryRequest = (pathValue: string) => {
    requestControllers.current.get(pathValue)?.abort();
    const generation = (requestGeneration.current.get(pathValue) || 0) + 1;
    requestGeneration.current.set(pathValue, generation);
    const controller = new AbortController();
    requestControllers.current.set(pathValue, controller);
    return { controller, generation };
  };

  const isCurrentRequest = (pathValue: string, generation: number) => activeRef.current && requestGeneration.current.get(pathValue) === generation;

  const reportError = useCallback((reason: unknown, pathValue?: string) => {
    if (reason instanceof DOMException && reason.name === 'AbortError') return;
    const error = (reason instanceof Error ? reason : new Error('FileSystem request failed')) as FileSystemError;
    if (pathValue) {
      setErrorPaths((current) => {
        const next = new Set(current);
        next.add(pathValue);
        return next;
      });
    } else if (!rootRef.current) {
      setRootError(error);
    } else {
      onToast(error.message || 'FileSystem request failed.', 'error');
    }
  }, [onToast]);

  const markAutoRefreshFailure = useCallback(() => {
    if (autoRefreshDegradedRef.current) return;
    autoRefreshDegradedRef.current = true;
    setAutoRefreshDegraded(true);
    onToast('Automatic FileSystem refresh failed; showing the last successful tree.', 'error');
  }, [onToast]);

  const clearAutoRefreshFailure = useCallback(() => {
    autoRefreshDegradedRef.current = false;
    setAutoRefreshDegraded(false);
  }, []);

  const loadRootEntries = useCallback(async (nextRoot: RootContext, signal?: AbortSignal): Promise<boolean> => {
    const pathValue = '.';
    const request = beginDirectoryRequest(pathValue);
    setLoading((current) => new Set(current).add(pathValue));
    setErrorPaths((current) => {
      const next = new Set(current);
      next.delete(pathValue);
      return next;
    });
    try {
      const all = await fetchAllEntries(instanceId, pathValue, nextRoot.revision, signal || request.controller.signal);
      if (!isCurrentRequest(pathValue, request.generation) || rootRef.current?.revision !== nextRoot.revision) return false;
      updateEntries(new Map(entriesRef.current).set(pathValue, all));
      pruneMissingDescendants(pathValue);
      setRootError(null);
      return true;
    } catch (error) {
      if (!(error instanceof DOMException && error.name === 'AbortError') && isCurrentRequest(pathValue, request.generation)) {
        reportError(error, pathValue);
        if (!entriesRef.current.has(pathValue)) setRootError((error instanceof Error ? error : new Error('Unable to list root')) as FileSystemError);
      }
      return false;
    } finally {
      if (isCurrentRequest(pathValue, request.generation)) {
        setLoading((current) => {
          const next = new Set(current);
          next.delete(pathValue);
          return next;
        });
      }
    }
  }, [instanceId, pruneMissingDescendants, reportError, updateEntries]);

  const recoverRoot = useCallback(async (nextRoot: RootContext, signal?: AbortSignal) => {
    updateRoot(nextRoot);
    resetTree(false);
    setRootError(null);
    onToast('Root changed. The file tree was refreshed.', 'info');
    await loadRootEntries(nextRoot, signal);
  }, [loadRootEntries, onToast, resetTree, updateRoot]);

  const loadDirectory = useCallback(async (pathValue: string, revision: string, signal?: AbortSignal): Promise<boolean> => {
    const request = beginDirectoryRequest(pathValue);
    setLoading((current) => new Set(current).add(pathValue));
    setErrorPaths((current) => {
      const next = new Set(current);
      next.delete(pathValue);
      return next;
    });
    try {
      const all = await fetchAllEntries(instanceId, pathValue, revision, signal || request.controller.signal);
      if (!isCurrentRequest(pathValue, request.generation) || rootRef.current?.revision !== revision) return false;
      updateEntries(new Map(entriesRef.current).set(pathValue, all));
      pruneMissingDescendants(pathValue);
      return true;
    } catch (reason) {
      const error = (reason instanceof Error ? reason : new Error('FileSystem request failed')) as FileSystemError;
      if (error instanceof DOMException && error.name === 'AbortError') return false;
      if (!isCurrentRequest(pathValue, request.generation)) return false;
      if (error.code === 'filesystem_root_changed' && error.root) {
        await recoverRoot(error.root, signal);
      } else if (error.code === 'filesystem_not_found') {
        pruneDirectory(pathValue);
      } else {
        reportError(error, pathValue);
      }
      return false;
    } finally {
      if (isCurrentRequest(pathValue, request.generation)) {
        setLoading((current) => {
          const next = new Set(current);
          next.delete(pathValue);
          return next;
        });
      }
    }
  }, [instanceId, pruneDirectory, pruneMissingDescendants, recoverRoot, reportError, updateEntries]);

  const reloadRoot = useCallback(async (): Promise<boolean> => {
    if (!instanceId) return false;
    globalController.current?.abort();
    requestControllers.current.forEach((controller) => controller.abort());
    setRootError(null);
    resetTree(true);
    try {
      const nextRoot = await loadRootContext(instanceId);
      updateRoot(nextRoot);
      const loaded = await loadRootEntries(nextRoot);
      if (!loaded) return false;
      lastGlobalRefreshAt.current = Date.now();
      clearAutoRefreshFailure();
      return true;
    } catch (error) {
      reportError(error);
      return false;
    }
  }, [clearAutoRefreshFailure, instanceId, loadRootEntries, reportError, resetTree, updateRoot]);

  const refreshFileTree = useCallback(async (automatic = false): Promise<boolean> => {
    const currentRoot = rootRef.current;
    if (!instanceId || !currentRoot || globalRefreshInFlight.current) return false;
    globalRefreshInFlight.current = true;
    const controller = new AbortController();
    globalController.current = controller;
    setRefreshing(true);
    const paths = ['.', ...Array.from(expandedRef.current).filter((pathValue) => pathValue !== '.')];
    try {
      const nextRoot = await loadRootContext(instanceId, controller.signal);
      if (nextRoot.revision !== currentRoot.revision) {
        await recoverRoot(nextRoot, controller.signal);
        lastGlobalRefreshAt.current = Date.now();
        clearAutoRefreshFailure();
        return true;
      }
      updateRoot(nextRoot);
      const results = await runWithConcurrency(paths, MAX_REFRESH_CONCURRENCY, (pathValue) => loadDirectory(pathValue, nextRoot.revision, controller.signal));
      const succeeded = results.every(Boolean);
      if (succeeded) {
        lastGlobalRefreshAt.current = Date.now();
        clearAutoRefreshFailure();
      }
      if (!succeeded && automatic) markAutoRefreshFailure();
      if (!succeeded && !automatic) onToast('Some expanded directories could not be refreshed.', 'error');
      return succeeded;
    } catch (error) {
      if (!(error instanceof DOMException && error.name === 'AbortError')) {
        if (automatic) markAutoRefreshFailure();
        else onToast('Unable to refresh the file tree.', 'error');
      }
      return false;
    } finally {
      if (globalController.current === controller) globalController.current = null;
      globalRefreshInFlight.current = false;
      setRefreshing(false);
    }
  }, [clearAutoRefreshFailure, instanceId, loadDirectory, markAutoRefreshFailure, onToast, recoverRoot, updateRoot]);

  const refreshDirectory = useCallback(async (pathValue: string) => {
    const currentRoot = rootRef.current;
    if (!currentRoot) return;
    await loadDirectory(pathValue, currentRoot.revision);
  }, [loadDirectory]);

  useEffect(() => {
    if (previousInstanceId.current === instanceId) return;
    previousInstanceId.current = instanceId;
    loadedInstanceId.current = '';
    globalController.current?.abort();
    requestControllers.current.forEach((controller) => controller.abort());
    resetTree(true);
    setRootError(null);
  }, [instanceId, resetTree]);

  useEffect(() => {
    activeRef.current = active;
    if (active) return;
    globalController.current?.abort();
    requestControllers.current.forEach((controller) => controller.abort());
    globalRefreshInFlight.current = false;
    setRefreshing(false);
    setLoading(new Set());
    if (!rootRef.current) loadedInstanceId.current = '';
  }, [active]);

  useEffect(() => {
    if (!active || !instanceId || loadedInstanceId.current === instanceId) return;
    loadedInstanceId.current = instanceId;
    void reloadRoot();
  }, [active, instanceId, reloadRoot]);

  useEffect(() => {
    if (!active || !instanceId || autoRefreshSeconds === 0) return undefined;
    let timer: number | undefined;
    let disposed = false;
    const interval = autoRefreshSeconds * 1000;
    const performRefresh = async () => {
      if (rootRef.current) return refreshFileTree(true);
      const succeeded = await reloadRoot();
      if (!succeeded) markAutoRefreshFailure();
      return succeeded;
    };
    const schedule = (delayOverride?: number) => {
      if (disposed || document.visibilityState !== 'visible') return;
      const elapsed = lastGlobalRefreshAt.current === null ? 0 : Date.now() - lastGlobalRefreshAt.current;
      timer = window.setTimeout(async () => {
        timer = undefined;
        if (disposed || document.visibilityState !== 'visible') return;
        const due = lastGlobalRefreshAt.current === null || Date.now() - lastGlobalRefreshAt.current >= interval;
        if (!due) {
          schedule();
          return;
        }
        const succeeded = await performRefresh();
        schedule(succeeded ? undefined : interval);
      }, delayOverride === undefined ? Math.max(250, interval - elapsed) : delayOverride);
    };
    const onVisibilityChange = () => {
      if (document.visibilityState === 'visible') {
        const overdue = lastGlobalRefreshAt.current === null || Date.now() - lastGlobalRefreshAt.current >= interval;
        if (overdue) {
          void performRefresh().then((succeeded) => schedule(succeeded ? undefined : interval));
        } else {
          schedule();
        }
      } else if (timer !== undefined) {
        window.clearTimeout(timer);
        timer = undefined;
      }
    };
    document.addEventListener('visibilitychange', onVisibilityChange);
    schedule();
    return () => {
      disposed = true;
      if (timer !== undefined) window.clearTimeout(timer);
      document.removeEventListener('visibilitychange', onVisibilityChange);
    };
  }, [active, autoRefreshSeconds, instanceId, markAutoRefreshFailure, reloadRoot, refreshFileTree]);

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
    if (expandedRef.current.has(entry.relativePath)) {
      const next = new Set(expandedRef.current);
      next.delete(entry.relativePath);
      updateExpanded(next);
      return;
    }
    updateExpanded(new Set(expandedRef.current).add(entry.relativePath));
    if (rootRef.current) await loadDirectory(entry.relativePath, rootRef.current.revision);
  };

  const openEntry = async (entry: FileEntry) => {
    setSelected(entry.relativePath);
    if (entry.type === 'directory') {
      setCurrentPath(entry.relativePath);
      if (!expandedRef.current.has(entry.relativePath)) await toggle(entry);
      else await refreshDirectory(entry.relativePath);
      return;
    }
    if (entry.type === 'file') setPreviewEntry(entry);
  };

  const navigate = async (pathValue: string) => {
    setCurrentPath(pathValue);
    if (rootRef.current) await refreshDirectory(pathValue);
  };

  const confirmUpload = async (files: LocalUploadFile[], policy: 'refuse' | 'overwrite' | 'update-if-newer') => {
    const currentRoot = rootRef.current;
    if (!currentRoot || !uploadTarget) return;
    const manifest = {
      rootRevision: currentRoot.revision,
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
      reportError(error);
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
        if (status.status === 'completed') await refreshDirectory(targetPath);
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

  const changeAutoRefresh = (seconds: number) => {
    writeAutoRefreshSeconds(seconds);
    setAutoRefreshSeconds(seconds);
  };

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
          <div className="filesystem-panel-heading">
            <span>Files</span>
            <div className="filesystem-panel-actions">
              <button className="icon-button" type="button" onClick={() => setShowHidden((value) => !value)} title={showHidden ? 'Hide hidden files' : 'Show hidden files'} aria-label={showHidden ? 'Hide hidden files' : 'Show hidden files'}>{showHidden ? <EyeOff size={14} aria-hidden="true" /> : <Eye size={14} aria-hidden="true" />}</button>
              <button className="icon-button" type="button" disabled={refreshing} onClick={() => void refreshFileTree()} title="Refresh file tree" aria-label="Refresh file tree">{refreshing ? <LoaderCircle className="spin" size={14} aria-hidden="true" /> : <RefreshCw size={14} aria-hidden="true" />}</button>
              <AutoRefreshMenu value={autoRefreshSeconds} degraded={autoRefreshDegraded} disabled={refreshing} onChange={changeAutoRefresh} />
            </div>
          </div>
          {rootEntry && <FileTree rootEntry={rootEntry} entries={entries} showHidden={showHidden} expanded={expanded} selected={selected} loading={loading} errorPaths={errorPaths} onToggle={(entry) => void toggle(entry)} onSelect={(entry) => setSelected(entry.relativePath)} onOpen={(entry) => void openEntry(entry)} onContextMenu={openMenu} onRootContextMenu={openMenu} />}
        </aside>
        <div className="filesystem-resizer" role="separator" aria-label="Resize file tree" aria-orientation="vertical" aria-valuemin={240} aria-valuemax={420} aria-valuenow={treeWidth} tabIndex={0} onPointerDown={(event) => { resizeStart.current = { x: event.clientX, width: treeWidth }; document.body.style.cursor = 'col-resize'; }} onKeyDown={(event) => { if (event.key === 'ArrowLeft' || event.key === 'ArrowRight') { event.preventDefault(); setTreeWidth((value) => Math.min(420, Math.max(240, value + (event.key === 'ArrowRight' ? 16 : -16)))); } }} />
        <div className="filesystem-preview-panel">
          <FilePreview instanceId={instanceId} root={root} entry={previewEntry} onClose={() => setPreviewEntry(null)} onToast={onToast} onRootChanged={() => void reloadRoot()} />
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
