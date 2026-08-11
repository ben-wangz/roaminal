import { useCallback, useEffect, useRef, useState } from 'react';
import { currentAccessToken, loadAuth } from '../auth/auth-client';
import { AuthSessionUI } from '../auth/auth-session-ui';
import { type Heartbeat } from '../status/heartbeat';
import { notify } from '../status/notifications';
import { TerminalRuntime } from '../terminal/terminal-runtime';
import { observeViewportHeight } from '../input/viewport';
import { defaultContextualMode, type ContextualMode } from '../input/contextual-keyboard-model';
import {
  loadStoredConnection,
  reconcileConnections,
  saveStoredConnection,
  selectConnection,
  type ConnectionView,
} from './connection-view';
import { useTerminalPreview } from './use-terminal-preview';
import { usePendingLaunch } from './use-pending-launch';
import { AppShellView, type Dialog } from './app-shell-view';
import { useAppShellActions } from './use-app-shell-actions';
import type { ConnectionInstanceSummary } from '../terminal/terminal-protocol';
export function AppShell() {
  const [auth, setAuth] = useState(loadAuth());
  const [connections, setConnections] = useState<ConnectionInstanceSummary[]>([]);
  const [view, setView] = useState<ConnectionView>(() =>
    loadStoredConnection(typeof window === 'undefined' ? null : window.localStorage),
  );
  const [workspaceOpen, setWorkspaceOpen] = useState(false);
  const [sidebarOpen, setSidebarOpen] = useState(
    () => typeof window === 'undefined' || !window.matchMedia('(max-width: 800px)').matches,
  );
  const [heartbeatState, setHeartbeatState] = useState<Heartbeat | null>(null);
  const [heartbeatLatency, setHeartbeatLatency] = useState<number | null>(null);
  const [error, setError] = useState('');
  const [toast, setToast] = useState<string | null>(null);
  const [executionStatus, setExecutionStatus] = useState<string | null>(null);
  const [search, setSearch] = useState(false);
  const [dialog, setDialog] = useState<Dialog>(null);
  const mainRuntime = useRef<TerminalRuntime | null>(null);
  const [currentRuntime, setCurrentRuntime] = useState<TerminalRuntime | null>(null);
  const [previewConnectionInstanceId, setPreviewConnectionInstanceId] = useState<string | null>(null);
  const { previewRuntimeRef, previewRuntime } = useTerminalPreview(auth, previewConnectionInstanceId, sidebarOpen);
  const connectionOrder = useRef<string[]>([]);
  const { activeLaunchId, startLaunch, clearLaunch, cancelLaunch } = usePendingLaunch(
    auth,
    mainRuntime,
    previewRuntimeRef,
  );
  const viewRef = useRef(view);
  const hydrated = useRef(false);
  const bootId = useRef<string | null>(null);
  const syncing = useRef(false);
  const stateRevision = useRef(0);
  const sidebarOpenButton = useRef<HTMLButtonElement>(null);
  const toastTimer = useRef<number | null>(null);
  const contextualModes = useRef(new Map<string, ContextualMode>());
  useEffect(() => observeViewportHeight(), []);
  function showToast(message: string) {
    setToast(message);
    if (toastTimer.current !== null) window.clearTimeout(toastTimer.current);
    toastTimer.current = window.setTimeout(() => {
      setToast(null);
      toastTimer.current = null;
    }, 4500);
  }
  const setActiveView = useCallback((next: ConnectionView) => {
    viewRef.current = next;
    setView(next);
  }, []);
  const activateConnection = useCallback(
    (id: string) => {
      setActiveView(selectConnection(viewRef.current, id));
    },
    [setActiveView],
  );
  const actions = useAppShellActions({
    auth,
    setAuth,
    setError,
    activeLaunchId,
    startLaunch,
    clearLaunch,
    cancelLaunch,
    mainRuntime,
    previewRuntimeRef,
    viewActiveConnectionInstanceId: view.activeConnectionInstanceId,
    viewRef,
    setActiveView,
    connections,
    setConnections,
    setCurrentRuntime,
    setWorkspaceOpen,
    setSidebarOpen,
    setSearch,
    setPreviewConnectionInstanceId,
    setHeartbeatLatency,
    setHeartbeatState,
    stateRevision,
    connectionOrder,
    hydrated,
    bootId,
    syncing,
    setDialog,
    showToast,
  });
  useEffect(() => {
    saveStoredConnection(window.localStorage, view);
  }, [view]);
  useEffect(() => {
    viewRef.current = view;
  }, [view]);
  useEffect(() => {
    if (!sidebarOpen) sidebarOpenButton.current?.focus();
  }, [sidebarOpen]);
  useEffect(
    () => () => {
      mainRuntime.current?.dispose();
      mainRuntime.current = null;
      previewRuntimeRef.current?.dispose();
      previewRuntimeRef.current = null;
    },
    [previewRuntimeRef],
  );
  useEffect(() => {
    const runtimeId = activeLaunchId || view.activeConnectionInstanceId;
    if (!auth || !workspaceOpen || !runtimeId) {
      mainRuntime.current?.dispose();
      mainRuntime.current = null;
      setCurrentRuntime(null);
      return;
    }
    const next = new TerminalRuntime(
      runtimeId,
      currentAccessToken,
      heartbeatState?.runtime.scrollbackLines || 1000,
      activeLaunchId ? 'connection-launches' : 'connection-instances',
    );
    const previous = mainRuntime.current;
    mainRuntime.current = next;
    setCurrentRuntime(next);
    previous?.dispose();
    return () => {
      next.dispose();
      if (mainRuntime.current === next) mainRuntime.current = null;
      setCurrentRuntime((current) => (current === next ? null : current));
    };
  }, [auth, workspaceOpen, view.activeConnectionInstanceId, activeLaunchId, heartbeatState?.runtime.scrollbackLines]);
  useEffect(() => {
    const runtimeId = activeLaunchId || view.activeConnectionInstanceId;
    if (!currentRuntime || currentRuntime.connectionInstanceId !== runtimeId) return;
    return currentRuntime.subscribeMessage((message) => {
      if (message?.type === 'launch_published') {
        setCurrentRuntime((current) => (current === currentRuntime ? null : current));
        clearLaunch();
        stateRevision.current += 1;
        setConnections((current) => [
          ...current.filter((connection) => connection.connectionInstanceId !== message.instance.connectionInstanceId),
          message.instance,
        ]);
        activateConnection(message.instance.connectionInstanceId);
        setWorkspaceOpen(true);
        return;
      }
      if (message?.type === 'status' && message.status === 'terminated') {
        const exitedID = currentRuntime.connectionInstanceId;
        if (activeLaunchId === exitedID) {
          setCurrentRuntime((current) => (current === currentRuntime ? null : current));
          clearLaunch();
          setWorkspaceOpen(false);
          showToast('tmux connection could not be started.');
          return;
        }
        setConnections((current) => {
          const next = current.filter((connection) => connection.connectionInstanceId !== exitedID);
          const nextView = reconcileConnections(
            next,
            viewRef.current,
            current.map((connection) => connection.connectionInstanceId),
          );
          setView(nextView);
          connectionOrder.current = next.map((connection) => connection.connectionInstanceId);
          if (!nextView.activeConnectionInstanceId) {
            setWorkspaceOpen(false);
            setSearch(false);
          }
          return next;
        });
        return;
      }
      if (message?.type === 'meta') {
        setConnections((current) =>
          current.map((connection) =>
            connection.connectionInstanceId === currentRuntime.connectionInstanceId
              ? {
                  ...connection,
                  title: message.title,
                  titleMode: message.titleMode,
                  cwd: message.cwd,
                  cols: message.cols,
                  rows: message.rows,
                  sourceState: message.sourceState as ConnectionInstanceSummary['sourceState'],
                  generationStatus: message.generationStatus,
                  generationError: message.generationError,
                }
              : connection,
          ),
        );
        return;
      }
      if (!message || message.type !== 'execution') return;
      if (message.phase === 'started') {
        setExecutionStatus(message.command ? `Running: ${message.command}` : 'Running command');
      } else if (message.phase === 'completed') {
        setExecutionStatus(null);
        showToast('Command completed');
        notify('Roaminal', 'Command completed');
      }
    });
  }, [activateConnection, clearLaunch, currentRuntime, view.activeConnectionInstanceId, activeLaunchId]);
  if (!auth) return <AuthSessionUI error={error} onLogin={actions.onLogin} />;
  const currentConnection = connections.find(
    (connection) => connection.connectionInstanceId === view.activeConnectionInstanceId,
  );
  const activeRuntimeId = activeLaunchId || view.activeConnectionInstanceId;
  const activeInstance =
    connections.find((connection) => connection.connectionInstanceId === view.activeConnectionInstanceId) || null;
  const contextualMode = activeInstance
    ? contextualModes.current.get(activeInstance.connectionInstanceId) || defaultContextualMode(activeInstance)
    : 'codex';
  function setContextualMode(mode: ContextualMode) {
    if (!activeInstance) return;
    contextualModes.current.set(activeInstance.connectionInstanceId, mode);
    setConnections((current) => [...current]);
  }
  const dialogConnection =
    dialog && 'connectionInstanceId' in dialog
      ? connections.find((connection) => connection.connectionInstanceId === dialog.connectionInstanceId)
      : undefined;
  return (
    <AppShellView
      workspaceOpen={workspaceOpen}
      sidebarOpen={sidebarOpen}
      sidebarOpenButton={sidebarOpenButton}
      connections={connections}
      view={view}
      heartbeatState={heartbeatState}
      heartbeatLatency={heartbeatLatency}
      currentConnection={currentConnection}
      activeInstance={activeInstance}
      currentRuntime={currentRuntime}
      activeRuntimeId={activeRuntimeId}
      previewConnectionInstanceId={previewConnectionInstanceId}
      previewRuntime={previewRuntime}
      contextualMode={contextualMode}
      search={search}
      executionStatus={executionStatus}
      toast={toast}
      dialog={dialog}
      dialogConnection={dialogConnection}
      authSessions={actions.authSessions}
      currentAuthSessionId={actions.currentAuthSessionId}
      authSessionBusy={actions.authSessionBusy}
      onToggleSidebar={actions.toggleSidebar}
      onOpenSidebar={() => setSidebarOpen(true)}
      onSelectConnection={actions.selectConnectionInstance}
      onPreviewStart={(id) => setPreviewConnectionInstanceId(id)}
      onPreviewEnd={(id) => setPreviewConnectionInstanceId((current) => (current === id ? null : current))}
      onUnavailableExtension={(name) => showToast(`${name} extension unavailable`)}
      onRename={(id) => setDialog({ type: 'rename', connectionInstanceId: id })}
      onAutomaticTitle={actions.resetTitle}
      onTerminate={(id) => setDialog({ type: 'terminate', connectionInstanceId: id })}
      onContextualModeChange={setContextualMode}
      onToggleSearch={() => setSearch((value) => !value)}
      onCloseSearch={() => setSearch(false)}
      onOpenConnections={() => {
        cancelLaunch();
        setWorkspaceOpen(false);
      }}
      onSignOut={actions.signOut}
      onOpenAuthSessions={() => void actions.openAuthSessions()}
      onOpenManager={() => {
        cancelLaunch();
        setWorkspaceOpen(false);
      }}
      onCreateConnection={actions.createConnection}
      onGenerated={actions.acceptGenerated}
      onOpenWorkspace={() => {
        if (view.activeConnectionInstanceId) setWorkspaceOpen(true);
      }}
      onShowToast={showToast}
      onRenameTitle={actions.updateTitle}
      onTerminateConnection={actions.terminateConnection}
      onRevokeAuthSession={(id) => void actions.revokeAuthSession(id)}
      onLogoutOtherAuthSessions={() => void actions.logoutOtherAuthSessions()}
      onCloseDialog={() => setDialog(null)}
    />
  );
}
