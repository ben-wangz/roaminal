import { Copy, Edit3, Home, KeyRound, Play, ShieldAlert, Trash2 } from 'lucide-react';
import type { ConnectionInstanceSummary } from '../terminal/terminal-protocol';
import type { ConnectionDefinition, ConfigSource } from './connection-api';
import { reusableInstanceForHost } from './connection-instance-selection';

export function SourceBand({ source, label = 'SSH config' }: { source: ConfigSource; label?: string }) {
  const warnings = source.warnings || [];
  const blockers = source.blockers || [];
  return (
    <div className={`source-band ${source.writable ? 'writable' : 'readonly'}`}>
      <div>
        <strong>{label}</strong>
        <span>{source.status}</span>
      </div>
      <div className="source-capabilities">
        <span>{source.readable ? 'read' : 'unreadable'}</span>
        <span>{source.writable ? 'write' : 'read-only'}</span>
        {warnings.length > 0 && <span>{warnings.length} warnings</span>}
      </div>
      {(blockers.length > 0 || source.reason) && <small>{source.reason || blockers.join(', ')}</small>}
    </div>
  );
}

export function LocalConnectionRow({ onConnect }: { onConnect: () => void }) {
  return (
    <article className="connection-row local-row">
      <div className="connection-row-main">
        <span className="row-icon local-icon">
          <Home size={17} aria-hidden="true" />
        </span>
        <div>
          <strong>Local</strong>
          <small>/workspace</small>
        </div>
      </div>
      <button
        className="icon-button play-button"
        type="button"
        onClick={onConnect}
        aria-label="Start local connection"
        title="Start local connection"
      >
        <Play size={16} fill="currentColor" aria-hidden="true" />
      </button>
    </article>
  );
}

type DefinitionRowProps = {
  definition: ConnectionDefinition;
  editable: boolean;
  connections: ConnectionInstanceSummary[];
  onConnect: (id: string, reuseFrom?: string, tmuxEnabled?: boolean) => Promise<void>;
  onEdit: () => void;
  onDuplicate: () => void;
  onDelete: () => void;
};

export function ConnectionDefinitionRow({
  definition,
  editable,
  connections,
  onConnect,
  onEdit,
  onDuplicate,
  onDelete,
}: DefinitionRowProps) {
  const reusable = reusableInstanceForHost(connections, definition.hostAlias || '');
  const destination =
    [definition.user, definition.hostName || definition.hostAlias].filter(Boolean).join('@') +
    (definition.port ? `:${definition.port}` : '');
  return (
    <article className="connection-row">
      <div className="connection-row-main">
        <span className="row-icon">
          <KeyRound size={17} aria-hidden="true" />
        </span>
        <div className="connection-copy">
          <strong>{definition.hostAlias}</strong>
          <small>{destination || 'OpenSSH config destination'}</small>
          <small className="row-facts">
            {definition.identityFileNames.length} managed keys |{' '}
            {definition.hostVerificationAssessment === 'weakened'
              ? 'weakened trust'
              : definition.hostVerificationAssessment}
            {definition.tmux?.enabled ? ` | tmux:${definition.tmux.sessionName}` : ''}
          </small>
        </div>
      </div>
      <div className="connection-row-status">
        {definition.warnings.length + definition.advancedDirectiveCount > 0 && (
          <span className="warning-badge">
            <ShieldAlert size={13} aria-hidden="true" />{' '}
            {definition.warnings.length + definition.advancedDirectiveCount}
          </span>
        )}
        {definition.tmux?.enabled && <span className="tmux-badge">tmux</span>}
        {(!editable || definition.capabilities.edit === false) && <span className="readonly-badge">read-only</span>}
      </div>
      <div className="connection-row-actions">
        <button
          className="icon-button"
          type="button"
          onClick={() =>
            void onConnect(
              definition.connectionDefinitionId,
              reusable?.connectionInstanceId,
              Boolean(definition.tmux?.enabled),
            )
          }
          aria-label={`Start connection to ${definition.hostAlias}`}
          title="Start connection"
        >
          <Play size={16} fill="currentColor" aria-hidden="true" />
        </button>
        <button
          className="icon-button"
          type="button"
          onClick={onEdit}
          disabled={!editable || definition.capabilities.edit === false}
          aria-label={`Edit ${definition.hostAlias}`}
          title="Edit connection"
        >
          <Edit3 size={15} aria-hidden="true" />
        </button>
        <button
          className="icon-button"
          type="button"
          onClick={onDuplicate}
          disabled={!editable || definition.capabilities.edit === false}
          aria-label={`Duplicate ${definition.hostAlias}`}
          title="Duplicate connection"
        >
          <Copy size={15} aria-hidden="true" />
        </button>
        <button
          className="icon-button danger-icon"
          type="button"
          onClick={onDelete}
          disabled={!editable || definition.capabilities.delete === false}
          aria-label={`Delete ${definition.hostAlias}`}
          title="Delete connection"
        >
          <Trash2 size={15} aria-hidden="true" />
        </button>
      </div>
    </article>
  );
}
