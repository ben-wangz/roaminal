import { useState } from 'react';
import { ShieldCheck, X } from 'lucide-react';
import { Modal } from '../ui/modal';

export function AuthSessionUI({ error, onLogin }: { error: string; onLogin: (password: string) => Promise<void> }) {
  const [password, setPassword] = useState('');
  const [busy, setBusy] = useState(false);
  const submit = async (event: React.FormEvent) => { event.preventDefault(); setBusy(true); try { await onLogin(password); } finally { setBusy(false); } };
  return <div className="auth-backdrop"><form className="auth-modal" onSubmit={submit}>
    <div className="brand-mark">r<span>&gt;</span></div><h1>Roaminal</h1><p className="auth-subtitle">Secure terminal access</p>
    <label htmlFor="password">Password</label><input id="password" type="password" autoComplete="current-password" autoFocus value={password} onChange={(event) => setPassword(event.target.value)} />
    {error && <div className="error-text" role="alert">{error}</div>}<button className="primary" disabled={busy || !password}>{busy ? 'Connecting...' : 'Connect'}</button>
  </form></div>;
}

export type AuthSessionSummary = { id: string; createdAt: string; lastSeenAt: string; refreshExpiresAt: string; userAgent: string; current: boolean };

export function AuthSessionsDialog({ sessions, currentId, busy, onRevoke, onLogoutOthers, onClose }: { sessions: AuthSessionSummary[]; currentId: string; busy: string | null; onRevoke: (id: string) => void; onLogoutOthers: () => void; onClose: () => void }) {
  return <Modal onClose={onClose}><section className="auth-sessions-dialog" aria-labelledby="auth-sessions-title">
    <header><div><h2 id="auth-sessions-title">Login sessions</h2><p>Review and revoke active refresh sessions.</p></div><button type="button" className="icon-button" aria-label="Close sessions" title="Close sessions" onClick={onClose}><X aria-hidden="true" size={17} /></button></header>
    <div className="auth-session-list">{sessions.map((session) => <div className="auth-session-row" key={session.id}>
      <div className="auth-session-copy"><strong>{session.current || session.id === currentId ? 'This browser' : 'Other browser'}</strong><small>{session.userAgent || 'Unknown client'} · last seen {new Date(session.lastSeenAt).toLocaleString()}</small><code>{session.id.slice(-12)}</code></div>
      <button type="button" className="text-button destructive-text" disabled={busy !== null} onClick={() => onRevoke(session.id)}>{busy === session.id ? 'Revoking...' : 'Revoke'}</button>
    </div>)}</div>
    <footer><ShieldCheck aria-hidden="true" size={15} /><button type="button" className="text-button" disabled={busy !== null} onClick={onLogoutOthers}>{busy === 'others' ? 'Revoking...' : 'Log out other sessions'}</button></footer>
  </section></Modal>;
}
