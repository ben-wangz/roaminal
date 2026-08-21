import { useEffect, useRef, useState } from 'react';
import { Modal } from './modal';
import { getAgent, getAgentInitialization, initializeAgent, type AgentDetails, type AgentInitialization } from '../agent/agent-api';
import { agentSummary } from '../agent/agent-api';
import type { ConnectionInstanceSummary } from '../terminal/terminal-protocol';
import type { ToastKind } from './toast';

export function AgentDialog({
  connection,
  onClose,
  onShowToast,
}: {
  connection: ConnectionInstanceSummary;
  onClose: () => void;
  onShowToast: (message: string, kind?: ToastKind) => void;
}) {
  const [details, setDetails] = useState<AgentDetails | null>(null);
  const [operation, setOperation] = useState<AgentInitialization | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const current = details?.agent || agentSummary(connection);
  useEffect(() => {
    let active = true;
    void getAgent(connection.connectionInstanceId)
      .then((value) => {
        if (active) setDetails(value);
      })
      .catch((err) => {
        if (active) setError((err as Error).message);
      });
    return () => {
      active = false;
    };
  }, [connection.connectionInstanceId]);
  useEffect(() => {
    const initializationId = details?.agent.initializationId || connection.agent?.initializationId;
    if (!initializationId || operation || details?.agent.component !== 'initializing') return;
    let active = true;
    void getAgentInitialization(initializationId)
      .then((value) => {
        if (active) setOperation(value);
      })
      .catch((err) => {
        if (active) setError((err as Error).message);
      });
    return () => {
      active = false;
    };
  }, [connection.agent?.initializationId, details, operation]);
  useEffect(() => {
    const operationId = operation?.initializationId;
    if (!operationId || operation?.status !== 'running') return;
    let active = true;
    const refresh = async () => {
      try {
        const next = await getAgentInitialization(operationId);
        if (!active) return;
        setOperation(next);
        if (next.status !== 'running') {
          const nextDetails = await getAgent(connection.connectionInstanceId);
          if (active) setDetails(nextDetails);
          if (next.status === 'completed') onShowToast('Codex Agent component installed.', 'success');
          if (next.status === 'failed') setError(next.error?.message || 'Agent initialization failed.');
        }
      } catch (err) {
        if (active) setError((err as Error).message);
      }
    };
    const timer = window.setInterval(() => void refresh(), 2000);
    void refresh();
    return () => {
      active = false;
      window.clearInterval(timer);
    };
  }, [connection.connectionInstanceId, onShowToast, operation?.initializationId, operation?.status]);
  async function initialize() {
    setBusy(true);
    setError('');
    try {
      const next = await initializeAgent(connection.connectionInstanceId);
      setOperation(next);
      if (next.status === 'completed') {
        setDetails(await getAgent(connection.connectionInstanceId));
      }
      if (next.status === 'failed') setError(next.error?.message || 'Agent initialization failed.');
    } catch (err) {
      setError((err as Error).message);
    } finally {
      setBusy(false);
    }
  }
  const endpoint = details?.endpoint?.display || 'Resolved from the active SSH connection';
  const initializing = current.component === 'initializing' || operation?.status === 'running';
  const supported = current.support === 'supported';
  const needsInstall = current.component === 'uninitialized' || current.component === 'error';
  const repair = current.component === 'ready' || current.component === 'error';
  return (
    <Modal onClose={onClose}>
      <div className="dialog-form agent-dialog">
        <div className="agent-dialog-heading">
          <span className={`agent-dialog-indicator agent-status-${current.component}`} aria-hidden="true" />
          <div>
            <h2>Codex Agent</h2>
            <p className="dialog-copy">{connection.title || 'Connection'}</p>
          </div>
        </div>
        <dl className="agent-detail-list">
          <div><dt>Endpoint</dt><dd>{endpoint}</dd></div>
          <div><dt>tmux session</dt><dd>{connection.tmuxSessionName || 'Unavailable'}</dd></div>
          <div><dt>Webhook</dt><dd>{details?.webhookUrl || 'Configured server endpoint'}</dd></div>
          <div><dt>Component</dt><dd>{current.component.replaceAll('_', ' ')}</dd></div>
          <div><dt>Activity</dt><dd>{current.activityLabel}</dd></div>
        </dl>
        {needsInstall && supported && (
          <p className="dialog-copy agent-confirm-copy">
            Roaminal will install the Codex Agent helper under <code>$HOME/.roaminal/</code> and merge the user-level
            <code>$HOME/.codex/hooks.json</code>. It sends status metadata only, not prompts, transcript content, tool
            arguments, or tool output.
          </p>
        )}
        {current.component === 'needs_trust' && (
          <p className="dialog-copy agent-trust-copy">
            The helper is installed. Restart Codex in this tmux session, open <code>/hooks</code>, and trust the Roaminal
            hook before status events can be reported.
          </p>
        )}
        {!supported && <p className="error-text">Agent unavailable: {current.supportReason.replaceAll('_', ' ')}</p>}
        {current.component === 'error' && current.errorMessage && <p className="error-text">{current.errorMessage}</p>}
        {details?.webhookUrl?.startsWith('http://') && (
          <p className="error-text">This Agent webhook uses HTTP. Only use it on a trusted loopback or explicitly configured private network.</p>
        )}
        {error && <div className="error-text" role="alert">{error}</div>}
        <div className="dialog-actions">
          <button type="button" className="text-button" onClick={onClose}>{needsInstall && !initializing ? 'Cancel' : 'Close'}</button>
          {supported && (needsInstall || current.component === 'needs_trust' || current.component === 'ready') && (
            <button type="button" className="primary" disabled={busy || initializing} onClick={() => void initialize()}>
              {initializing ? 'Initializing...' : repair ? 'Repair' : 'Initialize'}
            </button>
          )}
        </div>
      </div>
    </Modal>
  );
}

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
