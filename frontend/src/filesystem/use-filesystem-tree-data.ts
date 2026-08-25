import { useCallback, useEffect, useRef, useState, type Dispatch, type MutableRefObject, type SetStateAction } from 'react';
import type { FileEntry, FileSystemError, RootContext } from './filesystem-types';
import { fetchAllEntries } from './filesystem-tree-requests';

type Params = {
  instanceId: string;
  active: boolean;
  onToast: (message: string, kind?: 'info' | 'success' | 'error') => void;
  selected: string | null;
  setSelected: Dispatch<SetStateAction<string | null>>;
  previewEntry: FileEntry | null;
  setPreviewEntry: Dispatch<SetStateAction<FileEntry | null>>;
  setCurrentPath: Dispatch<SetStateAction<string>>;
  globalController: MutableRefObject<AbortController | null>;
};

export function useFilesystemTreeData({
  instanceId,
  active,
  onToast,
  selected,
  setSelected,
  previewEntry,
  setPreviewEntry,
  setCurrentPath,
  globalController,
}: Params) {
  const [root, setRoot] = useState<RootContext | null>(null);
  const [rootError, setRootError] = useState<FileSystemError | null>(null);
  const [entries, setEntries] = useState<Map<string, FileEntry[]>>(new Map());
  const [expanded, setExpanded] = useState<Set<string>>(new Set());
  const [loading, setLoading] = useState<Set<string>>(new Set());
  const [errorPaths, setErrorPaths] = useState<Set<string>>(new Set());
  const rootRef = useRef<RootContext | null>(null);
  const entriesRef = useRef<Map<string, FileEntry[]>>(new Map());
  const expandedRef = useRef<Set<string>>(new Set());
  const requestGeneration = useRef(new Map<string, number>());
  const requestControllers = useRef(new Map<string, AbortController>());
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
    for (const key of nextEntries.keys()) if (matchesPath(key)) nextEntries.delete(key);
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
  }, [previewEntry, selected, setPreviewEntry, setSelected, updateEntries, updateExpanded]);

  const pruneMissingDescendants = useCallback((pathValue: string) => {
    const prefix = pathValue === '.' ? '' : `${pathValue}/`;
    const nextEntries = new Map(entriesRef.current);
    const removed = new Set<string>();
    const candidates = [...new Set([...nextEntries.keys(), ...expandedRef.current, ...requestControllers.current.keys()])]
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
    updateExpanded(new Set([...expandedRef.current].filter((key) => !isRemovedPath(key))));
    setLoading((current) => new Set([...current].filter((key) => !isRemovedPath(key))));
    setErrorPaths((current) => new Set([...current].filter((key) => !isRemovedPath(key))));
    if ((selected && isRemovedPath(selected)) || (previewEntry && isRemovedPath(previewEntry.relativePath))) {
      setSelected(null);
      setPreviewEntry(null);
    }
  }, [previewEntry, selected, setPreviewEntry, setSelected, updateEntries, updateExpanded]);

  const resetTree = useCallback((clearRoot: boolean, preservePreview = false) => {
    const nextEntries = new Map<string, FileEntry[]>();
    const nextExpanded = new Set<string>();
    entriesRef.current = nextEntries;
    expandedRef.current = nextExpanded;
    setEntries(nextEntries);
    setExpanded(nextExpanded);
    setLoading(new Set());
    setErrorPaths(new Set());
    setSelected(null);
    if (!preservePreview) setPreviewEntry(null);
    setCurrentPath('.');
    if (clearRoot && !preservePreview) updateRoot(null);
  }, [setCurrentPath, setPreviewEntry, setSelected, updateRoot]);

  const beginDirectoryRequest = (pathValue: string) => {
    requestControllers.current.get(pathValue)?.abort();
    const generation = (requestGeneration.current.get(pathValue) || 0) + 1;
    requestGeneration.current.set(pathValue, generation);
    const controller = new AbortController();
    requestControllers.current.set(pathValue, controller);
    return { controller, generation };
  };
  const isCurrentRequest = (pathValue: string, generation: number) =>
    activeRef.current && requestGeneration.current.get(pathValue) === generation;

  const reportError = useCallback((reason: unknown, pathValue?: string) => {
    if (reason instanceof DOMException && reason.name === 'AbortError') return;
    const error = (reason instanceof Error ? reason : new Error('FileSystem request failed')) as FileSystemError;
    if (pathValue) {
      setErrorPaths((current) => new Set(current).add(pathValue));
    } else if (!rootRef.current) {
      setRootError(error);
    } else {
      onToast(error.message || 'FileSystem request failed.', 'error');
    }
  }, [onToast]);

  const loadRootEntries = useCallback(async (nextRoot: RootContext, signal?: AbortSignal): Promise<boolean> => {
    const pathValue = '.';
    const request = beginDirectoryRequest(pathValue);
    setLoading((current) => new Set(current).add(pathValue));
    setErrorPaths((current) => new Set([...current].filter((path) => path !== pathValue)));
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
      if (isCurrentRequest(pathValue, request.generation)) setLoading((current) => new Set([...current].filter((path) => path !== pathValue)));
    }
  }, [instanceId, pruneMissingDescendants, reportError, updateEntries]);

  const recoverRoot = useCallback(async (nextRoot: RootContext, signal?: AbortSignal) => {
    updateRoot(nextRoot);
    resetTree(false, true);
    setRootError(null);
    onToast('Root changed. The file tree was refreshed.', 'info');
    await loadRootEntries(nextRoot, signal);
  }, [loadRootEntries, onToast, resetTree, updateRoot]);

  const loadDirectory = useCallback(async (pathValue: string, revision: string, signal?: AbortSignal): Promise<boolean> => {
    const request = beginDirectoryRequest(pathValue);
    setLoading((current) => new Set(current).add(pathValue));
    setErrorPaths((current) => new Set([...current].filter((path) => path !== pathValue)));
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
		if (error.code === 'filesystem_root_changed' && error.root) await recoverRoot(error.root, signal);
      else if (error.code === 'filesystem_not_found') pruneDirectory(pathValue);
      else reportError(error, pathValue);
      return false;
    } finally {
      if (isCurrentRequest(pathValue, request.generation)) setLoading((current) => new Set([...current].filter((path) => path !== pathValue)));
    }
  }, [instanceId, pruneDirectory, pruneMissingDescendants, recoverRoot, reportError, updateEntries]);

  useEffect(() => {
    if (previousInstanceId.current === instanceId) return;
    previousInstanceId.current = instanceId;
    loadedInstanceId.current = '';
    globalController.current?.abort();
    requestControllers.current.forEach((controller) => controller.abort());
    resetTree(true);
    setRootError(null);
  }, [globalController, instanceId, resetTree]);

  useEffect(() => {
    activeRef.current = active;
    if (active) return;
    globalController.current?.abort();
    requestControllers.current.forEach((controller) => controller.abort());
    setLoading(new Set());
    if (!rootRef.current) loadedInstanceId.current = '';
  }, [active, globalController]);

  return {
    root,
    rootError,
    entries,
    expanded,
    loading,
    errorPaths,
    rootRef,
    expandedRef,
    activeRef,
    loadedInstanceId,
    requestControllers,
    globalController,
    setLoading,
    setRootError,
    updateRoot,
    updateExpanded,
    resetTree,
    reportError,
    loadRootEntries,
    loadDirectory,
    recoverRoot,
  };
}
