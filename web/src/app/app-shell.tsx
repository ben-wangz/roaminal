import { useEffect, useRef, useState } from 'react';
import { PanelLeftOpen, Search } from 'lucide-react';
import { api, clearAuth, loadAuth, login, refresh } from '../auth/auth-client';
import { AuthSessionUI } from '../auth/auth-session-ui';
import { heartbeat, type Heartbeat } from '../status/heartbeat';
import { SystemStatus } from '../status/system-status';
import { Toast } from '../ui/toast';
import { Sidebar } from '../ui/sidebar';
import { TerminalRuntime } from '../terminal/terminal-runtime';
import { TerminalPreviewRuntime } from '../terminal/terminal-preview';
import { TerminalViewport } from '../terminal/terminal-viewport';
import { TerminalSearch } from '../terminal/terminal-search';
import { TouchKeyboard } from '../input/touch-keyboard';
import { RenameTitleDialog, TerminateDialog } from '../ui/terminal-dialogs';
import { loadStoredSession, reconcileSession, saveStoredSession, selectSession as selectStoredSession, type SessionView } from './session-view';
import type { SessionSummary } from '../terminal/terminal-protocol';

type Dialog = { type: 'rename' | 'terminate'; sessionId: string } | null;

export function AppShell() {
  const [auth, setAuth] = useState(loadAuth());
  const [sessions, setSessions] = useState<SessionSummary[]>([]);
  const [view, setView] = useState<SessionView>(() => loadStoredSession(typeof window === 'undefined' ? null : window.localStorage));
  const [sidebarOpen, setSidebarOpen] = useState(() => typeof window === 'undefined' || !window.matchMedia('(max-width: 800px)').matches);
  const [heartbeatState, setHeartbeatState] = useState<Heartbeat | null>(null);
  const [error, setError] = useState('');
  const [toast, setToast] = useState<string | null>(null);
  const [search, setSearch] = useState(false);
  const [dialog, setDialog] = useState<Dialog>(null);
  const mainRuntime = useRef<TerminalRuntime | null>(null);
  const [currentRuntime, setCurrentRuntime] = useState<TerminalRuntime | null>(null);
  const previewRuntimeRef = useRef<TerminalPreviewRuntime | null>(null);
  const [previewSessionId, setPreviewSessionId] = useState<string | null>(null);
  const [previewRuntime, setPreviewRuntime] = useState<TerminalPreviewRuntime | null>(null);
  const sessionOrder = useRef<string[]>([]);
  const bootId = useRef<string | null>(null);
  const syncing = useRef(false);
  const creatingInitial = useRef(false);
  const sidebarOpenButton = useRef<HTMLButtonElement>(null);

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

    const next = new TerminalRuntime(view.activeSessionId, () => auth.accessToken);
    const previous = mainRuntime.current;
    mainRuntime.current = next;
    setCurrentRuntime(next);
    previous?.dispose();
    return () => {
      next.dispose();
      if (mainRuntime.current === next) mainRuntime.current = null;
      setCurrentRuntime((current) => current === next ? null : current);
    };
  }, [auth, view.activeSessionId]);

  useEffect(() => {
    previewRuntimeRef.current?.dispose();
    previewRuntimeRef.current = null;
    setPreviewRuntime(null);
    if (!auth || !previewSessionId || !sidebarOpen) return;
    const next = new TerminalPreviewRuntime(previewSessionId, () => auth.accessToken);
    previewRuntimeRef.current = next;
    setPreviewRuntime(next);
    return () => {
      next.dispose();
      if (previewRuntimeRef.current === next) previewRuntimeRef.current = null;
      setPreviewRuntime((current) => current === next ? null : current);
    };
  }, [auth, previewSessionId, sidebarOpen]);

  async function createSession() {
    try {
      const session = await api<SessionSummary>('/api/sessions', { method: 'POST', body: '{}' });
      setSessions((current) => [...current.filter((item) => item.id !== session.id), session]);
      setView((current) => selectStoredSession(current, session.id));
    } catch (err) { setToast((err as Error).message); }
  }

  async function sync() {
    if (syncing.current) return;
    syncing.current = true;
    try {
      const next = await heartbeat();
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
      if ((event.ctrlKey || event.metaKey) && event.shiftKey && event.key.toLowerCase() === 't') { event.preventDefault(); void createSession(); }
      if ((event.ctrlKey || event.metaKey) && event.key.toLowerCase() === 'f' && view.activeSessionId) { event.preventDefault(); setSearch(true); }
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
    try { await updateTitle(id, null); } catch (err) { setToast((err as Error).message); }
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
    } catch (err) { setToast((err as Error).message); }
  }

  async function onLogin(password: string) {
    try { const next = await login(password); setAuth(next); setError(''); }
    catch (err) { setError((err as Error).message); }
  }

  function signOut() {
    const current = auth;
    if (!current) return;
    void api('/api/auth/logout', { method: 'POST', body: JSON.stringify({ refreshToken: current.refreshToken }) }, current)
      .catch(() => setToast('Local sign-out completed; server session may remain.'))
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

  if (!auth) return <AuthSessionUI error={error} onLogin={onLogin} />;
  const currentSession = sessions.find((session) => session.id === view.activeSessionId);
  const activeRuntime = currentRuntime?.sessionId === view.activeSessionId ? currentRuntime : null;
  const dialogSession = dialog ? sessions.find((session) => session.id === dialog.sessionId) : undefined;

  return <div className="app-shell">
    <Sidebar id="terminal-sidebar" sessions={sessions} active={view.activeSessionId} open={sidebarOpen} previewSessionId={previewSessionId} previewRuntime={previewRuntime?.sessionId === previewSessionId ? previewRuntime : null} onToggle={toggleSidebar} onSelect={selectSession} onPreviewStart={(id) => setPreviewSessionId(id)} onPreviewEnd={(id) => setPreviewSessionId((current) => current === id ? null : current)} onUnavailableExtension={(name) => setToast(`${name} extension unavailable`)} onRename={(id) => setDialog({ type: 'rename', sessionId: id })} onAutomaticTitle={resetTitle} onTerminate={(id) => setDialog({ type: 'terminate', sessionId: id })} onCreate={createSession} />
    <main className={`main-panel ${sidebarOpen ? '' : 'expanded'}`}>
      <header className="topbar">
        {!sidebarOpen && <button ref={sidebarOpenButton} className="icon-button sidebar-open-button" type="button" onClick={() => setSidebarOpen(true)} aria-label="Open sidebar" title="Open sidebar" aria-expanded={false} aria-controls="terminal-sidebar"><PanelLeftOpen aria-hidden="true" size={18} /></button>}
        <SystemStatus connected={Boolean(heartbeatState)} hostname={heartbeatState?.system.hostname || ''} sessionCount={sessions.length} />
        <div className="top-actions"><button className="icon-button" onClick={() => setSearch((value) => !value)} aria-label="Search terminal" title="Search terminal"><Search aria-hidden="true" size={17} /></button><button className="text-button" onClick={signOut}>Sign out</button></div>
      </header>
      {search && activeRuntime && <TerminalSearch runtime={activeRuntime} onClose={() => setSearch(false)} />}
      <section className="terminal-stage">{activeRuntime ? <TerminalViewport key={activeRuntime.sessionId} runtime={activeRuntime} /> : <div className="empty-state"><div className="brand-mark">r<span>&gt;</span></div><button className="primary" onClick={createSession}>Create terminal</button></div>}</section>
      {activeRuntime && <TouchKeyboard onInput={(value) => activeRuntime.send({ type: 'input', data: value })} />}
      <footer className="statusbar"><span>{currentSession?.cwd || 'No terminal'}</span><span>{currentSession ? `${currentSession.cols}×${currentSession.rows}` : ''}</span></footer>
    </main>
    <Toast message={toast} />
    {dialog?.type === 'rename' && dialogSession && <RenameTitleDialog session={dialogSession} onSave={(title) => updateTitle(dialogSession.id, title)} onClose={() => setDialog(null)} />}
    {dialog?.type === 'terminate' && dialogSession && <TerminateDialog session={dialogSession} onConfirm={() => terminateSession(dialogSession.id)} onClose={() => setDialog(null)} />}
  </div>;
}
