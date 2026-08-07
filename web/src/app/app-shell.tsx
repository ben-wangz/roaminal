import { useEffect, useRef, useState } from 'react';
import { api, clearAuth, loadAuth, login, refresh } from '../auth/auth-client';
import { AuthSessionUI } from '../auth/auth-session-ui';
import { heartbeat, type Heartbeat } from '../status/heartbeat';
import { SystemStatus } from '../status/system-status';
import { Toast } from '../ui/toast';
import { Sidebar } from '../ui/sidebar';
import { TerminalTabs } from '../terminal/terminal-tabs';
import { TerminalRuntime } from '../terminal/terminal-runtime';
import { TerminalViewport } from '../terminal/terminal-viewport';
import { TerminalSearch } from '../terminal/terminal-search';
import { TouchKeyboard } from '../input/touch-keyboard';
import { RenameTitleDialog, TerminateDialog } from '../ui/terminal-dialogs';
import { closeTab, loadStoredTabs, openTab, reconcileTabs, saveStoredTabs, type TabView } from './session-view';
import type { SessionSummary } from '../terminal/terminal-protocol';

type Dialog = { type: 'rename' | 'terminate'; sessionId: string } | null;

export function AppShell() {
  const [auth, setAuth] = useState(loadAuth());
  const [sessions, setSessions] = useState<SessionSummary[]>([]);
  const [view, setView] = useState<TabView>(() => loadStoredTabs(typeof window === 'undefined' ? null : window.localStorage));
  const [sidebarOpen, setSidebarOpen] = useState(true);
  const [heartbeatState, setHeartbeatState] = useState<Heartbeat | null>(null);
  const [error, setError] = useState('');
  const [toast, setToast] = useState<string | null>(null);
  const [search, setSearch] = useState(false);
  const [dialog, setDialog] = useState<Dialog>(null);
  const runtimes = useRef(new Map<string, TerminalRuntime>());
  const bootId = useRef<string | null>(null);
  const syncing = useRef(false);
  const creatingInitial = useRef(false);

  useEffect(() => { saveStoredTabs(window.localStorage, view); }, [view]);
  useEffect(() => () => { for (const runtime of runtimes.current.values()) runtime.dispose(); runtimes.current.clear(); }, []);

  async function createSession() {
    try {
      const session = await api<SessionSummary>('/api/sessions', { method: 'POST', body: '{}' });
      setSessions((current) => [...current.filter((item) => item.id !== session.id), session]);
      setView((current) => openTab(current, session.id));
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
      setSessions(next.sessions);
      setView((current) => reconcileTabs(next.sessions, current));
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
    document.title = view.activeTabId ? `Roaminal · ${sessions.find((session) => session.id === view.activeTabId)?.cwd || 'Terminal'}` : 'Roaminal';
  }, [view.activeTabId, sessions]);

  useEffect(() => {
    const handler = (event: KeyboardEvent) => {
      if ((event.ctrlKey || event.metaKey) && event.shiftKey && event.key.toLowerCase() === 't') { event.preventDefault(); void createSession(); }
      if ((event.ctrlKey || event.metaKey) && event.shiftKey && event.key.toLowerCase() === 'w' && view.activeTabId) { event.preventDefault(); closeViewTab(view.activeTabId); }
      if ((event.ctrlKey || event.metaKey) && event.key.toLowerCase() === 'f' && view.activeTabId) { event.preventDefault(); setSearch(true); }
    };
    window.addEventListener('keydown', handler);
    return () => window.removeEventListener('keydown', handler);
  }, [view.activeTabId]);

  function runtime(id: string): TerminalRuntime {
    const current = runtimes.current.get(id);
    if (current) return current;
    const next = new TerminalRuntime(id, () => auth?.accessToken || null);
    runtimes.current.set(id, next);
    return next;
  }

  function selectSession(id: string) {
    setView((current) => openTab(current, id));
    if (window.matchMedia('(max-width: 800px)').matches) setSidebarOpen(false);
  }

  function closeViewTab(id: string) {
    runtimes.current.get(id)?.dispose();
    runtimes.current.delete(id);
    setView((current) => closeTab(current, id));
    if (view.activeTabId === id) setSearch(false);
  }

  async function updateTitle(id: string, title: string | null) {
    const updated = await api<SessionSummary>(`/api/sessions/${id}/title`, { method: 'PATCH', body: JSON.stringify({ title }) });
    setSessions((current) => current.map((session) => session.id === id ? updated : session));
    setDialog(null);
  }

  async function terminateSession(id: string) {
    try {
      await api(`/api/sessions/${id}`, { method: 'DELETE' });
      runtimes.current.get(id)?.dispose();
      runtimes.current.delete(id);
      setSessions((current) => current.filter((session) => session.id !== id));
      setView((current) => closeTab(current, id));
      setDialog(null);
      setSearch(false);
    } catch (err) { setToast((err as Error).message); }
  }

  async function onLogin(password: string) {
    try { const next = await login(password); setAuth(next); setError(''); }
    catch (err) { setError((err as Error).message); }
  }

  function signOut() {
    for (const runtime of runtimes.current.values()) runtime.dispose();
    runtimes.current.clear();
    clearAuth();
    setAuth(null);
  }

  if (!auth) return <AuthSessionUI error={error} onLogin={onLogin} />;
  const currentSession = sessions.find((session) => session.id === view.activeTabId);
  const currentRuntime = currentSession ? runtime(currentSession.id) : null;
  const openSessions = view.openTabIds.map((id) => sessions.find((session) => session.id === id)).filter((session): session is SessionSummary => Boolean(session));
  const dialogSession = dialog ? sessions.find((session) => session.id === dialog.sessionId) : undefined;

  return <div className="app-shell">
    <Sidebar id="terminal-sidebar" sessions={sessions} active={view.activeTabId} open={sidebarOpen} onToggle={() => setSidebarOpen((value) => !value)} onSelect={selectSession} onRename={(id) => setDialog({ type: 'rename', sessionId: id })} onAutomaticTitle={(id) => void updateTitle(id, null)} onTerminate={(id) => setDialog({ type: 'terminate', sessionId: id })} onCreate={createSession} />
    <main className={`main-panel ${sidebarOpen ? '' : 'expanded'}`}>
      <header className="topbar">
        {!sidebarOpen && <button className="icon-button sidebar-open-button" onClick={() => setSidebarOpen(true)} aria-label="Open sidebar" title="Open sidebar" aria-expanded={false} aria-controls="terminal-sidebar">›</button>}
        <SystemStatus connected={Boolean(heartbeatState)} hostname={heartbeatState?.system.hostname || ''} sessionCount={sessions.length} />
        <div className="top-actions"><button className="icon-button" onClick={() => setSearch((value) => !value)} aria-label="Search terminal" title="Search terminal">⌕</button><button className="text-button" onClick={signOut}>Sign out</button></div>
      </header>
      <TerminalTabs sessions={openSessions} active={view.activeTabId} onSelect={selectSession} onCloseTab={closeViewTab} onRename={(id) => setDialog({ type: 'rename', sessionId: id })} onAutomaticTitle={(id) => void updateTitle(id, null)} onTerminate={(id) => setDialog({ type: 'terminate', sessionId: id })} onCreate={createSession} />
      {search && currentRuntime && <TerminalSearch runtime={currentRuntime} onClose={() => setSearch(false)} />}
      <section className="terminal-stage">{currentRuntime ? <TerminalViewport key={currentRuntime.sessionId} runtime={currentRuntime} /> : <div className="empty-state"><div className="brand-mark">r<span>&gt;</span></div><button className="primary" onClick={createSession}>Create terminal</button></div>}</section>
      {currentRuntime && <TouchKeyboard onInput={(value) => currentRuntime.send({ type: 'input', data: value })} />}
      <footer className="statusbar"><span>{currentSession?.cwd || 'No terminal'}</span><span>{currentSession ? `${currentSession.cols}×${currentSession.rows}` : ''}</span></footer>
    </main>
    <Toast message={toast} />
    {dialog?.type === 'rename' && dialogSession && <RenameTitleDialog session={dialogSession} onSave={(title) => updateTitle(dialogSession.id, title)} onClose={() => setDialog(null)} />}
    {dialog?.type === 'terminate' && dialogSession && <TerminateDialog session={dialogSession} onConfirm={() => terminateSession(dialogSession.id)} onClose={() => setDialog(null)} />}
  </div>;
}
