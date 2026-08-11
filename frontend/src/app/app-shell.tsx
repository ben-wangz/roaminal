import { useCallback, useEffect, useRef, useState } from 'react';
import { PanelLeftOpen, Search, ShieldCheck } from 'lucide-react';
import { api, clearAuth, currentAccessToken, loadAuth, login, refresh } from '../auth/auth-client';
import { AuthSessionUI, AuthSessionsDialog, type AuthSessionSummary } from '../auth/auth-session-ui';
import { heartbeat, type Heartbeat } from '../status/heartbeat';
import { RemoteMonitorBand } from '../status/remote-monitor-band';
import { notify } from '../status/notifications';
import { connectionDisplayName } from '../status/connection-label';
import { SystemStatus } from '../status/system-status';
import { Toast } from '../ui/toast';
import { ConnectionSidebar } from '../ui/connection-sidebar';
import { TerminalRuntime } from '../terminal/terminal-runtime';
import { TerminalViewport } from '../terminal/terminal-viewport';
import { TerminalSearch } from '../terminal/terminal-search';
import { TouchKeyboard } from '../input/touch-keyboard';
import { observeViewportHeight } from '../input/viewport';
import { matchesShortcut, SHORTCUTS } from '../input/shortcuts';
import { defaultContextualMode, type ContextualMode } from '../input/contextual-keyboard-model';
import { RenameTitleDialog, CloseConnectionDialog } from '../ui/connection-dialogs';
import { ConnectionManager } from '../connections/connection-manager';
import { startConnectionLaunch } from '../connections/connection-api';
import {
  loadStoredConnection,
  reconcileConnections,
  saveStoredConnection,
  selectConnection,
  type ConnectionView,
} from './connection-view';
import { useTerminalPreview } from './use-terminal-preview';
import { usePendingLaunch } from './use-pending-launch';
import type { ConnectionInstanceSummary } from '../terminal/terminal-protocol';
type Dialog = { type: 'rename' | 'terminate'; connectionInstanceId: string } | { type: 'auth' } | null;
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
  const [authSessions, setAuthSessions] = useState<AuthSessionSummary[]>([]);
  const [currentAuthSessionId, setCurrentAuthSessionId] = useState('');
  const [authSessionBusy, setAuthSessionBusy] = useState<string | null>(null);
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
  const activateConnection = useCallback((id: string) => {
    setActiveView(selectConnection(viewRef.current, id));
  }, [setActiveView]);
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
  async function createConnection(connectionDefinitionId: string, reuseFrom?: string, tmuxEnabled?: boolean) {
    try {
      if (tmuxEnabled) {
        const launch = await startConnectionLaunch(connectionDefinitionId, reuseFrom);
        setCurrentRuntime(null);
        startLaunch(launch.launchId);
        setWorkspaceOpen(true);
        return;
      }
      clearLaunch();
      const session = await api<ConnectionInstanceSummary>('/api/connection-instances', {
        method: 'POST',
        body: JSON.stringify({ connectionDefinitionId, reuseFromConnectionInstanceId: reuseFrom || null }),
      });
      stateRevision.current += 1;
      setConnections((current) => [
        ...current.filter((item) => item.connectionInstanceId !== session.connectionInstanceId),
        session,
      ]);
      activateConnection(session.connectionInstanceId);
      setWorkspaceOpen(true);
    } catch (err) {
      showToast((err as Error).message);
    }
  }
  async function acceptGenerated(instance: ConnectionInstanceSummary) {
    stateRevision.current += 1;
    setConnections((current) => [
      ...current.filter((item) => item.connectionInstanceId !== instance.connectionInstanceId),
      instance,
    ]);
    activateConnection(instance.connectionInstanceId);
    setWorkspaceOpen(true);
  }
  const sync = useCallback(async () => {
    if (syncing.current) return;
    syncing.current = true;
    try {
      const revision = stateRevision.current;
      const startedAt = performance.now();
      const next = await heartbeat();
      if (revision !== stateRevision.current) return;
      setHeartbeatLatency(Math.round(performance.now() - startedAt));
      if (bootId.current && bootId.current !== next.runtime.bootId) {
        window.location.reload();
        return;
      }
      bootId.current = next.runtime.bootId;
      setHeartbeatState(next);
      const nextView = reconcileConnections(next.connectionInstances, viewRef.current, connectionOrder.current);
      setActiveView(nextView);
      if (!hydrated.current && !activeLaunchId) {
        hydrated.current = true;
        setWorkspaceOpen(Boolean(nextView.activeConnectionInstanceId));
      } else if (!activeLaunchId && !nextView.activeConnectionInstanceId) {
        setWorkspaceOpen(false);
      }
      connectionOrder.current = next.connectionInstances.map((connection) => connection.connectionInstanceId);
      setConnections(next.connectionInstances);
    } catch (err) {
      if ((err as Error).message === 'unauthorized') {
        const next = await refresh();
        setAuth(next);
      }
    } finally {
      syncing.current = false;
    }
  }, [activeLaunchId, setActiveView]);
  useEffect(() => {
    if (!auth) return;
    void sync();
    const timer = window.setInterval(() => void sync(), 1000);
    return () => window.clearInterval(timer);
  }, [auth, activeLaunchId, sync]);
  useEffect(() => {
    const activeConnection = connections.find(
      (connection) => connection.connectionInstanceId === view.activeConnectionInstanceId,
    );
    document.title = activeConnection
      ? `Roaminal - ${activeConnection.title || activeConnection.cwd || 'Connection'}`
      : 'Roaminal';
  }, [view.activeConnectionInstanceId, connections]);
  useEffect(() => {
    const handler = (event: KeyboardEvent) => {
      if (matchesShortcut(event, SHORTCUTS[0])) {
        event.preventDefault();
        setWorkspaceOpen(false);
      }
      if (matchesShortcut(event, SHORTCUTS[1]) && view.activeConnectionInstanceId) {
        event.preventDefault();
        setSearch(true);
      }
      if (matchesShortcut(event, SHORTCUTS[2])) {
        event.preventDefault();
        toggleSidebar();
      }
    };
    window.addEventListener('keydown', handler);
    return () => window.removeEventListener('keydown', handler);
  }, [view.activeConnectionInstanceId]);
  function selectConnectionInstance(id: string) {
    if (viewRef.current.activeConnectionInstanceId !== id || activeLaunchId) setCurrentRuntime(null);
    activateConnection(id);
    setWorkspaceOpen(true);
    setSearch(false);
    setPreviewConnectionInstanceId(null);
    if (window.matchMedia('(max-width: 800px)').matches) setSidebarOpen(false);
  }
  function toggleSidebar() {
    setSidebarOpen((value) => {
      if (value) setPreviewConnectionInstanceId(null);
      return !value;
    });
  }
  async function updateTitle(id: string, title: string | null) {
    const updated = await api<ConnectionInstanceSummary>(`/api/connection-instances/${id}/title`, {
      method: 'PATCH',
      body: JSON.stringify({ title }),
    });
    setConnections((current) =>
      current.map((connection) => (connection.connectionInstanceId === id ? updated : connection)),
    );
    setDialog(null);
  }
  async function resetTitle(id: string) {
    try {
      await updateTitle(id, null);
    } catch (err) {
      showToast((err as Error).message);
    }
  }
  async function terminateConnection(id: string) {
    try {
      stateRevision.current += 1;
      await api(`/api/connection-instances/${id}`, { method: 'DELETE' });
      setConnections((current) => {
        const next = current.filter((connection) => connection.connectionInstanceId !== id);
        const nextView = reconcileConnections(
          next,
          viewRef.current,
          current.map((connection) => connection.connectionInstanceId),
        );
        setActiveView(nextView);
        return next;
      });
      setDialog(null);
      setSearch(false);
      setPreviewConnectionInstanceId(null);
    } catch (err) {
      showToast((err as Error).message);
    }
  }
  async function onLogin(password: string) {
    try {
      const next = await login(password);
      setAuth(next);
      setError('');
    } catch (err) {
      setError((err as Error).message);
    }
  }
  function signOut() {
    const current = auth;
    if (!current) return;
    cancelLaunch();
    void api(
      '/api/auth/logout',
      { method: 'POST', body: JSON.stringify({ refreshToken: current.refreshToken }) },
      current,
    )
      .catch(() => showToast('Local sign-out completed; server session may remain.'))
      .finally(() => {
        mainRuntime.current?.dispose();
        mainRuntime.current = null;
        previewRuntimeRef.current?.dispose();
        previewRuntimeRef.current = null;
        setPreviewConnectionInstanceId(null);
        clearAuth();
        setAuth(null);
      });
  }
  async function openAuthSessions() {
    try {
      const [listed, current] = await Promise.all([
        api<{ sessions: AuthSessionSummary[] }>('/api/auth/sessions'),
        api<{ sessionId: string }>('/api/auth/session'),
      ]);
      setAuthSessions(listed.sessions);
      setCurrentAuthSessionId(current.sessionId);
      setDialog({ type: 'auth' });
    } catch (err) {
      showToast((err as Error).message);
    }
  }
  async function revokeAuthSession(id: string) {
    setAuthSessionBusy(id);
    try {
      await api(`/api/auth/sessions/${id}`, { method: 'DELETE' });
      setAuthSessions((current) => current.filter((session) => session.id !== id));
      if (id === currentAuthSessionId) signOut();
    } catch (err) {
      showToast((err as Error).message);
    } finally {
      setAuthSessionBusy(null);
    }
  }
  async function logoutOtherAuthSessions() {
    setAuthSessionBusy('others');
    try {
      await api('/api/auth/logout-others', { method: 'POST', body: '{}' });
      setAuthSessions((current) => current.filter((session) => session.id === currentAuthSessionId));
    } catch (err) {
      showToast((err as Error).message);
    } finally {
      setAuthSessionBusy(null);
    }
  }
  if (!auth) return <AuthSessionUI error={error} onLogin={onLogin} />;
  const currentConnection = connections.find(
    (connection) => connection.connectionInstanceId === view.activeConnectionInstanceId,
  );
  const activeRuntimeId = activeLaunchId || view.activeConnectionInstanceId;
  const activeRuntime = currentRuntime?.connectionInstanceId === activeRuntimeId ? currentRuntime : null;
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
    <div className="app-shell">
      {workspaceOpen && (
        <ConnectionSidebar
          id="connection-sidebar"
          connections={connections}
          active={view.activeConnectionInstanceId}
          open={sidebarOpen}
          previewConnectionInstanceId={previewConnectionInstanceId}
          previewRuntime={previewRuntime?.connectionInstanceId === previewConnectionInstanceId ? previewRuntime : null}
          onToggle={toggleSidebar}
          onSelect={selectConnectionInstance}
          onPreviewStart={(id) => setPreviewConnectionInstanceId(id)}
          onPreviewEnd={(id) => setPreviewConnectionInstanceId((current) => (current === id ? null : current))}
          onUnavailableExtension={(name) => showToast(`${name} extension unavailable`)}
          onRename={(id) => setDialog({ type: 'rename', connectionInstanceId: id })}
          onAutomaticTitle={resetTitle}
          onTerminate={(id) => setDialog({ type: 'terminate', connectionInstanceId: id })}
          activeInstance={activeInstance}
          activeRuntime={activeRuntime}
          contextualMode={contextualMode}
          onContextualModeChange={setContextualMode}
        />
      )}
      <main className={`main-panel ${workspaceOpen && !sidebarOpen ? 'expanded' : ''}`}>
        <header className="topbar">
          {workspaceOpen && !sidebarOpen && (
            <button
              ref={sidebarOpenButton}
              className="icon-button sidebar-open-button"
              type="button"
              onClick={() => setSidebarOpen(true)}
              aria-label="Open sidebar"
              title="Open sidebar"
              aria-expanded={false}
              aria-controls="connection-sidebar"
            >
              <PanelLeftOpen aria-hidden="true" size={18} />
            </button>
          )}
          <SystemStatus
            connected={Boolean(heartbeatState)}
            connectionName={connectionDisplayName(currentConnection || null, connections)}
            system={heartbeatState?.system || null}
            connectionCount={connections.length}
            latencyMs={heartbeatLatency}
            persistenceDegraded={Boolean(heartbeatState?.runtime.persistenceDegraded)}
          />
          <div className="top-actions">
            {workspaceOpen && (
              <>
                <button
                  className="icon-button"
                  onClick={() => setSearch((value) => !value)}
                  aria-label="Search terminal"
                  title="Search terminal"
                >
                  <Search aria-hidden="true" size={17} />
                </button>
                <button
                  className="text-button"
                  onClick={() => {
                    cancelLaunch();
                    setWorkspaceOpen(false);
                  }}
                >
                  Connections
                </button>
              </>
            )}
            <button className="text-button" onClick={() => void openAuthSessions()}>
              <ShieldCheck aria-hidden="true" size={15} /> Sessions
            </button>
            <button className="text-button" onClick={signOut}>
              Sign out
            </button>
          </div>
        </header>
        {workspaceOpen && <RemoteMonitorBand instance={activeInstance} />}
        {workspaceOpen ? (
          <>
            <>
              {search && activeRuntime && <TerminalSearch runtime={activeRuntime} onClose={() => setSearch(false)} />}
            </>
            <section className="terminal-stage">
              {activeRuntime ? (
                <TerminalViewport key={activeRuntime.connectionInstanceId} runtime={activeRuntime} />
              ) : (
                <div className="empty-state">
                  <div className="brand-mark">
                    r<span>&gt;</span>
                  </div>
                  <button
                    className="primary"
                    onClick={() => {
                      cancelLaunch();
                      setWorkspaceOpen(false);
                    }}
                  >
                    Open connection manager
                  </button>
                </div>
              )}
            </section>
            {activeRuntime && <TouchKeyboard onInput={(value) => activeRuntime.input(value)} />}
            <footer className="statusbar">
              <span>{currentConnection?.cwd || 'No connection'}</span>
              <span className="execution-status" aria-live="polite">
                {executionStatus || (currentConnection ? `${currentConnection.cols}x${currentConnection.rows}` : '')}
              </span>
            </footer>
          </>
        ) : (
          <ConnectionManager
            connections={connections}
            onConnect={createConnection}
            onGenerated={acceptGenerated}
            onOpenWorkspace={() => {
              if (view.activeConnectionInstanceId) setWorkspaceOpen(true);
            }}
            onToast={showToast}
          />
        )}
      </main>
      <Toast message={toast} />
      {dialog?.type === 'rename' && dialogConnection && (
        <RenameTitleDialog
          connection={dialogConnection}
          onSave={(title) => updateTitle(dialogConnection.connectionInstanceId, title)}
          onClose={() => setDialog(null)}
        />
      )}
      {dialog?.type === 'terminate' && dialogConnection && (
        <CloseConnectionDialog
          connection={dialogConnection}
          onConfirm={() => terminateConnection(dialogConnection.connectionInstanceId)}
          onClose={() => setDialog(null)}
        />
      )}
      {dialog?.type === 'auth' && (
        <AuthSessionsDialog
          sessions={authSessions}
          currentId={currentAuthSessionId}
          busy={authSessionBusy}
          onRevoke={(id) => void revokeAuthSession(id)}
          onLogoutOthers={() => void logoutOtherAuthSessions()}
          onClose={() => setDialog(null)}
        />
      )}
    </div>
  );
}
