import { useState } from 'react';

export function AuthSessionUI({ error, onLogin }: { error: string; onLogin: (password: string) => Promise<void> }) {
  const [password, setPassword] = useState('');
  const [busy, setBusy] = useState(false);
  const submit = async (event: React.FormEvent) => { event.preventDefault(); setBusy(true); try { await onLogin(password); } finally { setBusy(false); } };
  return <div className="auth-backdrop"><form className="auth-modal" onSubmit={submit}>
    <div className="brand-mark">r<span>&gt;</span></div><h1>Roaminal</h1><p className="auth-subtitle">Secure terminal access</p>
    <label htmlFor="password">Password</label><input id="password" type="password" autoFocus value={password} onChange={(event) => setPassword(event.target.value)} />
    {error && <div className="error-text" role="alert">{error}</div>}<button className="primary" disabled={busy || !password}>{busy ? 'Connecting...' : 'Connect'}</button>
  </form></div>;
}
