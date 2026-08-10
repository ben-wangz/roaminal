import { useEffect, useRef, useState } from 'react';
import { PanelLeftOpen, Search, ShieldCheck } from 'lucide-react';
import { api, clearAuth, loadAuth, login, refresh } from '../auth/auth-client';
import { AuthSessionUI, AuthSessionsDialog, type AuthSessionSummary } from '../auth/auth-session-ui';
import { heartbeat, type Heartbeat } from '../status/heartbeat';
import { notify } from '../status/notifications'; import { SystemStatus } from '../status/system-status'; import { Toast } from '../ui/toast'; import { Sidebar } from '../ui/sidebar';
import { TerminalRuntime } from '../terminal/terminal-runtime';
import { TerminalViewport } from '../terminal/terminal-viewport';
import { TerminalSearch } from '../terminal/terminal-search';
import { TouchKeyboard } from '../input/touch-keyboard'; import { observeViewportHeight } from '../input/viewport'; import { matchesShortcut, SHORTCUTS } from '../input/shortcuts';
import { RenameTitleDialog, TerminateDialog } from '../ui/terminal-dialogs';
import { ConnectionManager } from '../connections/connection-manager'; import { startConnectionLaunch } from '../connections/connection-api';
import { loadStoredSession, reconcileSession, saveStoredSession, selectSession as selectStoredSession, type SessionView } from './session-view';
import { useTerminalPreview } from './use-terminal-preview';
import type { ConnectionInstanceSummary } from '../terminal/terminal-protocol';
type Dialog = { type: 'rename' | 'terminate'; sessionId: string } | { type: 'auth' } | null;
export function AppShell() {
  const [auth, setAuth] = useState(loadAuth());
  const [sessions, setSessions] = useState<ConnectionInstanceSummary[]>([]);
  const [view, setView] = useState<SessionView>(() => loadStoredSession(typeof window === 'undefined' ? null : window.localStorage));
  const [workspaceOpen, setWorkspaceOpen] = useState(false);
  const [activeLaunchId, setActiveLaunchId] = useState<string | null>(null);
  const [sidebarOpen, setSidebarOpen] = useState(() => typeof window === 'undefined' || !window.matchMedia('(max-width: 800px)').matches);
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
  const [previewSessionId, setPreviewSessionId] = useState<string | null>(null);
  const { previewRuntimeRef, previewRuntime } = useTerminalPreview(auth, previewSessionId, sidebarOpen);
  const sessionOrder = useRef<string[]>([]);
  const viewRef = useRef(view);
  const hydrated = useRef(false);
  const bootId = useRef<string | null>(null);
  const syncing = useRef(false);
  const stateRevision = useRef(0); const sidebarOpenButton = useRef<HTMLButtonElement>(null);
  const toastTimer = useRef<number | null>(null);
  useEffect(() => observeViewportHeight(), []);
  function showToast(message: string) {
    setToast(message);
    if (toastTimer.current !== null) window.clearTimeout(toastTimer.current);
    toastTimer.current = window.setTimeout(() => { setToast(null); toastTimer.current = null; }, 4500);
  }
  function setActiveView(next: SessionView) { viewRef.current = next; setView(next); }
  function activateSession(id: string) { setActiveView(selectStoredSession(viewRef.current, id)); }
  useEffect(() => { saveStoredSession(window.localStorage, view); }, [view]);
  useEffect(() => { viewRef.current = view; }, [view]);
  useEffect(() => { if (!sidebarOpen) sidebarOpenButton.current?.focus(); }, [sidebarOpen]);
  useEffect(() => () => {
    mainRuntime.current?.dispose();
    mainRuntime.current = null;
    previewRuntimeRef.current?.dispose();
    previewRuntimeRef.current = null;
  }, []);
  useEffect(() => {
    const runtimeId = activeLaunchId || view.activeSessionId;
    if (!auth || !workspaceOpen || !runtimeId) {
      mainRuntime.current?.dispose();
      mainRuntime.current = null;
      setCurrentRuntime(null);
      return;
    }
    const next = new TerminalRuntime(runtimeId, () => auth.accessToken, heartbeatState?.runtime.scrollbackLines || 1000, activeLaunchId ? 'connection-launches' : 'connection-instances');
    const previous = mainRuntime.current;
    mainRuntime.current = next;
    setCurrentRuntime(next);
    previous?.dispose();
    return () => {
      next.dispose();
      if (mainRuntime.current === next) mainRuntime.current = null;
      setCurrentRuntime((current) => current === next ? null : current);
    };
  }, [auth, workspaceOpen, view.activeSessionId, activeLaunchId, heartbeatState?.runtime.scrollbackLines]);
  useEffect(() => {
    const runtimeId = activeLaunchId || view.activeSessionId;
    if (!currentRuntime || currentRuntime.sessionId !== runtimeId) return;
    return currentRuntime.subscribeMessage((message) => {
      if (message?.type === 'launch_published') {
        setActiveLaunchId(null);
        stateRevision.current += 1;
        setSessions((current) => [...current.filter((session) => session.id !== message.instance.id), message.instance]);
        activateSession(message.instance.id);
        setWorkspaceOpen(true);
        return;
      }
      if (message?.type === 'status' && message.status === 'terminated') {
        const exitedID = currentRuntime.sessionId;
        if (activeLaunchId === exitedID) {
          setActiveLaunchId(null);
          setWorkspaceOpen(false);
          showToast('tmux connection could not be started.');
          return;
        }
        setSessions((current) => {
          const next = current.filter((session) => session.id !== exitedID);
          const nextView = reconcileSession(next, viewRef.current, current.map((session) => session.id));
          setView(nextView);
          sessionOrder.current = next.map((session) => session.id);
          if (!nextView.activeSessionId) {
            setWorkspaceOpen(false);
            setSearch(false);
          }
          return next;
        });
        return;
      }
      if (message?.type === 'meta') {
        setSessions((current) => current.map((session) => session.id === currentRuntime.sessionId ? { ...session, title: message.title, titleMode: message.titleMode, cwd: message.cwd, cols: message.cols, rows: message.rows, sourceState: message.sourceState as ConnectionInstanceSummary['sourceState'], generationStatus: message.generationStatus, generationError: message.generationError, generationStaging: message.generationStaging } : session));
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
  }, [currentRuntime, view.activeSessionId, activeLaunchId]);
  async function createConnection(connectionDefinitionId: string, reuseFrom?: string, tmuxEnabled?: boolean) {
    try {
      if (tmuxEnabled) {
        const launch = await startConnectionLaunch(connectionDefinitionId, reuseFrom);
        setActiveLaunchId(launch.launchId);
        setWorkspaceOpen(true);
        return;
      }
      setActiveLaunchId(null);
      const session = await api<ConnectionInstanceSummary>('/api/connection-instances', { method: 'POST', body: JSON.stringify({ connectionDefinitionId, reuseFromConnectionInstanceId: reuseFrom || null }) });
      stateRevision.current += 1; setSessions((current) => [...current.filter((item) => item.id !== session.id), session]);
      activateSession(session.id);
      setWorkspaceOpen(true);
    } catch (err) { showToast((err as Error).message); }
  }
  async function acceptGenerated(instance: ConnectionInstanceSummary) {
    stateRevision.current += 1; setSessions((current) => [...current.filter((item) => item.id !== instance.id), instance]);
    activateSession(instance.id);
    setWorkspaceOpen(true);
  }
  async function sync() {
    if (syncing.current) return;
    syncing.current = true;
    try {
      const revision = stateRevision.current;
      const startedAt = performance.now();
      const next = await heartbeat();
      if (revision !== stateRevision.current) return;
      setHeartbeatLatency(Math.round(performance.now() - startedAt));
      if (bootId.current && bootId.current !== next.runtime.bootId) { window.location.reload(); return; }
      bootId.current = next.runtime.bootId;
      setHeartbeatState(next);
      const nextView = reconcileSession(next.connectionInstances, viewRef.current, sessionOrder.current); setActiveView(nextView);
      if (!hydrated.current && !activeLaunchId) {
        hydrated.current = true;
        setWorkspaceOpen(Boolean(nextView.activeSessionId));
      } else if (!activeLaunchId && !nextView.activeSessionId) {
        setWorkspaceOpen(false);
      }
      sessionOrder.current = next.connectionInstances.map((session) => session.id);
      setSessions(next.connectionInstances);
    } catch (err) {
      if ((err as Error).message === 'unauthorized') {
        const next = await refresh();
        setAuth(next);
      }
    } finally { syncing.current = false; }
  }
  useEffect(() => {
    if (!auth) return;
    void sync();
    const timer = window.setInterval(() => void sync(), 1000);
    return () => window.clearInterval(timer);
  }, [auth, activeLaunchId]);
  useEffect(() => {
    const activeSession = sessions.find((session) => session.id === view.activeSessionId);
    document.title = activeSession ? `Roaminal - ${activeSession.title || activeSession.cwd || 'Connection'}` : 'Roaminal';
  }, [view.activeSessionId, sessions]);
  useEffect(() => {
    const handler = (event: KeyboardEvent) => {
      if (matchesShortcut(event, SHORTCUTS[0])) { event.preventDefault(); setWorkspaceOpen(false); }
      if (matchesShortcut(event, SHORTCUTS[1]) && view.activeSessionId) { event.preventDefault(); setSearch(true); }
      if (matchesShortcut(event, SHORTCUTS[2])) { event.preventDefault(); toggleSidebar(); }
    };
    window.addEventListener('keydown', handler);
    return () => window.removeEventListener('keydown', handler);
  }, [view.activeSessionId]);
  function selectSession(id: string) {
    activateSession(id);
    setWorkspaceOpen(true);
    setSearch(false);
    setPreviewSessionId(null);
    if (window.matchMedia('(max-width: 800px)').matches) setSidebarOpen(false);
  }
  function toggleSidebar() {
    setSidebarOpen((value) => {
      if (value) setPreviewSessionId(null);
      return !value;
    });
  }
  async function updateTitle(id: string, title: string | null) {
    const updated = await api<ConnectionInstanceSummary>(`/api/connection-instances/${id}/title`, { method: 'PATCH', body: JSON.stringify({ title }) });
    setSessions((current) => current.map((session) => session.id === id ? updated : session));
    setDialog(null);
  }
  async function resetTitle(id: string) {
    try { await updateTitle(id, null); } catch (err) { showToast((err as Error).message); }
  }
  async function terminateSession(id: string) {
    try {
      stateRevision.current += 1;
      await api(`/api/connection-instances/${id}`, { method: 'DELETE' });
      setSessions((current) => {
        const next = current.filter((session) => session.id !== id);
        const nextView = reconcileSession(next, viewRef.current, current.map((session) => session.id)); setActiveView(nextView);
        return next;
      });
      setDialog(null);
      setSearch(false);
      setPreviewSessionId(null);
    } catch (err) { showToast((err as Error).message); }
  }
  async function onLogin(password: string) {
    try { const next = await login(password); setAuth(next); setError(''); }
    catch (err) { setError((err as Error).message); }
  }
  function signOut() {
    const current = auth;
    if (!current) return;
    void api('/api/auth/logout', { method: 'POST', body: JSON.stringify({ refreshToken: current.refreshToken }) }, current)
      .catch(() => showToast('Local sign-out completed; server session may remain.'))
      .finally(() => {
        mainRuntime.current?.dispose();
        mainRuntime.current = null;
        previewRuntimeRef.current?.dispose();
        previewRuntimeRef.current = null;
        setPreviewSessionId(null);
        clearAuth();
        setAuth(null);
      });
  }
  async function openAuthSessions() {
    try {
      const [listed, current] = await Promise.all([
        api<{ sessions: AuthSessionSummary[] }>('/api/auth/sessions'),
        api<{ sessionId: string }>('/api/auth/session')
      ]);
      setAuthSessions(listed.sessions);
      setCurrentAuthSessionId(current.sessionId);
      setDialog({ type: 'auth' });
    } catch (err) { showToast((err as Error).message); }
  }
  async function revokeAuthSession(id: string) {
    setAuthSessionBusy(id);
    try {
      await api(`/api/auth/sessions/${id}`, { method: 'DELETE' });
      setAuthSessions((current) => current.filter((session) => session.id !== id));
      if (id === currentAuthSessionId) signOut();
    } catch (err) { showToast((err as Error).message); }
    finally { setAuthSessionBusy(null); }
  }
  async function logoutOtherAuthSessions() {
    setAuthSessionBusy('others');
    try {
      await api('/api/auth/logout-others', { method: 'POST', body: '{}' });
      setAuthSessions((current) => current.filter((session) => session.id === currentAuthSessionId));
    } catch (err) { showToast((err as Error).message); }
    finally { setAuthSessionBusy(null); }
  }
  if (!auth) return <AuthSessionUI error={error} onLogin={onLogin} />;
  const currentSession = sessions.find((session) => session.id === view.activeSessionId);
  const activeRuntime = currentRuntime?.sessionId === view.activeSessionId ? currentRuntime : null;
  const dialogSession = dialog && 'sessionId' in dialog ? sessions.find((session) => session.id === dialog.sessionId) : undefined;
  return <div className="app-shell">
    {workspaceOpen && <Sidebar id="connection-sidebar" sessions={sessions} active={view.activeSessionId} open={sidebarOpen} previewSessionId={previewSessionId} previewRuntime={previewRuntime?.sessionId === previewSessionId ? previewRuntime : null} onToggle={toggleSidebar} onSelect={selectSession} onPreviewStart={(id) => setPreviewSessionId(id)} onPreviewEnd={(id) => setPreviewSessionId((current) => current === id ? null : current)} onUnavailableExtension={(name) => showToast(`${name} extension unavailable`)} onRename={(id) => setDialog({ type: 'rename', sessionId: id })} onAutomaticTitle={resetTitle} onTerminate={(id) => setDialog({ type: 'terminate', sessionId: id })} onCreate={() => setWorkspaceOpen(false)} />}
    <main className={`main-panel ${workspaceOpen && !sidebarOpen ? 'expanded' : ''}`}>
      <header className="topbar">
        {workspaceOpen && !sidebarOpen && <button ref={sidebarOpenButton} className="icon-button sidebar-open-button" type="button" onClick={() => setSidebarOpen(true)} aria-label="Open sidebar" title="Open sidebar" aria-expanded={false} aria-controls="connection-sidebar"><PanelLeftOpen aria-hidden="true" size={18} /></button>}
        <SystemStatus connected={Boolean(heartbeatState)} system={heartbeatState?.system || null} sessionCount={sessions.length} latencyMs={heartbeatLatency} persistenceDegraded={Boolean(heartbeatState?.runtime.persistenceDegraded)} />
        <div className="top-actions">{workspaceOpen && <><button className="icon-button" onClick={() => setSearch((value) => !value)} aria-label="Search terminal" title="Search terminal"><Search aria-hidden="true" size={17} /></button><button className="text-button" onClick={() => setWorkspaceOpen(false)}>Connections</button></>}<button className="text-button" onClick={() => void openAuthSessions()}><ShieldCheck aria-hidden="true" size={15} /> Sessions</button><button className="text-button" onClick={signOut}>Sign out</button></div>
      </header>
      {workspaceOpen ? <><>{search && activeRuntime && <TerminalSearch runtime={activeRuntime} onClose={() => setSearch(false)} />}</><section className="terminal-stage">{activeRuntime ? <TerminalViewport key={activeRuntime.sessionId} runtime={activeRuntime} /> : <div className="empty-state"><div className="brand-mark">r<span>&gt;</span></div><button className="primary" onClick={() => setWorkspaceOpen(false)}>Open connection manager</button></div>}</section>{activeRuntime && <TouchKeyboard onInput={(value) => activeRuntime.send({ type: 'input', data: value })} />}<footer className="statusbar"><span>{currentSession?.cwd || 'No connection'}</span><span className="execution-status" aria-live="polite">{executionStatus || (currentSession ? `${currentSession.cols}x${currentSession.rows}` : '')}</span></footer></> : <ConnectionManager instances={sessions} onConnect={createConnection} onGenerated={acceptGenerated} onOpenWorkspace={() => { if (view.activeSessionId) setWorkspaceOpen(true); }} onToast={showToast} />}
    </main>
    <Toast message={toast} />
    {dialog?.type === 'rename' && dialogSession && <RenameTitleDialog session={dialogSession} onSave={(title) => updateTitle(dialogSession.id, title)} onClose={() => setDialog(null)} />}
    {dialog?.type === 'terminate' && dialogSession && <TerminateDialog session={dialogSession} onConfirm={() => terminateSession(dialogSession.id)} onClose={() => setDialog(null)} />}
    {dialog?.type === 'auth' && <AuthSessionsDialog sessions={authSessions} currentId={currentAuthSessionId} busy={authSessionBusy} onRevoke={(id) => void revokeAuthSession(id)} onLogoutOthers={() => void logoutOtherAuthSessions()} onClose={() => setDialog(null)} />}
  </div>;
}
