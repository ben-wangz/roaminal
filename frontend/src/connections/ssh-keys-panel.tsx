import { useState } from 'react';
import { Clipboard, KeyRound, RefreshCw, Trash2 } from 'lucide-react';
import { publicKey, type ConnectionDefinition, type GenerationRequest, type SSHKey } from './connection-api';
import type { ToastKind } from '../ui/toast';

type Props = {
  keys: SSHKey[];
  definitions: ConnectionDefinition[];
  busy: boolean;
  onRefresh: () => void;
  onGenerate: (algorithm: GenerationRequest['algorithm']) => void;
  onDelete: (key: SSHKey) => Promise<void>;
  onToast: (message: string, kind?: ToastKind) => void;
};

export function SSHKeysPanel({ keys, definitions, busy, onRefresh, onGenerate, onDelete, onToast }: Props) {
  const [copying, setCopying] = useState<string | null>(null);
  async function copy(key: SSHKey) {
    setCopying(key.keyId);
    try {
      await navigator.clipboard.writeText(await publicKey(key.keyId));
      onToast('Public key copied.', 'success');
    } catch (error) {
      onToast((error as Error).message, 'error');
    } finally {
      setCopying(null);
    }
  }
  return (
    <div className="keys-panel">
      <div className="settings-toolbar settings-key-toolbar">
        <p className="panel-muted">Private key contents never leave the SSH directory.</p>
        <div className="key-generate-actions">
          <button className="settings-secondary-action" type="button" onClick={onRefresh} disabled={busy}>
            <RefreshCw size={17} aria-hidden="true" className={busy ? 'spin' : ''} /> Refresh
          </button>
          <button className="text-button" type="button" onClick={() => onGenerate('ed25519')} disabled={busy}>
            <KeyRound size={15} aria-hidden="true" /> Generate Ed25519
          </button>
          <button className="primary" type="button" onClick={() => onGenerate('rsa')} disabled={busy}>
            <KeyRound size={15} aria-hidden="true" /> Generate RSA
          </button>
        </div>
      </div>
      <div className="key-table" role="table" aria-label="SSH keys">
        <div className="key-table-head" role="row">
          <span role="columnheader">Filename</span>
          <span role="columnheader">Algorithm</span>
          <span role="columnheader">Fingerprint</span>
          <span role="columnheader">Usage</span>
          <span role="columnheader" aria-label="Actions" />
        </div>
        {keys.map((key) => (
          <div className="key-table-row" role="row" key={key.keyId}>
            <span role="cell">
              <strong>{key.fileName}</strong>
              <small>
                {key.readOnly ? 'read-only' : 'writable'} | {key.status}
              </small>
            </span>
            <span role="cell">
              {key.algorithm}
              {key.bits ? ` ${key.bits}` : ''}
            </span>
            <code role="cell">{key.fingerprint || 'Unavailable'}</code>
            <span role="cell">
              {(() => {
                const referenceCount = definitions.filter((definition) => definition.identityFileNames.includes(key.fileName)).length;
                return referenceCount ? `${referenceCount} definition${referenceCount === 1 ? '' : 's'}` : '-';
              })()}
            </span>
            <span className="key-actions" role="cell">
              {key.publicKeyAvailable && (
                <button
                  className="icon-button"
                  type="button"
                  disabled={copying === key.keyId}
                  onClick={() => void copy(key)}
                  aria-label={`Copy public key ${key.fileName}`}
                  title="Copy public key"
                >
                  <Clipboard size={15} aria-hidden="true" />
                </button>
              )}
              <button
                className="icon-button danger-icon"
                type="button"
                  disabled={busy || key.readOnly}
                onClick={() => void onDelete(key)}
                aria-label={`Delete SSH key ${key.fileName}`}
                title={key.readOnly ? 'Mounted key cannot be deleted' : 'Delete SSH key'}
              >
                <Trash2 size={15} aria-hidden="true" />
              </button>
            </span>
          </div>
        ))}
        {keys.length === 0 && <div className="manager-empty">No managed Ed25519 or RSA keys detected.</div>}
      </div>
    </div>
  );
}
