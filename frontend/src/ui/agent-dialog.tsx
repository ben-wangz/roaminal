import { useEffect, useState } from 'react';
import { getAgent, getAgentInitialization, initializeAgent, agentSummary, type AgentDetails, type AgentInitialization } from '../agent/agent-api';
import type { ConnectionInstanceSummary } from '../terminal/terminal-protocol';
import type { ToastKind } from './toast';
import { Modal } from './modal';

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
