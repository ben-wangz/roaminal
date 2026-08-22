import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { loadAuth } from '../auth/auth-client';
import { AuthSessionUI } from '../auth/auth-session-ui';
import { type Heartbeat } from '../status/heartbeat';
import { TerminalRuntime } from '../terminal/terminal-runtime';
import { observeViewportHeight, SIDEBAR_BREAKPOINT_QUERY } from '../input/viewport';
import { defaultContextualMode, type ContextualMode } from '../input/contextual-keyboard-model';
import {
  loadStoredConnection,
  saveStoredConnection,
  selectConnection,
  type ConnectionView,
} from './connection-view';
import { useTerminalPreview } from './use-terminal-preview';
import { usePendingLaunch } from './use-pending-launch';
import { AppShellView, type Dialog } from './app-shell-view';
import { useAppShellActions } from './use-app-shell-actions';
import type { ConnectionInstanceSummary } from '../terminal/terminal-protocol';
import { normalizeConnectionInstanceLayout, type ConnectionInstanceLayout } from '../connections/connection-instance-groups';
import { browserAppearanceStorage, loadAppearance, saveAppearance, type TerminalAppearance } from '../appearance/appearance-model';
import { useAppearanceStorage } from '../appearance/use-appearance-storage';
import type { AppPage } from './app-state';
import { useMainTerminalRuntime } from './use-main-terminal-runtime';
import { useRuntimeMessages } from './use-runtime-messages';
import type { ToastKind, ToastState } from '../ui/toast';
import { useWorkspaceMode } from './use-workspace-mode';

export function AppShell() {
  const [auth, setAuth] = useState(loadAuth());
  const [connections, setConnections] = useState<ConnectionInstanceSummary[]>([]);
  const [connectionInstanceLayout, setConnectionInstanceLayout] = useState<ConnectionInstanceLayout | null>(null);
  const [view, setView] = useState<ConnectionView>(() => loadStoredConnection(typeof window === 'undefined' ? null : window.localStorage));
  const [page, setPage] = useState<AppPage>('connections');
  const [appearance, setAppearance] = useState<TerminalAppearance>(() => loadAppearance(browserAppearanceStorage()));
  const [sidebarOpen, setSidebarOpen] = useState(
    () => typeof window === 'undefined' || !window.matchMedia(SIDEBAR_BREAKPOINT_QUERY).matches,
  );
  const [heartbeatState, setHeartbeatState] = useState<Heartbeat | null>(null);
  const [heartbeatLatency, setHeartbeatLatency] = useState<number | null>(null);
  const [heartbeatConnected, setHeartbeatConnected] = useState(true);
  const [error, setError] = useState('');
  const [toast, setToast] = useState<ToastState | null>(null);
  const [executionStatus, setExecutionStatus] = useState<string | null>(null);
  const [search, setSearch] = useState(false);
  const [dialog, setDialog] = useState<Dialog>(null);
  const mainRuntime = useRef<TerminalRuntime | null>(null);
  const [currentRuntime, setCurrentRuntime] = useState<TerminalRuntime | null>(null);
  const [previewConnectionInstanceId, setPreviewConnectionInstanceId] = useState<string | null>(null);
  const { previewRuntimeRef, previewRuntime } = useTerminalPreview(auth, previewConnectionInstanceId, sidebarOpen, appearance);
  const connectionOrder = useRef<string[]>([]);
  const connectionInstanceLayoutRef = useRef<ConnectionInstanceLayout | null>(null);
  const pendingConnectionInstanceLayout = useRef<ConnectionInstanceLayout | null>(null);
  const pendingConnectionOrder = useRef<string[] | null>(null);
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
  const showToast = useCallback((message: string, kind: ToastKind = 'info') => {
    setToast({ message, kind });
    if (toastTimer.current !== null) window.clearTimeout(toastTimer.current);
    toastTimer.current = window.setTimeout(() => {
      setToast(null);
      toastTimer.current = null;
    }, 4500);
  }, []);
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
    page,
    viewRef,
    setActiveView,
    connections,
    setConnections,
    setConnectionInstanceLayout,
    connectionInstanceLayoutRef,
    pendingConnectionInstanceLayout,
    setCurrentRuntime,
    setPage,
    setSidebarOpen,
    setSearch,
    setPreviewConnectionInstanceId,
    setHeartbeatLatency,
    setHeartbeatConnected,
    setHeartbeatState,
    stateRevision,
    connectionOrder,
    pendingConnectionOrder,
    hydrated,
    bootId,
    syncing,
    setDialog,
    showToast,
  });
  const { workspaceMode, onOpenFileSystem, onOpenTerminal } = useWorkspaceMode({
    view,
    viewRef,
    selectConnection: actions.selectConnectionInstance,
    setPage,
  });
  useEffect(() => {
    saveStoredConnection(window.localStorage, view);
  }, [view]);
  useEffect(() => {
    viewRef.current = view;
  }, [view]);
  useEffect(() => {
    connectionInstanceLayoutRef.current = connectionInstanceLayout;
  }, [connectionInstanceLayout]);
  useEffect(() => {
    if (auth) return;
    pendingConnectionInstanceLayout.current = null;
    connectionInstanceLayoutRef.current = null;
    setConnectionInstanceLayout(null);
  }, [auth]);
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
  useMainTerminalRuntime({
    auth,
    page,
    runtimeId: activeLaunchId || view.activeConnectionInstanceId,
    scrollbackLines: heartbeatState?.runtime.scrollbackLines || 1000,
    endpoint: activeLaunchId ? 'connection-launches' : 'connection-instances',
    appearance,
    mainRuntime,
    currentRuntime,
    setCurrentRuntime,
  });
  useAppearanceStorage(setAppearance);
  useRuntimeMessages({
    currentRuntime,
    activeLaunchId,
    viewActiveConnectionInstanceId: view.activeConnectionInstanceId,
    viewRef,
    connectionOrder,
    stateRevision,
    activateConnection,
    clearLaunch,
    setConnections,
    setCurrentRuntime,
    setView,
    setPage,
    setSearch,
    setExecutionStatus,
    showToast,
  });
  const currentConnection = connections.find(
    (connection) => connection.connectionInstanceId === view.activeConnectionInstanceId,
  );
  const activeRuntimeId = activeLaunchId || view.activeConnectionInstanceId;
  const activeInstance =
    connections.find((connection) => connection.connectionInstanceId === view.activeConnectionInstanceId) || null;
  const sidebarLayout = useMemo(() => normalizeConnectionInstanceLayout(connectionInstanceLayout, connections), [connectionInstanceLayout, connections]);
  const contextualMode = activeInstance
    ? contextualModes.current.get(activeInstance.connectionInstanceId) || defaultContextualMode(activeInstance)
    : 'codex';
  const setContextualMode = useCallback(
    (mode: ContextualMode) => {
      if (!activeInstance) return;
      contextualModes.current.set(activeInstance.connectionInstanceId, mode);
      setConnections((current) => [...current]);
    },
    [activeInstance],
  );
  const handlePreviewStart = useCallback((id: string) => setPreviewConnectionInstanceId(id), []);
  const handlePreviewEnd = useCallback(
    (id: string) => setPreviewConnectionInstanceId((current) => (current === id ? null : current)),
    [],
  );
  const handleAgent = useCallback((id: string) => {
    if (workspaceMode === 'filesystem') {
      onOpenTerminal(id);
      return;
    }
    setDialog({ type: 'agent', connectionInstanceId: id });
  }, [onOpenTerminal, setDialog, workspaceMode]);
  const handleOpenFileSystem = useCallback((id: string) => {
    setPreviewConnectionInstanceId(null);
    onOpenFileSystem(id);
  }, [onOpenFileSystem]);
  const handleRename = useCallback((id: string) => setDialog({ type: 'rename', connectionInstanceId: id }), []);
  const handleTerminate = useCallback((id: string) => setDialog({ type: 'terminate', connectionInstanceId: id }), []);
  const handleOpenSidebar = useCallback(() => setSidebarOpen(true), []);
  const handleToggleSearch = useCallback(() => setSearch((value) => !value), []);
  const handleCloseSearch = useCallback(() => setSearch(false), []);
  const handleOpenConnections = useCallback(() => {
    cancelLaunch();
    setPreviewConnectionInstanceId(null);
    setSearch(false);
    setPage('connections');
  }, [cancelLaunch, setSearch]);
  const handleOpenAppearance = useCallback(() => {
    setPreviewConnectionInstanceId(null);
    setSearch(false);
    setPage('appearance');
  }, [setSearch]);
  const handleOpenWorkspace = useCallback(() => {
    if (viewRef.current.activeConnectionInstanceId) setPage('workspace');
  }, []);
  const handleSaveAppearance = useCallback(
    (next: TerminalAppearance) => {
      if (!saveAppearance(browserAppearanceStorage(), next)) {
        showToast('Unable to save appearance in this browser.', 'error');
        return;
      }
      setAppearance(next);
      showToast('Appearance saved.', 'success');
    },
    [showToast],
  );
  const handleCloseDialog = useCallback(() => setDialog(null), []);
  if (!auth) return <AuthSessionUI error={error} onLogin={actions.onLogin} />;
  const dialogConnection =
    dialog && 'connectionInstanceId' in dialog
      ? connections.find((connection) => connection.connectionInstanceId === dialog.connectionInstanceId)
      : undefined;
  return (
    <AppShellView
      page={page}
      appearance={appearance}
      sidebarOpen={sidebarOpen}
      sidebarOpenButton={sidebarOpenButton}
      connections={connections}
      connectionInstanceLayout={sidebarLayout}
      loginSessionId={actions.currentAuthSessionId}
      view={view}
      heartbeatState={heartbeatState}
      heartbeatLatency={heartbeatLatency}
      heartbeatConnected={heartbeatConnected}
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
      onOpenSidebar={handleOpenSidebar}
      onSelectConnection={actions.selectConnectionInstance}
      onMoveConnectionInstance={actions.moveConnectionInstanceToGroup}
      onReorderConnectionGroup={actions.reorderConnectionInstanceGroup}
      onCreateConnectionGroup={actions.createConnectionInstanceGroup}
      onRenameConnectionGroup={actions.renameConnectionInstanceGroup}
      onDeleteConnectionGroup={actions.deleteConnectionInstanceGroup}
      onMoveConnectionGroupMembers={actions.moveGroupMembersToUngrouped}
      onPreviewStart={handlePreviewStart}
      onPreviewEnd={handlePreviewEnd}
      onAgent={handleAgent}
      onOpenFileSystem={handleOpenFileSystem}
      onRename={handleRename}
      onAutomaticTitle={actions.resetTitle}
      onTerminate={handleTerminate}
      onContextualModeChange={setContextualMode}
      onToggleSearch={handleToggleSearch}
      onCloseSearch={handleCloseSearch}
      onOpenConnections={handleOpenConnections}
      onOpenAppearance={handleOpenAppearance}
      onSignOut={actions.signOut}
      onOpenAuthSessions={() => void actions.openAuthSessions()}
      onOpenManager={handleOpenConnections}
      onCreateConnection={actions.createConnection}
      onGenerated={actions.acceptGenerated}
      onOpenWorkspace={handleOpenWorkspace}
      onSaveAppearance={handleSaveAppearance}
      onShowToast={showToast}
      onRenameTitle={actions.updateTitle}
      onTerminateConnection={actions.terminateConnection}
      onRevokeAuthSession={(id) => void actions.revokeAuthSession(id)}
      onLogoutOtherAuthSessions={() => void actions.logoutOtherAuthSessions()}
      onCloseDialog={handleCloseDialog}
      workspaceMode={workspaceMode}
    />
  );
}
