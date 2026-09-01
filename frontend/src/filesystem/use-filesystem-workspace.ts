import { useCallback, useMemo, useRef, useState, type MouseEvent } from 'react';
import type { FileEntry, LocalUploadFile, UploadStatus } from './filesystem-types';
import { cancelUpload, createUpload, loadUpload } from './filesystem-api';
import { useFilesystemRefresh } from './use-filesystem-refresh';
import { useFilesystemTreeData } from './use-filesystem-tree-data';

type Params = {
  instanceId: string;
  active: boolean;
  onToast: (message: string, kind?: 'info' | 'success' | 'error') => void;
  onOpenFile?: (entry: FileEntry) => void;
};

export type FileSystemContextState = { entry: FileEntry; x: number; y: number } | null;

export function useFilesystemWorkspace({ instanceId, active, onToast, onOpenFile }: Params) {
  const [selected, setSelected] = useState<string | null>(null);
  const [previewEntry, setPreviewEntry] = useState<FileEntry | null>(null);
  const [currentPath, setCurrentPath] = useState('.');
  const [menu, setMenu] = useState<FileSystemContextState>(null);
  const [uploadTarget, setUploadTarget] = useState<FileEntry | null>(null);
  const [uploadStatus, setUploadStatus] = useState<UploadStatus | null>(null);
  const [showHidden, setShowHidden] = useState(true);
  const globalController = useRef<AbortController | null>(null);
  const tree = useFilesystemTreeData({
    instanceId,
    active,
    onToast,
    selected,
    setSelected,
    previewEntry,
    setPreviewEntry,
    setCurrentPath,
    globalController,
  });
  const refresh = useFilesystemRefresh({
    instanceId,
    active,
    onToast,
    rootRef: tree.rootRef,
    expandedRef: tree.expandedRef,
    activeRef: tree.activeRef,
    loadedInstanceId: tree.loadedInstanceId,
    globalController,
    setLoading: tree.setLoading,
    setRootError: tree.setRootError,
    updateRoot: tree.updateRoot,
    resetTree: tree.resetTree,
    reportError: tree.reportError,
    loadRootEntries: tree.loadRootEntries,
    loadDirectory: tree.loadDirectory,
    recoverRoot: tree.recoverRoot,
  });

  const toggle = async (entry: FileEntry) => {
    setSelected(entry.relativePath);
    if (tree.expandedRef.current.has(entry.relativePath)) {
      const next = new Set(tree.expandedRef.current);
      next.delete(entry.relativePath);
      tree.updateExpanded(next);
      return;
    }
    tree.updateExpanded(new Set(tree.expandedRef.current).add(entry.relativePath));
    if (tree.rootRef.current) await tree.loadDirectory(entry.relativePath, tree.rootRef.current.revision);
  };

  const openEntry = async (entry: FileEntry) => {
    setSelected(entry.relativePath);
    if (entry.type === 'directory') {
      setCurrentPath(entry.relativePath);
      if (!tree.expandedRef.current.has(entry.relativePath)) await toggle(entry);
      else await refresh.refreshDirectory(entry.relativePath);
      return;
    }
    if (entry.type === 'file') {
      setPreviewEntry(entry);
      onOpenFile?.(entry);
    }
  };

  const navigate = async (pathValue: string) => {
    setCurrentPath(pathValue);
    await refresh.refreshDirectory(pathValue);
  };

  const pollUpload = async (uploadId: string, targetPath: string): Promise<void> => {
    try {
      const status = await loadUpload(instanceId, uploadId);
      setUploadStatus(status);
      if (status.status === 'completed' || status.status === 'failed' || status.status === 'partial-failure' || status.status === 'cancelled') {
        if (status.status === 'completed') onToast('Upload completed.', 'success');
        else if (status.status !== 'cancelled') onToast('Upload finished with errors.', 'error');
        if (status.status === 'completed') await refresh.refreshDirectory(targetPath);
        return;
      }
      window.setTimeout(() => void pollUpload(uploadId, targetPath), 650);
    } catch (error) {
      onToast(error instanceof Error ? error.message : 'Unable to read upload status.', 'error');
    }
  };

  const confirmUpload = async (files: LocalUploadFile[], policy: 'refuse' | 'overwrite' | 'update-if-newer') => {
    const currentRoot = tree.rootRef.current;
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
      tree.reportError(error);
      onToast(error instanceof Error ? error.message : 'Unable to start upload.', 'error');
    }
  };

  const cancelCurrentUpload = async () => {
    if (!uploadStatus) return;
    try {
      setUploadStatus(await cancelUpload(instanceId, uploadStatus.uploadId));
    } catch (error) {
      onToast(error instanceof Error ? error.message : 'Unable to cancel upload.', 'error');
    }
  };

  const breadcrumb = useMemo(() => currentPath === '.' ? ['.'] : ['.', ...currentPath.split('/')], [currentPath]);
  const rootEntry = useMemo<FileEntry | null>(() => tree.root ? {
    name: tree.root.absolutePath,
    relativePath: '.',
    absolutePath: tree.root.absolutePath,
    type: 'directory',
    size: null,
    modifiedAt: null,
    mode: 0,
    symlink: false,
  } : null, [tree.root]);
  const openMenuAt = useCallback((entry: FileEntry, x: number, y: number) => {
    const viewportWidth = window.visualViewport?.width || window.innerWidth;
    const viewportHeight = window.visualViewport?.height || window.innerHeight;
    setSelected(entry.relativePath);
    setMenu({
      entry,
      x: Math.max(8, Math.min(x, viewportWidth - 258)),
      y: Math.max(8, Math.min(y, viewportHeight - 218)),
    });
  }, []);
  const openMenu = useCallback((event: MouseEvent, entry: FileEntry) => {
    event.preventDefault();
    openMenuAt(entry, event.clientX, event.clientY);
  }, [openMenuAt]);

  return {
    instanceId,
    active,
    ...tree,
    ...refresh,
    selected,
    setSelected,
    previewEntry,
    setPreviewEntry,
    currentPath,
    menu,
    setMenu,
    uploadTarget,
    setUploadTarget,
    uploadStatus,
    setUploadStatus,
    showHidden,
    setShowHidden,
    breadcrumb,
    rootEntry,
    toggle,
    openEntry,
    navigate,
    confirmUpload,
    cancelCurrentUpload,
    openMenu,
    openMenuAt,
  };
}

export type FileSystemWorkspaceState = ReturnType<typeof useFilesystemWorkspace>;
