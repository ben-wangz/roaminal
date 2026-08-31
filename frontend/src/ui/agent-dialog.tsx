import { useEffect, useState } from 'react';
import { getAgent, getAgentInitialization, initializeAgent, agentSummary, type AgentDetails, type AgentInitialization } from '../agent/agent-api';
import type { ConnectionInstanceSummary } from '../terminal/terminal-protocol';
import type { ToastKind } from './toast';
import { Modal } from './modal';
import { loadAuth } from '../auth/auth-client';
import { fetchNotificationPreferences, saveNotificationPreference, type NotificationPreference } from '../status/notification-api';
import { updateNotificationPreference as updateBrowserNotificationPreference } from '../status/notification-service';

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
  const [notificationPreference, setNotificationPreference] = useState<NotificationPreference | null>(null);
  const [notificationPreferenceBusy, setNotificationPreferenceBusy] = useState(false);
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
    const auth = loadAuth();
    if (!auth || !connection.connectionDefinitionId || !connection.tmuxSessionName) return;
    let active = true;
    void fetchNotificationPreferences(auth).then((result) => {
      if (!active) return;
      const match = result.preferences.find((preference) => preference.connectionDefinitionId === connection.connectionDefinitionId && preference.tmuxSessionName === connection.tmuxSessionName);
      setNotificationPreference(match || {
        connectionDefinitionId: connection.connectionDefinitionId!, tmuxSessionName: connection.tmuxSessionName!, enabled: false, runningToRelax: false, runningToError: false,
      });
    }).catch(() => undefined);
    return () => { active = false; };
  }, [connection.connectionDefinitionId, connection.tmuxSessionName]);
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
          <div><dt>Component</dt><dd>{current.component.replaceAll('_', ' ')}</dd></div>
          <div><dt>Activity</dt><dd>{current.activityLabel}</dd></div>
          <div><dt>Agent state</dt><dd>{current.stateLabel || 'Agent status unknown'}</dd></div>
        </dl>
        {notificationPreference && (
          <section className="agent-notification-preferences" aria-labelledby="agent-notification-preferences-title">
            <h3 id="agent-notification-preferences-title">Browser notifications</h3>
            <label className="checkbox-row">
              <input
                id="agent-notification-enabled"
                name="agentNotificationEnabled"
                type="checkbox"
                checked={notificationPreference.enabled}
                disabled={notificationPreferenceBusy}
                onChange={(event) => void updateNotificationPreference({ enabled: event.target.checked })}
              />
              Notify for this connection
            </label>
            <label className="checkbox-row">
              <input
                id="agent-notification-running-relax"
                name="agentNotificationRunningToRelax"
                type="checkbox"
                checked={notificationPreference.runningToRelax}
                disabled={notificationPreferenceBusy || !notificationPreference.enabled}
                onChange={(event) => void updateNotificationPreference({ runningToRelax: event.target.checked })}
              />
              Agent running to idle
            </label>
            <label className="checkbox-row">
              <input
                id="agent-notification-running-error"
                name="agentNotificationRunningToError"
                type="checkbox"
                checked={notificationPreference.runningToError}
                disabled={notificationPreferenceBusy || !notificationPreference.enabled}
                onChange={(event) => void updateNotificationPreference({ runningToError: event.target.checked })}
              />
              Agent running to error
            </label>
          </section>
        )}
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

  async function updateNotificationPreference(update: Partial<NotificationPreference>) {
    if (!notificationPreference || notificationPreferenceBusy) return;
    const auth = loadAuth();
    if (!auth) {
      setError('Authentication is no longer available.');
      return;
    }
    const next = { ...notificationPreference, ...update };
    setNotificationPreferenceBusy(true);
    setNotificationPreference(next);
    try {
      const saved = await saveNotificationPreference(auth, next);
      setNotificationPreference(saved);
      updateBrowserNotificationPreference(saved);
    } catch (err) {
      setNotificationPreference(notificationPreference);
      setError((err as Error).message);
    } finally {
      setNotificationPreferenceBusy(false);
    }
  }
}
