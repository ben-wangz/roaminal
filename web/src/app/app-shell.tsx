import { useEffect, useMemo, useRef, useState } from 'react';
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
import type { SessionSummary } from '../terminal/terminal-protocol';

export function AppShell() {
  const [auth, setAuth] = useState(loadAuth());
  const [sessions, setSessions] = useState<SessionSummary[]>([]);
  const [active, setActive] = useState<string | null>(null);
  const [sidebarOpen, setSidebarOpen] = useState(true);
  const [heartbeatState, setHeartbeatState] = useState<Heartbeat | null>(null);
  const [error, setError] = useState('');
  const [toast, setToast] = useState<string | null>(null);
  const [search, setSearch] = useState(false);
  const runtimes = useRef(new Map<string, TerminalRuntime>());

  const sync = async () => { try { const next = await heartbeat(); setHeartbeatState(next); setSessions(next.sessions); setActive((current) => current && next.sessions.some((session) => session.id === current) ? current : next.sessions[0]?.id || null); } catch (err) { if ((err as Error).message === 'unauthorized') { const next = await refresh(); setAuth(next); } } };
  useEffect(() => { if (!auth) return; void sync(); const timer = window.setInterval(() => void sync(), 1000); return () => window.clearInterval(timer); }, [auth]);
  useEffect(() => { if (!auth || sessions.length) return; void api<SessionSummary>('/api/sessions', { method: 'POST', body: '{}' }).then((session) => { setSessions([session]); setActive(session.id); }); }, [auth, sessions.length]);
  useEffect(() => { document.title = active ? `Roaminal · ${sessions.find((session) => session.id === active)?.cwd || 'Terminal'}` : 'Roaminal'; }, [active, sessions]);
  useEffect(() => { const handler = (event: KeyboardEvent) => { if ((event.ctrlKey || event.metaKey) && event.shiftKey && event.key.toLowerCase() === 't') { event.preventDefault(); void createSession(); } if ((event.ctrlKey || event.metaKey) && event.shiftKey && event.key.toLowerCase() === 'w' && active) { event.preventDefault(); void closeSession(active); } if ((event.ctrlKey || event.metaKey) && event.key.toLowerCase() === 'f' && active) { event.preventDefault(); setSearch(true); } }; window.addEventListener('keydown', handler); return () => window.removeEventListener('keydown', handler); }, [active]);

  async function createSession() { try { const session = await api<SessionSummary>('/api/sessions', { method: 'POST', body: '{}' }); setSessions((current) => [...current, session]); setActive(session.id); } catch (err) { setToast((err as Error).message); } }
  async function closeSession(id: string) { try { await api(`/api/sessions/${id}`, { method: 'DELETE' }); runtimes.current.get(id)?.dispose(); runtimes.current.delete(id); const remaining = sessions.filter((session) => session.id !== id); setSessions(remaining); setActive((current) => current === id ? (remaining[0]?.id || null) : current); if (!remaining.length) await createSession(); } catch (err) { setToast((err as Error).message); } }
  function runtime(id: string): TerminalRuntime { const current = runtimes.current.get(id); if (current) return current; const next = new TerminalRuntime(id, () => auth?.accessToken || null); runtimes.current.set(id, next); return next; }
  async function onLogin(password: string) { try { const next = await login(password); setAuth(next); setError(''); } catch (err) { setError((err as Error).message); } }
  if (!auth) return <AuthSessionUI error={error} onLogin={onLogin} />;
  const currentSession = sessions.find((session) => session.id === active);
  const currentRuntime = currentSession ? runtime(currentSession.id) : null;
  return <div className="app-shell"><Sidebar sessions={sessions} active={active} open={sidebarOpen} onToggle={() => setSidebarOpen((value) => !value)} onSelect={setActive} onClose={closeSession} onCreate={createSession} /><main className={`main-panel ${sidebarOpen ? '' : 'expanded'}`}><header className="topbar"><SystemStatus connected={Boolean(heartbeatState)} hostname={heartbeatState?.system.hostname || ''} sessionCount={sessions.length} /><div className="top-actions"><button className="icon-button" onClick={() => setSearch((value) => !value)} aria-label="Search terminal" title="Search terminal">⌕</button><button className="text-button" onClick={() => { clearAuth(); setAuth(null); }}>Sign out</button></div></header><TerminalTabs sessions={sessions} active={active} onSelect={setActive} onClose={closeSession} onCreate={createSession} />{search && currentRuntime && <TerminalSearch runtime={currentRuntime} onClose={() => setSearch(false)} />}<section className="terminal-stage">{currentRuntime ? <TerminalViewport runtime={currentRuntime} /> : <div className="empty-state"><div className="brand-mark">r<span>&gt;</span></div><button className="primary" onClick={createSession}>Create terminal</button></div>}</section>{currentRuntime && <TouchKeyboard onInput={(value) => currentRuntime.send({ type: 'input', data: value })} />}<footer className="statusbar"><span>{currentSession?.cwd || 'No terminal'}</span><span>{currentSession ? `${currentSession.cols}×${currentSession.rows}` : ''}</span></footer></main><Toast message={toast} /></div>;
}
