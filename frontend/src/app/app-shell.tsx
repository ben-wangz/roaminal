import { useEffect, useRef, useState } from 'react';
import { PanelLeftOpen, Search, ShieldCheck } from 'lucide-react';
import { api, clearAuth, loadAuth, login, refresh } from '../auth/auth-client';
import { AuthSessionUI, AuthSessionsDialog, type AuthSessionSummary } from '../auth/auth-session-ui';
import { heartbeat, type Heartbeat } from '../status/heartbeat';
import { notify } from '../status/notifications';
import { SystemStatus } from '../status/system-status';
import { Toast } from '../ui/toast';
import { Sidebar } from '../ui/sidebar';
import { TerminalRuntime } from '../terminal/terminal-runtime';
import { TerminalPreviewRuntime } from '../terminal/terminal-preview';
import { TerminalViewport } from '../terminal/terminal-viewport';
import { TerminalSearch } from '../terminal/terminal-search';
import { TouchKeyboard } from '../input/touch-keyboard';
import { observeViewportHeight } from '../input/viewport';
import { matchesShortcut, SHORTCUTS } from '../input/shortcuts';
import { RenameTitleDialog, TerminateDialog } from '../ui/terminal-dialogs';
import { loadStoredSession, reconcileSession, saveStoredSession, selectSession as selectStoredSession, type SessionView } from './session-view';
import type { SessionSummary } from '../terminal/terminal-protocol';

type Dialog = { type: 'rename' | 'terminate'; sessionId: string } | { type: 'auth' } | null;

export function AppShell() {
  const [auth, setAuth] = useState(loadAuth());
  const [sessions, setSessions] = useState<SessionSummary[]>([]);
  const [view, setView] = useState<SessionView>(() => loadStoredSession(typeof window === 'undefined' ? null : window.localStorage));
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
  const previewRuntimeRef = useRef<TerminalPreviewRuntime | null>(null);
  const [previewSessionId, setPreviewSessionId] = useState<string | null>(null);
  const [previewRuntime, setPreviewRuntime] = useState<TerminalPreviewRuntime | null>(null);
  const previewGeneration = useRef(0);
  const sessionOrder = useRef<string[]>([]);
  const bootId = useRef<string | null>(null);
  const syncing = useRef(false);
  const creatingInitial = useRef(false);
  const sidebarOpenButton = useRef<HTMLButtonElement>(null);
  const toastTimer = useRef<number | null>(null);

  useEffect(() => observeViewportHeight(), []);

  function showToast(message: string) {
    setToast(message);
    if (toastTimer.current !== null) window.clearTimeout(toastTimer.current);
    toastTimer.current = window.setTimeout(() => { setToast(null); toastTimer.current = null; }, 4500);
  }

  useEffect(() => { saveStoredSession(window.localStorage, view); }, [view]);
  useEffect(() => { if (!sidebarOpen) sidebarOpenButton.current?.focus(); }, [sidebarOpen]);
  useEffect(() => () => {
    mainRuntime.current?.dispose();
    mainRuntime.current = null;
    previewRuntimeRef.current?.dispose();
    previewRuntimeRef.current = null;
  }, []);

  useEffect(() => {
    if (!auth || !view.activeSessionId) {
      mainRuntime.current?.dispose();
      mainRuntime.current = null;
      setCurrentRuntime(null);
      return;
    }

    const next = new TerminalRuntime(view.activeSessionId, () => auth.accessToken, heartbeatState?.runtime.scrollbackLines || 1000);
    const previous = mainRuntime.current;
    mainRuntime.current = next;
    setCurrentRuntime(next);
    previous?.dispose();
    return () => {
      next.dispose();
      if (mainRuntime.current === next) mainRuntime.current = null;
      setCurrentRuntime((current) => current === next ? null : current);
    };
  }, [auth, view.activeSessionId, heartbeatState?.runtime.scrollbackLines]);

  useEffect(() => {
    if (!currentRuntime || currentRuntime.sessionId !== view.activeSessionId) return;
    return currentRuntime.subscribeMessage((message) => {
      if (message?.type === 'status' && message.status === 'terminated') {
        setSessions((current) => current.map((session) => session.id === currentRuntime.sessionId ? { ...session, closed: true, exitStatus: message.exitStatus || session.exitStatus || null } : session));
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
  }, [currentRuntime, view.activeSessionId]);

  useEffect(() => {
    const generation = ++previewGeneration.current;
    previewRuntimeRef.current?.dispose();
    previewRuntimeRef.current = null;
    setPreviewRuntime(null);
    if (!auth || !previewSessionId || !sidebarOpen) return;
    const timer = window.setTimeout(() => {
      if (generation !== previewGeneration.current) return;
      const next = new TerminalPreviewRuntime(previewSessionId, () => auth.accessToken);
      previewRuntimeRef.current = next;
      setPreviewRuntime(next);
    }, 100);
    return () => {
      window.clearTimeout(timer);
      if (previewGeneration.current !== generation) return;
      previewRuntimeRef.current?.dispose();
      previewRuntimeRef.current = null;
      setPreviewRuntime(null);
    };
  }, [auth, previewSessionId, sidebarOpen]);

  async function createSession() {
    try {
      const session = await api<SessionSummary>('/api/sessions', { method: 'POST', body: '{}' });
      setSessions((current) => [...current.filter((item) => item.id !== session.id), session]);
      setView((current) => selectStoredSession(current, session.id));
    } catch (err) { showToast((err as Error).message); }
  }

  async function sync() {
    if (syncing.current) return;
    syncing.current = true;
    try {
      const startedAt = performance.now();
      const next = await heartbeat();
      setHeartbeatLatency(Math.round(performance.now() - startedAt));
      if (bootId.current && bootId.current !== next.runtime.bootId) { window.location.reload(); return; }
      bootId.current = next.runtime.bootId;
      setHeartbeatState(next);
      setView((current) => reconcileSession(next.sessions, current, sessionOrder.current));
      sessionOrder.current = next.sessions.map((session) => session.id);
      setSessions(next.sessions);
      if (!next.sessions.length && !creatingInitial.current) {
        creatingInitial.current = true;
        await createSession();
        creatingInitial.current = false;
      }
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
  }, [auth]);

  useEffect(() => {
    const activeSession = sessions.find((session) => session.id === view.activeSessionId);
    document.title = activeSession ? `Roaminal · ${activeSession.title || activeSession.cwd || 'Terminal'}` : 'Roaminal';
  }, [view.activeSessionId, sessions]);

  useEffect(() => {
    const handler = (event: KeyboardEvent) => {
      if (matchesShortcut(event, SHORTCUTS[0])) { event.preventDefault(); void createSession(); }
      if (matchesShortcut(event, SHORTCUTS[1]) && view.activeSessionId) { event.preventDefault(); setSearch(true); }
      if (matchesShortcut(event, SHORTCUTS[2])) { event.preventDefault(); toggleSidebar(); }
    };
    window.addEventListener('keydown', handler);
    return () => window.removeEventListener('keydown', handler);
  }, [view.activeSessionId]);

  function selectSession(id: string) {
    setView((current) => selectStoredSession(current, id));
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
    const updated = await api<SessionSummary>(`/api/sessions/${id}/title`, { method: 'PATCH', body: JSON.stringify({ title }) });
    setSessions((current) => current.map((session) => session.id === id ? updated : session));
    setDialog(null);
  }

  async function resetTitle(id: string) {
    try { await updateTitle(id, null); } catch (err) { showToast((err as Error).message); }
  }

  async function terminateSession(id: string) {
    try {
      await api(`/api/sessions/${id}`, { method: 'DELETE' });
      setSessions((current) => {
        const next = current.filter((session) => session.id !== id);
        setView((viewState) => reconcileSession(next, viewState, current.map((session) => session.id)));
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
        setPreviewRuntime(null);
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
    <Sidebar id="terminal-sidebar" sessions={sessions} active={view.activeSessionId} open={sidebarOpen} previewSessionId={previewSessionId} previewRuntime={previewRuntime?.sessionId === previewSessionId ? previewRuntime : null} onToggle={toggleSidebar} onSelect={selectSession} onPreviewStart={(id) => setPreviewSessionId(id)} onPreviewEnd={(id) => setPreviewSessionId((current) => current === id ? null : current)} onUnavailableExtension={(name) => showToast(`${name} extension unavailable`)} onRename={(id) => setDialog({ type: 'rename', sessionId: id })} onAutomaticTitle={resetTitle} onTerminate={(id) => setDialog({ type: 'terminate', sessionId: id })} onCreate={createSession} />
    <main className={`main-panel ${sidebarOpen ? '' : 'expanded'}`}>
      <header className="topbar">
        {!sidebarOpen && <button ref={sidebarOpenButton} className="icon-button sidebar-open-button" type="button" onClick={() => setSidebarOpen(true)} aria-label="Open sidebar" title="Open sidebar" aria-expanded={false} aria-controls="terminal-sidebar"><PanelLeftOpen aria-hidden="true" size={18} /></button>}
        <SystemStatus connected={Boolean(heartbeatState)} system={heartbeatState?.system || null} sessionCount={sessions.length} latencyMs={heartbeatLatency} persistenceDegraded={Boolean(heartbeatState?.runtime.persistenceDegraded)} />
        <div className="top-actions"><button className="icon-button" onClick={() => setSearch((value) => !value)} aria-label="Search terminal" title="Search terminal"><Search aria-hidden="true" size={17} /></button><button className="text-button" onClick={() => void openAuthSessions()}><ShieldCheck aria-hidden="true" size={15} /> Sessions</button><button className="text-button" onClick={signOut}>Sign out</button></div>
      </header>
      {search && activeRuntime && <TerminalSearch runtime={activeRuntime} onClose={() => setSearch(false)} />}
      <section className="terminal-stage">{activeRuntime ? <><TerminalViewport key={activeRuntime.sessionId} runtime={activeRuntime} />{currentSession?.closed && <div className="terminal-exited" role="status"><strong>Terminal exited</strong><span>{formatExitStatus(currentSession.exitStatus)}</span><div className="terminal-exited-actions"><button className="primary" onClick={() => void createSession()}>Create terminal</button><button className="danger-button" onClick={() => void terminateSession(currentSession.id)}>Delete history</button></div></div>}</> : <div className="empty-state"><div className="brand-mark">r<span>&gt;</span></div><button className="primary" onClick={createSession}>Create terminal</button></div>}</section>
      {activeRuntime && !currentSession?.closed && <TouchKeyboard onInput={(value) => activeRuntime.send({ type: 'input', data: value })} />}
      <footer className="statusbar"><span>{currentSession?.cwd || 'No terminal'}</span><span className="execution-status" aria-live="polite">{executionStatus || (currentSession ? `${currentSession.cols}×${currentSession.rows}` : '')}</span></footer>
    </main>
    <Toast message={toast} />
    {dialog?.type === 'rename' && dialogSession && <RenameTitleDialog session={dialogSession} onSave={(title) => updateTitle(dialogSession.id, title)} onClose={() => setDialog(null)} />}
    {dialog?.type === 'terminate' && dialogSession && <TerminateDialog session={dialogSession} onConfirm={() => terminateSession(dialogSession.id)} onClose={() => setDialog(null)} />}
    {dialog?.type === 'auth' && <AuthSessionsDialog sessions={authSessions} currentId={currentAuthSessionId} busy={authSessionBusy} onRevoke={(id) => void revokeAuthSession(id)} onLogoutOthers={() => void logoutOtherAuthSessions()} onClose={() => setDialog(null)} />}
  </div>;
}

function formatExitStatus(status: SessionSummary['exitStatus']): string {
  if (!status) return 'The shell ended normally.';
  if (status.signal !== null) return `Signal ${status.signal}`;
  return `Exit code ${status.exitCode ?? 0}`;
}
