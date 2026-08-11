import { KeyRound, Play, X } from 'lucide-react';
import { Modal } from '../ui/modal';
import type { GenerationRequest } from './connection-api';

type Props = {
  value: GenerationRequest;
  existingAlgorithms: Set<string>;
  busy: boolean;
  onChange: (value: GenerationRequest) => void;
  onSubmit: (event: React.FormEvent) => void;
  onClose: () => void;
};

export function SSHKeyGenerationDialog({ value, existingAlgorithms, busy, onChange, onSubmit, onClose }: Props) {
  return (
    <Modal onClose={onClose}>
      <form className="connection-editor" onSubmit={onSubmit}>
        <header>
          <div>
            <p className="eyebrow">SSH KEYGEN</p>
            <h2>Generate key</h2>
          </div>
          <button className="icon-button" type="button" onClick={onClose} aria-label="Close key generator">
            <X size={17} aria-hidden="true" />
          </button>
        </header>
        <label>
          Algorithm
          <select
            value={value.algorithm}
            onChange={(event) =>
              onChange({
                ...value,
                algorithm: event.target.value as GenerationRequest['algorithm'],
                fileName: event.target.value === 'rsa' ? 'id_rsa' : 'id_ed25519',
                rsaBits: event.target.value === 'rsa' ? 3072 : null,
              })
            }
          >
            <option value="ed25519" disabled={existingAlgorithms.has('ed25519')}>
              Ed25519{existingAlgorithms.has('ed25519') ? ' (already exists)' : ''}
            </option>
            <option value="rsa" disabled={existingAlgorithms.has('rsa')}>
              RSA{existingAlgorithms.has('rsa') ? ' (already exists)' : ''}
            </option>
          </select>
        </label>
        {value.algorithm === 'rsa' && (
          <label>
            RSA bits
            <select
              value={value.rsaBits || 3072}
              onChange={(event) => onChange({ ...value, rsaBits: Number(event.target.value) })}
            >
              <option value="2048">2048</option>
              <option value="3072">3072</option>
              <option value="4096">4096</option>
            </select>
          </label>
        )}
        <label>
          Filename
          <input
            required
            value={value.fileName}
            onChange={(event) => onChange({ ...value, fileName: event.target.value })}
          />
        </label>
        <label>
          Comment <span className="optional">optional</span>
          <input
            maxLength={255}
            value={value.comment}
            onChange={(event) => onChange({ ...value, comment: event.target.value })}
          />
        </label>
        <div className="risk-warning">
          <KeyRound size={16} aria-hidden="true" /> Passphrase prompts appear only in the new key generation terminal.
        </div>
        <footer>
          <button className="text-button" type="button" onClick={onClose}>
            Cancel
          </button>
          <button className="primary" type="submit" disabled={busy}>
            <Play size={15} fill="currentColor" aria-hidden="true" /> {busy ? 'Starting...' : 'Open keygen terminal'}
          </button>
        </footer>
      </form>
    </Modal>
  );
}
