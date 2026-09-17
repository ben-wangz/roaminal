import { useState } from 'react';
import { RefreshCw, ShieldCheck } from 'lucide-react';
import { beginLogin, confirmTOTPSetup, fetchTOTPSetup, verifyTOTP, type TOTPSetup } from './auth-client';
import type { AuthState } from './auth-storage';
import { RoaminalApiError } from '../api/http-client';

type LoginStageView = 'password' | 'setup' | 'verify';

function sixDigits(value: string): string {
  return value.replace(/\D+/g, '').slice(0, 6);
}

function pendingStageFailed(error: unknown): boolean {
  return error instanceof RoaminalApiError && (error.status === 409 || (error.status === 401 && error.field === 'pendingToken'));
}

// AuthSessionUI runs the explicit password, setup, TOTP verification, and
// authenticated states of the mandatory TOTP login. Pending tokens, setup
// secrets, QR material, and codes live only in component memory.
export function AuthSessionUI({ error, onAuthenticated }: { error: string; onAuthenticated: (auth: AuthState) => void }) {
  const [stage, setStage] = useState<LoginStageView>('password');
  const [password, setPassword] = useState('');
  const [pendingToken, setPendingToken] = useState('');
  const [setup, setSetup] = useState<TOTPSetup | null>(null);
  const [code, setCode] = useState('');
  const [busy, setBusy] = useState(false);
  const [stageError, setStageError] = useState('');
  const [notice, setNotice] = useState('');

  const restartPassword = (message: string) => {
    setStage('password');
    setPassword('');
    setCode('');
    setSetup(null);
    setPendingToken('');
    setStageError(message);
  };

  const submitPassword = async (event: React.FormEvent) => {
    event.preventDefault();
    setBusy(true);
    setStageError('');
    try {
      const pending = await beginLogin(password);
      setPendingToken(pending.pendingToken);
      if (pending.nextStep === 'setup_totp') {
        setSetup(await fetchTOTPSetup(pending.pendingToken));
        setStage('setup');
      } else {
        setStage('verify');
      }
      setPassword('');
      setCode('');
    } catch (err) {
      setStageError((err as Error).message);
    } finally {
      setBusy(false);
    }
  };

  const submitSetupCode = async (event: React.FormEvent) => {
    event.preventDefault();
    if (!setup) return;
    setBusy(true);
    setStageError('');
    try {
      await confirmTOTPSetup(pendingToken, code);
      restartPassword('');
      setNotice('Two-factor authentication enabled. Sign in again with your password and the next code.');
    } catch (err) {
      if (pendingStageFailed(err)) restartPassword((err as Error).message);
      else {
        setStageError((err as Error).message);
        setCode('');
      }
    } finally {
      setBusy(false);
    }
  };

  const submitVerifyCode = async (event: React.FormEvent) => {
    event.preventDefault();
    setBusy(true);
    setStageError('');
    try {
      onAuthenticated(await verifyTOTP(pendingToken, code));
    } catch (err) {
      if (pendingStageFailed(err)) restartPassword((err as Error).message);
      else {
        setStageError((err as Error).message);
        setCode('');
      }
    } finally {
      setBusy(false);
    }
  };

  return <div className="auth-backdrop"><form className="auth-modal" onSubmit={stage === 'password' ? submitPassword : stage === 'setup' ? submitSetupCode : submitVerifyCode}>
    <div className="brand-mark">r<span>&gt;</span></div><h1>Roaminal</h1>
    {stage === 'password' && <>
      <p className="auth-subtitle">Secure terminal access</p>
      <label className="auth-username-label" htmlFor="username">Username</label><input id="username" name="username" className="auth-username" type="text" autoComplete="username" value="roaminal" readOnly tabIndex={-1} />
      <label htmlFor="password">Password</label><input id="password" name="password" type="password" autoComplete="current-password" autoFocus value={password} onChange={(event) => setPassword(event.target.value)} />
      {(error || stageError) && <div className="error-text" role="alert">{stageError || error}</div>}
      {notice && <div className="auth-notice" role="status">{notice}</div>}
      <button className="primary" disabled={busy || !password}>{busy ? 'Connecting...' : 'Continue'}</button>
    </>}
    {stage === 'setup' && setup && <>
      <p className="auth-subtitle">Set up two-factor authentication</p>
      <div className="auth-totp-qr"><img src={setup.qrImage} alt="TOTP enrollment QR code" width={192} height={192} /></div>
      <label htmlFor="totp-secret">Manual entry secret</label>
      <input id="totp-secret" className="auth-totp-secret" type="text" readOnly value={setup.secret} onFocus={(event) => event.currentTarget.select()} />
      <label htmlFor="totp-setup-code">Six-digit confirmation code</label>
      <input id="totp-setup-code" type="text" inputMode="numeric" pattern="[0-9]*" autoComplete="one-time-code" maxLength={6} autoFocus placeholder="000000" value={code} onChange={(event) => setCode(sixDigits(event.target.value))} />
      {stageError && <div className="error-text" role="alert">{stageError}</div>}
      <button className="primary" disabled={busy || code.length !== 6}>{busy ? 'Verifying...' : 'Enable two-factor'}</button>
      <p className="auth-hint">Scan the QR code with an authenticator app, then confirm with the current code. After enabling, wait for a new code before signing in again.</p>
    </>}
    {stage === 'verify' && <>
      <p className="auth-subtitle">Enter your two-factor code</p>
      <label htmlFor="totp-verify-code">Six-digit code</label>
      <input id="totp-verify-code" type="text" inputMode="numeric" pattern="[0-9]*" autoComplete="one-time-code" maxLength={6} autoFocus placeholder="000000" value={code} onChange={(event) => setCode(sixDigits(event.target.value))} />
      {stageError && <div className="error-text" role="alert">{stageError}</div>}
      <button className="primary" disabled={busy || code.length !== 6}>{busy ? 'Verifying...' : 'Verify'}</button>
      <p className="auth-hint">Codes rotate every 30 seconds. If your code was rejected, wait for the next one.</p>
    </>}
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
