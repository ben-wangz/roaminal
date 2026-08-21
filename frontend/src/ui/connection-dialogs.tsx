import { useEffect, useRef, useState } from 'react';
import { Modal } from './modal';
import type { ConnectionInstanceSummary } from '../terminal/terminal-protocol';

function titleError(value: string): string {
  const trimmed = value.trim();
  if (!trimmed) return 'Title is required.';
  if ([...trimmed].length > 128 || new TextEncoder().encode(trimmed).length > 512) return 'Title is too long.';
  for (const rune of trimmed) {
    const code = rune.codePointAt(0) || 0;
    if (
      code < 0x20 ||
      (code >= 0x7f && code <= 0x9f) ||
      (code >= 0x202a && code <= 0x202e) ||
      (code >= 0x2066 && code <= 0x2069)
    )
      return 'Title contains a prohibited character.';
  }
  return '';
}

export function RenameTitleDialog({
  connection,
  onSave,
  onClose,
}: {
  connection: ConnectionInstanceSummary;
  onSave: (title: string | null) => Promise<void>;
  onClose: () => void;
}) {
  const [value, setValue] = useState(connection.titleMode === 'custom' ? connection.title : '');
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);
  const input = useRef<HTMLInputElement>(null);
  useEffect(() => input.current?.focus(), []);
  async function save(event: React.FormEvent) {
    event.preventDefault();
    const message = titleError(value);
    if (message) {
      setError(message);
      return;
    }
    setBusy(true);
    try {
      await onSave(value.trim());
    } catch (err) {
      setError((err as Error).message);
    } finally {
      setBusy(false);
    }
  }
  return (
    <Modal onClose={onClose}>
      <form className="dialog-form" onSubmit={save}>
        <h2>Rename connection</h2>
        <label htmlFor="terminal-title">Title</label>
        <input
          id="terminal-title"
          ref={input}
          value={value}
          onChange={(event) => {
            setValue(event.target.value);
            setError('');
          }}
          maxLength={128}
        />
        {error && (
          <div className="error-text" role="alert">
            {error}
          </div>
        )}
        <div className="dialog-actions">
          <button type="button" className="text-button" onClick={onClose}>
            Cancel
          </button>
          <button type="submit" className="primary" disabled={busy}>
            {busy ? 'Saving...' : 'Save title'}
          </button>
        </div>
      </form>
    </Modal>
  );
}

export function AutomaticTitleDialog({
  connection,
  onReset,
  onClose,
}: {
  connection: ConnectionInstanceSummary;
  onReset: () => Promise<void>;
  onClose: () => void;
}) {
  const [busy, setBusy] = useState(false);
  async function reset() {
    setBusy(true);
    try {
      await onReset();
    } finally {
      setBusy(false);
    }
  }
  return (
    <Modal onClose={onClose}>
      <div className="dialog-form">
        <h2>Use automatic title</h2>
        <p className="dialog-copy">
          The shell will control the title for {connection.connectionInstanceId.slice(0, 6)}.
        </p>
        <div className="dialog-actions">
          <button type="button" className="text-button" onClick={onClose}>
            Cancel
          </button>
          <button type="button" className="primary" disabled={busy} onClick={() => void reset()}>
            {busy ? 'Updating...' : 'Use automatic title'}
          </button>
        </div>
      </div>
    </Modal>
  );
}

export function CloseConnectionDialog({
  connection,
  onConfirm,
  onClose,
}: {
  connection: ConnectionInstanceSummary;
  onConfirm: () => Promise<void>;
  onClose: () => void;
}) {
  const [busy, setBusy] = useState(false);
  async function confirm() {
    setBusy(true);
    try {
      await onConfirm();
    } finally {
      setBusy(false);
    }
  }
  return (
    <Modal onClose={onClose}>
      <div className="dialog-form">
        <h2>Close connection?</h2>
        <p className="dialog-copy">
          This stops the managed process for {connection.title || 'Connection'} (
          {connection.connectionInstanceId.slice(0, 6)}).
        </p>
        <div className="dialog-actions">
          <button type="button" className="text-button" onClick={onClose}>
            Cancel
          </button>
          <button type="button" className="danger-button" disabled={busy} onClick={() => void confirm()}>
            {busy ? 'Working...' : 'Close connection'}
          </button>
        </div>
      </div>
    </Modal>
  );
}
