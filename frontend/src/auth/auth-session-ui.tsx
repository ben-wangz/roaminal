import { useState } from 'react';
import { RefreshCw, ShieldCheck } from 'lucide-react';

export function AuthSessionUI({ error, onLogin }: { error: string; onLogin: (password: string) => Promise<void> }) {
  const [password, setPassword] = useState('');
  const [busy, setBusy] = useState(false);
  const submit = async (event: React.FormEvent) => { event.preventDefault(); setBusy(true); try { await onLogin(password); } finally { setBusy(false); } };
  return <div className="auth-backdrop"><form className="auth-modal" onSubmit={submit}>
    <div className="brand-mark">r<span>&gt;</span></div><h1>Roaminal</h1><p className="auth-subtitle">Secure terminal access</p>
    <label className="auth-username-label" htmlFor="username">Username</label><input id="username" name="username" className="auth-username" type="text" autoComplete="username" value="roaminal" readOnly tabIndex={-1} />
    <label htmlFor="password">Password</label><input id="password" name="password" type="password" autoComplete="current-password" autoFocus value={password} onChange={(event) => setPassword(event.target.value)} />
    {error && <div className="error-text" role="alert">{error}</div>}<button className="primary" disabled={busy || !password}>{busy ? 'Connecting...' : 'Connect'}</button>
  </form></div>;
}

export type AuthSessionSummary = { id: string; createdAt: string; lastSeenAt: string; refreshExpiresAt: string; userAgent: string; current: boolean };

type AuthSessionsActions = {
  sessions: AuthSessionSummary[];
  currentId: string;
  busy: string | null;
  onRevoke: (id: string) => void;
  onLogoutOthers: () => void;
};

function AuthSessionRows({ sessions, currentId, busy, onRevoke }: Pick<AuthSessionsActions, 'sessions' | 'currentId' | 'busy' | 'onRevoke'>) {
  return <>{sessions.map((session) => <div className="auth-session-row" key={session.id}>
    <div className="auth-session-copy"><strong>{session.current || session.id === currentId ? 'This browser' : 'Other browser'}</strong><small>{session.userAgent || 'Unknown client'} · last seen {new Date(session.lastSeenAt).toLocaleString()}</small><code>{session.id.slice(-12)}</code></div>
    <button type="button" className="text-button destructive-text" disabled={busy !== null} onClick={() => onRevoke(session.id)}>{busy === session.id ? 'Revoking...' : 'Revoke'}</button>
  </div>)}</>;
}

export type AuthSessionsPanelProps = AuthSessionsActions & {
  loading: boolean;
  onRefresh: () => void;
};

export function AuthSessionsPanel({ sessions, currentId, busy, onRevoke, onLogoutOthers, loading, onRefresh }: AuthSessionsPanelProps) {
  const controlsDisabled = loading || busy !== null;
  return <section className="settings-panel settings-auth-sessions-panel" aria-labelledby="settings-auth-sessions-title">
    <header className="settings-auth-sessions-header">
      <div><h2 id="settings-auth-sessions-title">Login sessions</h2><p>Review and revoke active refresh sessions.</p></div>
      <button type="button" className="settings-secondary-action" disabled={controlsDisabled} onClick={onRefresh}>
        <RefreshCw size={16} aria-hidden="true" className={loading ? 'spin' : ''} /> Refresh
      </button>
    </header>
    <div className="auth-session-list" aria-live="polite">
      {loading ? <div className="settings-auth-sessions-status" role="status">Loading sessions...</div>
        : sessions.length ? <AuthSessionRows sessions={sessions} currentId={currentId} busy={busy} onRevoke={onRevoke} />
          : <div className="settings-auth-sessions-status">No active login sessions.</div>}
    </div>
    <footer className="settings-auth-sessions-footer"><ShieldCheck aria-hidden="true" size={15} /><button type="button" className="text-button" disabled={controlsDisabled} onClick={onLogoutOthers}>{busy === 'others' ? 'Revoking...' : 'Log out other sessions'}</button></footer>
  </section>;
}
