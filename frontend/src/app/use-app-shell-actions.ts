import { type Dispatch, type MutableRefObject, type SetStateAction } from 'react';
import type { AuthState } from '../auth/auth-storage';
import type { ConnectionView } from './connection-view';
import { useHeartbeat } from './use-heartbeat';
import type { TerminalRuntime } from '../terminal/terminal-runtime';
import type { ConnectionInstanceSummary } from '../terminal/terminal-protocol';
import { useAuthSessionActions } from './use-auth-session-actions';
import type { AppPage } from './app-state';
import type { ToastKind } from '../ui/toast';
import { useConnectionInstanceLayoutActions } from './use-connection-instance-layout-actions';
import { useConnectionLifecycleActions } from './use-connection-lifecycle-actions';
import { ConnectionInstanceController } from '../connections/connection-instance-controller';
import { useConnectionInstanceActions } from './use-connection-instance-actions';
import { useAppShellChromeActions } from './use-app-shell-chrome-actions';
import type { WorkspaceTool } from './workspace-tool';
import type { WorkspaceContent } from './workspace-content';
import type { Dialog } from './app-shell-overlays';

type DisposableRuntimeRef = MutableRefObject<{ dispose(): void } | null>;

type Params = {
  auth: AuthState | null;
  setAuth: Dispatch<SetStateAction<AuthState | null>>;
  setError: Dispatch<SetStateAction<string>>;
  activeLaunchId: string | null;
  startLaunch: (id: string) => void;
  clearLaunch: () => void;
  cancelLaunch: () => void;
  mainRuntime: MutableRefObject<TerminalRuntime | null>;
  previewRuntimeRef: DisposableRuntimeRef;
  viewActiveConnectionInstanceId: string | null;
  page: AppPage;
  viewRef: MutableRefObject<ConnectionView>;
  setActiveView: (next: ConnectionView) => void;
  connections: ConnectionInstanceSummary[];
  controller: ConnectionInstanceController;
  setCurrentRuntime: Dispatch<SetStateAction<TerminalRuntime | null>>;
  setPage: Dispatch<SetStateAction<AppPage>>;
  workspaceTool: WorkspaceTool;
  setWorkspaceTool: Dispatch<SetStateAction<WorkspaceTool>>;
  setWorkspaceToolOpen: Dispatch<SetStateAction<boolean>>;
  setWorkspaceContent: Dispatch<SetStateAction<WorkspaceContent>>;
  setSearch: Dispatch<SetStateAction<boolean>>;
  setPreviewConnectionInstanceId: Dispatch<SetStateAction<string | null>>;
  setDialog: Dispatch<SetStateAction<Dialog>>;
  showToast: (message: string, kind?: ToastKind) => void;
};

export function useAppShellActions({
  auth,
  setAuth,
  setError,
  activeLaunchId,
  startLaunch,
  clearLaunch,
  cancelLaunch,
  mainRuntime,
  previewRuntimeRef,
  viewActiveConnectionInstanceId,
  page,
  viewRef,
  setActiveView,
  connections,
  controller,
  setCurrentRuntime,
  setPage,
  workspaceTool,
  setWorkspaceTool,
  setWorkspaceToolOpen,
  setWorkspaceContent,
  setSearch,
  setPreviewConnectionInstanceId,
  setDialog,
  showToast,
}: Params) {
  const pauseHeartbeat = useHeartbeat({
    auth,
    setAuth,
    activeLaunchId,
    page,
    setPage,
    viewRef,
    setActiveView,
    controller,
  });

  const authActions = useAuthSessionActions({
    auth,
    setAuth,
    cancelLaunch,
    mainRuntime,
    previewRuntimeRef,
    setPreviewConnectionInstanceId,
    pauseHeartbeat,
    setDialog,
    showToast,
  });

  const layoutActions = useConnectionInstanceLayoutActions({
    controller,
    showToast,
  });
  const lifecycleActions = useConnectionLifecycleActions({
    setAuth,
    setError,
    setCurrentRuntime,
    setActiveView,
    setDialog,
    setPreviewConnectionInstanceId,
    setSearch,
    setWorkspaceContent,
    mainRuntime,
    previewRuntimeRef,
    controller,
    viewRef,
    showToast,
  });

  const connectionInstanceActions = useConnectionInstanceActions({
    activeLaunchId,
    startLaunch,
    clearLaunch,
    activeConnectionInstanceId: viewActiveConnectionInstanceId,
    viewRef,
    setActiveView,
    connections,
    controller,
    setCurrentRuntime,
    setPage,
    workspaceTool,
    setWorkspaceToolOpen,
    setWorkspaceContent,
    setSearch,
    setPreviewConnectionInstanceId,
    showToast,
  });
  const chromeActions = useAppShellChromeActions({
    workspaceTool,
    setWorkspaceTool,
    setWorkspaceToolOpen,
    setPreviewConnectionInstanceId,
  });

  return {
    ...authActions,
		...connectionInstanceActions,
		...layoutActions,
		...chromeActions,
    ...lifecycleActions,
  };
}
