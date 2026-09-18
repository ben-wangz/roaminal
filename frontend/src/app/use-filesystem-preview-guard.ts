import { useEffect } from 'react';

type FilesystemPreviewGuardOptions = {
  activeInstanceId?: string;
  fileSystemAvailable: boolean;
  filesystemInstanceId: string;
  filesystemInstanceReady: boolean;
  filesystemPreviewEntry: unknown;
  page: string;
  setPreviewEntry: (entry: null) => void;
  setWorkspaceContent: (content: 'terminal') => void;
  workspaceContent: string;
};

export function useFilesystemPreviewGuard({
  activeInstanceId,
  fileSystemAvailable,
  filesystemInstanceId,
  filesystemInstanceReady,
  filesystemPreviewEntry,
  page,
  setPreviewEntry,
  setWorkspaceContent,
  workspaceContent,
}: FilesystemPreviewGuardOptions) {
  useEffect(() => {
    const previewBelongsToActiveInstance = Boolean(
      fileSystemAvailable
      && filesystemInstanceReady
      && filesystemInstanceId
      && filesystemInstanceId === activeInstanceId
      && filesystemPreviewEntry,
    );
    if (!previewBelongsToActiveInstance && workspaceContent === 'file-preview') setWorkspaceContent('terminal');
  }, [activeInstanceId, fileSystemAvailable, filesystemInstanceId, filesystemInstanceReady, filesystemPreviewEntry, setWorkspaceContent, workspaceContent]);

  useEffect(() => {
    if (page !== 'workspace' && filesystemPreviewEntry) setPreviewEntry(null);
  }, [filesystemPreviewEntry, page, setPreviewEntry]);
}
