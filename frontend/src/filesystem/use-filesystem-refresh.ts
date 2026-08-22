import { useCallback, useEffect, useRef, useState, type Dispatch, type MutableRefObject, type SetStateAction } from 'react';
import { loadRoot as loadRootContext } from './filesystem-api';
import { readAutoRefreshSeconds, writeAutoRefreshSeconds } from './auto-refresh-settings';
import { runWithConcurrency } from './filesystem-tree-requests';
import type { FileSystemError, RootContext } from './filesystem-types';

const MAX_REFRESH_CONCURRENCY = 3;

type Params = {
  instanceId: string;
  active: boolean;
  onToast: (message: string, kind?: 'info' | 'success' | 'error') => void;
  rootRef: MutableRefObject<RootContext | null>;
  expandedRef: MutableRefObject<Set<string>>;
  activeRef: MutableRefObject<boolean>;
  loadedInstanceId: MutableRefObject<string>;
  globalController: MutableRefObject<AbortController | null>;
  setLoading: Dispatch<SetStateAction<Set<string>>>;
  setRootError: Dispatch<SetStateAction<FileSystemError | null>>;
  updateRoot: (root: RootContext | null) => void;
  resetTree: (clearRoot: boolean) => void;
  reportError: (reason: unknown, pathValue?: string) => void;
  loadRootEntries: (root: RootContext, signal?: AbortSignal) => Promise<boolean>;
  loadDirectory: (path: string, revision: string, signal?: AbortSignal) => Promise<boolean>;
  recoverRoot: (root: RootContext, signal?: AbortSignal) => Promise<void>;
};

export function useFilesystemRefresh({
  instanceId,
  active,
  onToast,
  rootRef,
  expandedRef,
  activeRef,
  loadedInstanceId,
  globalController,
  setLoading,
  setRootError,
  updateRoot,
  resetTree,
  reportError,
  loadRootEntries,
  loadDirectory,
  recoverRoot,
}: Params) {
  const [refreshing, setRefreshing] = useState(false);
  const [autoRefreshDegraded, setAutoRefreshDegraded] = useState(false);
  const [autoRefreshSeconds, setAutoRefreshSeconds] = useState(readAutoRefreshSeconds);
  const [treeWidth, setTreeWidth] = useState(290);
  const resizeStart = useRef<{ x: number; width: number } | null>(null);
  const globalRefreshInFlight = useRef(false);
  const lastGlobalRefreshAt = useRef<number | null>(null);

  const markAutoRefreshFailure = useCallback(() => {
    if (autoRefreshDegraded) return;
    setAutoRefreshDegraded(true);
    onToast('Automatic FileSystem refresh failed; showing the last successful tree.', 'error');
  }, [autoRefreshDegraded, onToast]);
  const clearAutoRefreshFailure = useCallback(() => setAutoRefreshDegraded(false), []);

  const reloadRoot = useCallback(async (): Promise<boolean> => {
    if (!instanceId) return false;
    globalController.current?.abort();
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
  }, [clearAutoRefreshFailure, globalController, instanceId, loadRootEntries, reportError, resetTree, setRootError, updateRoot]);

  const refreshFileTree = useCallback(async (automatic = false): Promise<boolean> => {
    const currentRoot = rootRef.current;
    if (!instanceId || !currentRoot || globalRefreshInFlight.current) return false;
    globalRefreshInFlight.current = true;
    const controller = new AbortController();
    globalController.current = controller;
    setRefreshing(true);
    const paths = ['.', ...Array.from(expandedRef.current).filter((path) => path !== '.')];
    try {
      const nextRoot = await loadRootContext(instanceId, controller.signal);
      if (nextRoot.revision !== currentRoot.revision) {
        await recoverRoot(nextRoot, controller.signal);
        lastGlobalRefreshAt.current = Date.now();
        clearAutoRefreshFailure();
        return true;
      }
      updateRoot(nextRoot);
      const results = await runWithConcurrency(paths, MAX_REFRESH_CONCURRENCY, (path) => loadDirectory(path, nextRoot.revision, controller.signal));
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
  }, [clearAutoRefreshFailure, expandedRef, globalController, instanceId, loadDirectory, markAutoRefreshFailure, onToast, recoverRoot, rootRef, updateRoot]);

  const refreshDirectory = useCallback(async (pathValue: string) => {
    const currentRoot = rootRef.current;
    if (currentRoot) await loadDirectory(pathValue, currentRoot.revision);
  }, [loadDirectory, rootRef]);

  const changeAutoRefresh = useCallback((seconds: number) => {
    writeAutoRefreshSeconds(seconds);
    setAutoRefreshSeconds(seconds);
  }, []);

  useEffect(() => {
    activeRef.current = active;
    if (active) return;
    globalController.current?.abort();
    globalRefreshInFlight.current = false;
    setRefreshing(false);
    setLoading(new Set());
  }, [active, activeRef, globalController, setLoading]);

  useEffect(() => {
    if (!active || !instanceId || loadedInstanceId.current === instanceId) return;
    loadedInstanceId.current = instanceId;
    void reloadRoot();
  }, [active, instanceId, loadedInstanceId, reloadRoot]);

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
        if (!due) return schedule();
        const succeeded = await performRefresh();
        schedule(succeeded ? undefined : interval);
      }, delayOverride === undefined ? Math.max(250, interval - elapsed) : delayOverride);
    };
    const onVisibilityChange = () => {
      if (document.visibilityState === 'visible') {
        const overdue = lastGlobalRefreshAt.current === null || Date.now() - lastGlobalRefreshAt.current >= interval;
        if (overdue) void performRefresh().then((succeeded) => schedule(succeeded ? undefined : interval));
        else schedule();
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
  }, [active, autoRefreshSeconds, instanceId, markAutoRefreshFailure, reloadRoot, refreshFileTree, rootRef]);

  useEffect(() => {
    const move = (event: PointerEvent) => {
      if (resizeStart.current) setTreeWidth(Math.min(420, Math.max(240, resizeStart.current.width + event.clientX - resizeStart.current.x)));
    };
    const end = () => { resizeStart.current = null; document.body.style.cursor = ''; };
    document.addEventListener('pointermove', move);
    document.addEventListener('pointerup', end);
    return () => {
      document.removeEventListener('pointermove', move);
      document.removeEventListener('pointerup', end);
    };
  }, []);

  return {
    refreshing,
    autoRefreshDegraded,
    autoRefreshSeconds,
    treeWidth,
    resizeStart,
    reloadRoot,
    refreshFileTree,
    refreshDirectory,
    changeAutoRefresh,
    setTreeWidth,
  };
}
