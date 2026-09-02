import { CheckCircle2, Copy, Edit3, FileText, Folder, Home, KeyRound, Play, ShieldAlert, Terminal, Trash2, TriangleAlert } from 'lucide-react';
import type { ConnectionInstanceSummary } from '../terminal/terminal-protocol';
import type { ConnectionDefinition, ConfigSource } from './connection-api';
import { reusableInstanceForHost } from './connection-instance-selection';

export function SourceBand({ source, label = 'SSH config', error }: { source: ConfigSource; label?: string; error?: string }) {
  const warnings = source.warnings || [];
  const blockers = source.blockers || [];
  const Icon = label === 'SSH config' ? FileText : label === 'Roaminal tmux' ? Terminal : Folder;
  const available = !error && source.readable && (source.writable || source.status !== 'unavailable');
  const state = error ? 'Error' : source.writable ? 'Writable' : source.readable ? 'Readable' : source.status === 'missing' ? 'Unavailable' : 'Read-only';
  return (
    <article className={`source-band source-card ${source.writable ? 'writable' : 'readonly'} ${available ? 'available' : 'unavailable'}`}>
      <div className="source-card-heading">
        <div className="source-card-title"><Icon size={21} aria-hidden="true" /><strong>{label}</strong></div>
        <span className={`source-card-state source-card-state-${error ? 'unavailable' : source.writable ? 'writable' : available ? 'readable' : 'unavailable'}`}>
          {state} {available ? <CheckCircle2 size={17} aria-hidden="true" /> : <TriangleAlert size={17} aria-hidden="true" />}
        </span>
      </div>
      <p className="source-card-fact">{label === 'SSH config' ? 'Saved SSH destinations' : label === 'Roaminal tmux' ? 'Session management' : 'Fallback PWD settings'}</p>
      <p className={`source-card-secondary ${error ? 'source-card-error' : ''}`}>{error || source.reason || blockers[0] || (warnings.length ? `${warnings.length} warning${warnings.length === 1 ? '' : 's'}` : source.writable ? 'Read and write available' : source.readable ? 'Read available' : 'Source unavailable')}</p>
    </article>
  );
}

export function LocalConnectionRow({ onConnect }: { onConnect: () => void }) {
  return (
    <article className="connection-row local-row" role="row">
      <div className="connection-row-main" role="cell">
        <span className="row-icon local-icon">
          <Home size={17} aria-hidden="true" />
        </span>
        <div>
          <strong>Local</strong>
          <small>/workspace</small>
        </div>
      </div>
      <div className="connection-row-cell connection-row-keys" role="cell">—</div>
      <div className="connection-row-cell connection-row-trust" role="cell"><span className="status-mark status-ok">✓</span> Trusted</div>
      <div className="connection-row-cell connection-row-tmux" role="cell"><span className="status-mark status-muted">−</span> Disabled</div>
      <div className="connection-row-actions" role="cell">
        <button
          className="icon-button play-button"
          type="button"
          onClick={onConnect}
          aria-label="Start local connection"
          title="Start local connection"
        >
          <Play size={16} fill="currentColor" aria-hidden="true" />
        </button>
      </div>
    </article>
  );
}

type DefinitionRowProps = {
  definition: ConnectionDefinition;
  editable: boolean;
  busy: boolean;
  connections: ConnectionInstanceSummary[];
  onConnect: (id: string, reuseFrom?: string, tmuxEnabled?: boolean) => Promise<boolean>;
  onEdit: () => void;
  onDuplicate: () => void;
  onDelete: () => void;
};

export function ConnectionDefinitionRow({
  definition,
  editable,
  busy,
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
    <article className="connection-row" role="row">
      <div className="connection-row-main" role="cell">
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
          {definition.warnings.length + definition.advancedDirectiveCount > 0 && (
            <small className="row-warning"><ShieldAlert size={13} aria-hidden="true" /> {definition.warnings.length + definition.advancedDirectiveCount} warning{definition.warnings.length + definition.advancedDirectiveCount === 1 ? '' : 's'}</small>
          )}
          {(!editable || definition.capabilities.edit === false) && <small className="row-readonly">Read-only</small>}
        </div>
      </div>
      <div className="connection-row-cell connection-row-keys" role="cell">
        {definition.identityFileNames.length || '—'}
      </div>
      <div className="connection-row-cell connection-row-trust" role="cell">
        <span className={`status-mark ${definition.hostVerificationAssessment === 'default' ? 'status-ok' : 'status-warning'}`} aria-hidden="true">
          {definition.hostVerificationAssessment === 'default' ? '✓' : '!'}
        </span>
        {definition.hostVerificationAssessment === 'weakened' ? 'Weakened' : definition.hostVerificationAssessment === 'unknown' ? 'Unknown' : 'Trusted'}
      </div>
      <div className="connection-row-cell connection-row-tmux" role="cell">
        <span className={`status-mark ${definition.tmux?.enabled ? 'status-ok' : 'status-muted'}`} aria-hidden="true">
          {definition.tmux?.enabled ? '✓' : '−'}
        </span>
        {definition.tmux?.enabled ? 'Enabled' : 'Disabled'}
      </div>
      <div className="connection-row-actions" role="cell">
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
          disabled={busy || !editable || definition.capabilities.edit === false}
          aria-label={`Edit ${definition.hostAlias}`}
          title="Edit connection"
        >
          <Edit3 size={15} aria-hidden="true" />
        </button>
        <button
          className="icon-button"
          type="button"
          onClick={onDuplicate}
          disabled={busy || !editable || definition.capabilities.edit === false}
          aria-label={`Duplicate ${definition.hostAlias}`}
          title="Duplicate connection"
        >
          <Copy size={15} aria-hidden="true" />
        </button>
        <button
          className="icon-button danger-icon"
          type="button"
          onClick={onDelete}
          disabled={busy || !editable || definition.capabilities.delete === false}
          aria-label={`Delete ${definition.hostAlias}`}
          title="Delete connection"
        >
          <Trash2 size={15} aria-hidden="true" />
        </button>
      </div>
    </article>
  );
}
