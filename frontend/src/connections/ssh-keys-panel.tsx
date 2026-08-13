import { useState } from 'react';
import { Clipboard, KeyRound, Trash2 } from 'lucide-react';
import type { ConnectionInstanceSummary } from '../terminal/terminal-protocol';
import { publicKey, type GenerationRequest, type SSHKey } from './connection-api';
import type { ToastKind } from '../ui/toast';

type Props = {
  keys: SSHKey[];
  connections: ConnectionInstanceSummary[];
  onGenerate: (algorithm: GenerationRequest['algorithm']) => void;
  onDelete: (key: SSHKey) => Promise<void>;
  onToast: (message: string, kind?: ToastKind) => void;
};

export function SSHKeysPanel({ keys, connections, onGenerate, onDelete, onToast }: Props) {
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
      <div className="manager-toolbar">
        <div>
          <h2>SSH keys</h2>
          <p className="panel-muted">Private key contents never leave the SSH directory.</p>
        </div>
        <div className="key-generate-actions">
          <button className="text-button" type="button" onClick={() => onGenerate('ed25519')}>
            <KeyRound size={15} aria-hidden="true" /> Generate Ed25519
          </button>
          <button className="primary" type="button" onClick={() => onGenerate('rsa')}>
            <KeyRound size={15} aria-hidden="true" /> Generate RSA
          </button>
        </div>
      </div>
      <div className="key-table" role="table" aria-label="SSH keys">
        <div className="key-table-head" role="row">
          <span>Filename</span>
          <span>Algorithm</span>
          <span>Fingerprint</span>
          <span>Usage</span>
          <span />
        </div>
        {keys.map((key) => (
          <div className="key-table-row" role="row" key={key.keyId}>
            <span>
              <strong>{key.fileName}</strong>
              <small>
                {key.readOnly ? 'read-only' : 'writable'} | {key.status}
              </small>
            </span>
            <span>
              {key.algorithm}
              {key.bits ? ` ${key.bits}` : ''}
            </span>
            <code>{key.fingerprint || 'Unavailable'}</code>
            <span>
              {connections.filter((instance) => instance.connectionDefinitionId && instance.sourceHostAlias).length
                ? 'referenced'
                : '-'}
            </span>
            <span className="key-actions">
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
                disabled={key.readOnly}
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
