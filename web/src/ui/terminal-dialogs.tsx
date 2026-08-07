import { useEffect, useRef, useState } from 'react';
import type { SessionSummary } from '../terminal/terminal-protocol';
import { Modal } from './modal';

function titleError(value: string): string {
  const trimmed = value.trim();
  if (!trimmed) return 'Title is required.';
  if ([...trimmed].length > 128 || new TextEncoder().encode(trimmed).length > 512) return 'Title is too long.';
  for (const rune of trimmed) {
    const code = rune.codePointAt(0) || 0;
    if (code < 0x20 || (code >= 0x7f && code <= 0x9f) || (code >= 0x202a && code <= 0x202e) || (code >= 0x2066 && code <= 0x2069)) return 'Title contains a prohibited character.';
  }
  return '';
}

export function RenameTitleDialog({ session, onSave, onClose }: { session: SessionSummary; onSave: (title: string | null) => Promise<void>; onClose: () => void }) {
  const [value, setValue] = useState(session.titleMode === 'custom' ? session.title : '');
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);
  const input = useRef<HTMLInputElement>(null);
  useEffect(() => input.current?.focus(), []);
  async function save(event: React.FormEvent) {
    event.preventDefault();
    const message = titleError(value);
    if (message) { setError(message); return; }
    setBusy(true);
    try { await onSave(value.trim()); } catch (err) { setError((err as Error).message); } finally { setBusy(false); }
  }
  return <Modal onClose={onClose}><form className="dialog-form" onSubmit={save}>
    <h2>Rename terminal</h2>
    <label htmlFor="terminal-title">Title</label>
    <input id="terminal-title" ref={input} value={value} onChange={(event) => { setValue(event.target.value); setError(''); }} maxLength={128} />
    {error && <div className="error-text" role="alert">{error}</div>}
    <div className="dialog-actions"><button type="button" className="text-button" onClick={onClose}>Cancel</button><button type="submit" className="primary" disabled={busy}>{busy ? 'Saving...' : 'Save title'}</button></div>
  </form></Modal>;
}

export function AutomaticTitleDialog({ session, onReset, onClose }: { session: SessionSummary; onReset: () => Promise<void>; onClose: () => void }) {
  const [busy, setBusy] = useState(false);
  async function reset() { setBusy(true); try { await onReset(); } finally { setBusy(false); } }
  return <Modal onClose={onClose}><div className="dialog-form"><h2>Use automatic title</h2><p className="dialog-copy">The shell will control the title for {session.id.slice(0, 6)}.</p><div className="dialog-actions"><button type="button" className="text-button" onClick={onClose}>Cancel</button><button type="button" className="primary" disabled={busy} onClick={() => void reset()}>{busy ? 'Updating...' : 'Use automatic title'}</button></div></div></Modal>;
}

export function TerminateDialog({ session, onConfirm, onClose }: { session: SessionSummary; onConfirm: () => Promise<void>; onClose: () => void }) {
  const [busy, setBusy] = useState(false);
  async function confirm() { setBusy(true); try { await onConfirm(); } finally { setBusy(false); } }
  return <Modal onClose={onClose}><div className="dialog-form"><h2>Terminate terminal?</h2><p className="dialog-copy">This stops the Bash process for “{session.title || 'Terminal'}” ({session.id.slice(0, 6)}). Scrollback and metadata will be deleted.</p><div className="dialog-actions"><button type="button" className="text-button" onClick={onClose}>Cancel</button><button type="button" className="danger-button" disabled={busy} onClick={() => void confirm()}>{busy ? 'Terminating...' : 'Terminate terminal'}</button></div></div></Modal>;
}
