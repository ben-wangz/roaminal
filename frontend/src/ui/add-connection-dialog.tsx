import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { RefreshCw } from 'lucide-react';
import { loadDefinitions, type ConnectionDefinition, type DefinitionCollection } from '../connections/connection-api';
import { reusableInstanceForHost } from '../connections/connection-instance-selection';
import type { ConnectionInstanceSummary } from '../terminal/terminal-protocol';
import { Modal } from './modal';

type Props = {
  connections: ConnectionInstanceSummary[];
  onCreateConnection: (definitionId: string, reuseFrom?: string, tmuxEnabled?: boolean) => Promise<boolean>;
  onClose: () => void;
};

export function AddConnectionDialog({ connections, onCreateConnection, onClose }: Props) {
  const [definitions, setDefinitions] = useState<DefinitionCollection | null>(null);
  const [selectedId, setSelectedId] = useState('');
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const [loadError, setLoadError] = useState('');
  const [submitError, setSubmitError] = useState('');
  const initialLoadStarted = useRef(false);

  const refreshDefinitions = useCallback(async () => {
    setLoading(true);
    setLoadError('');
    try {
      const result = await loadDefinitions();
      setDefinitions(result.data);
    } catch (error) {
      setLoadError((error as Error).message);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    if (initialLoadStarted.current) return;
    initialLoadStarted.current = true;
    void refreshDefinitions();
  }, [refreshDefinitions]);

  const availableDefinitions = useMemo(
    () => (definitions?.definitions || []).filter((definition) => definition.type === 'ssh'),
    [definitions],
  );
  const selectedDefinition = selectedId === 'local'
    ? null
    : availableDefinitions.find((definition) => definition.connectionDefinitionId === selectedId);

  async function submit(event: React.FormEvent) {
    event.preventDefault();
    if (!selectedId || busy) return;
    setSubmitError('');
    setBusy(true);
    const reuseFrom = selectedDefinition
      ? reusableInstanceForHost(connections, selectedDefinition.hostAlias || '')?.connectionInstanceId
      : undefined;
    const ok = await onCreateConnection(
      selectedId,
      reuseFrom,
      Boolean(selectedDefinition?.tmux?.enabled),
    );
    if (ok) {
      onClose();
      return;
    }
    setSubmitError('Connection could not be started. Review the error message and retry.');
    setBusy(false);
  }

  return (
    <Modal onClose={busy ? undefined : onClose}>
      <form className="dialog-form add-connection-dialog" onSubmit={(event) => void submit(event)}>
        <header className="add-connection-heading">
          <div>
            <h2>Add connection</h2>
            <p className="dialog-copy">Choose a saved connection definition to start.</p>
          </div>
          <button
            className="icon-button"
            type="button"
            onClick={() => void refreshDefinitions()}
            disabled={loading || busy}
            aria-label="Refresh connection definitions"
            title="Refresh connection definitions"
          >
            <RefreshCw size={16} aria-hidden="true" className={loading ? 'spin' : ''} />
          </button>
        </header>
        <label htmlFor="add-connection-definition">Connection definition</label>
        <select
          id="add-connection-definition"
          name="connectionDefinition"
          value={selectedId}
          onChange={(event) => {
            setSelectedId(event.target.value);
            setSubmitError('');
          }}
          disabled={loading || busy}
        >
          <option value="">Select a connection</option>
          <option value="local">Local</option>
          {availableDefinitions.map((definition) => (
            <option key={definition.connectionDefinitionId} value={definition.connectionDefinitionId}>
              {definitionLabel(definition)}
            </option>
          ))}
        </select>
        {loading && <p className="dialog-copy" role="status">Loading connection definitions...</p>}
        {loadError && (
          <div className="error-text" role="alert">
            Unable to load connection definitions: {loadError}
          </div>
        )}
        {submitError && <div className="error-text" role="alert">{submitError}</div>}
        <div className="dialog-actions">
          <button type="button" className="text-button" onClick={onClose} disabled={busy}>
            Cancel
          </button>
          <button type="submit" className="primary" disabled={!selectedId || loading || busy}>
            {busy ? 'Connecting...' : 'Confirm and connect'}
          </button>
        </div>
      </form>
    </Modal>
  );
}

function definitionLabel(definition: ConnectionDefinition): string {
  const destination = [definition.user, definition.hostName || definition.hostAlias].filter(Boolean).join('@');
  return destination && definition.port ? `${definition.hostAlias} (${destination}:${definition.port})` : definition.hostAlias || destination || 'SSH connection';
}
