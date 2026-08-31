import { ShieldAlert, X } from 'lucide-react';
import { Modal } from '../ui/modal';
import type { SSHKey } from './connection-api';
import type { ConnectionDraft, ConnectionEditor } from './connection-definition-model';

type Props = {
  editor: Exclude<ConnectionEditor, null>;
  draft: ConnectionDraft;
  keys: SSHKey[];
  busy: boolean;
  optionsAvailable?: boolean;
  onDraft: (draft: ConnectionDraft) => void;
  onSave: (event: React.FormEvent) => void;
  onClose: () => void;
};

export function ConnectionDefinitionEditor({ editor, draft, keys, busy, optionsAvailable = true, onDraft, onSave, onClose }: Props) {
  const set = (key: keyof ConnectionDraft, value: string) => onDraft({ ...draft, [key]: value });
  return (
    <Modal onClose={onClose}>
      <form className="connection-editor" onSubmit={onSave}>
        <header>
          <div>
            <p className="eyebrow">STRUCTURED SSH CONFIG</p>
            <h2>{editor.mode === 'create' ? 'New connection' : 'Edit ' + (editor.definition?.hostAlias || '')}</h2>
          </div>
          <button className="icon-button" type="button" onClick={onClose} aria-label="Close editor">
            <X size={17} aria-hidden="true" />
          </button>
        </header>
        <label>
          Host alias
          <input
            id="connection-host-alias"
            name="hostAlias"
            required
            pattern={'[A-Za-z0-9][\\-A-Za-z0-9._]{0,254}'}
            value={draft.hostAlias}
            onChange={(event) => set('hostAlias', event.target.value)}
          />
        </label>
        <label>
          HostName
          <input
            id="connection-host-name"
            name="hostName"
            value={draft.hostName}
            onChange={(event) => set('hostName', event.target.value)}
            placeholder="destination hostname"
          />
        </label>
        <div className="form-grid">
          <label>
            User
            <input id="connection-user" name="user" value={draft.user} onChange={(event) => set('user', event.target.value)} />
          </label>
          <label>
            Port
            <input
              id="connection-port"
              name="port"
              type="number"
              min="1"
              max="65535"
              value={draft.port}
              onChange={(event) => set('port', event.target.value)}
            />
          </label>
        </div>
        <label>
          Identity files
          <div className="identity-options">
            {keys.map((key) => (
              <label key={key.keyId}>
                <input
                  id={`connection-identity-${key.keyId}`}
                  name="identityFile"
                  type="checkbox"
                  checked={draft.identities.includes(key.fileName)}
                  onChange={(event) =>
                    onDraft({
                      ...draft,
                      identities: event.target.checked
                        ? [...draft.identities, key.fileName]
                        : draft.identities.filter((name) => name !== key.fileName),
                    })
                  }
                />
                {key.fileName}
              </label>
            ))}
          </div>
        </label>
        <div className="form-grid">
          <label>
            IdentitiesOnly
            <select id="connection-identities-only" name="identitiesOnly" value={draft.identitiesOnly} onChange={(event) => set('identitiesOnly', event.target.value)}>
              <option value="">Unset</option>
              <option value="yes">yes</option>
              <option value="no">no</option>
            </select>
          </label>
          <label>
            ServerAliveInterval
            <input
              id="connection-server-alive-interval"
              name="serverAliveInterval"
              type="number"
              min="0"
              max="4294967295"
              value={draft.serverAliveInterval}
              onChange={(event) => set('serverAliveInterval', event.target.value)}
            />
          </label>
        </div>
        <div className="form-grid">
          <label>
            StrictHostKeyChecking
            <select
              id="connection-strict-host-key-checking"
              name="strictHostKeyChecking"
              value={draft.strictHostKeyChecking}
              onChange={(event) => set('strictHostKeyChecking', event.target.value)}
            >
              <option value="">default</option>
              <option value="no">no</option>
            </select>
          </label>
          <label>
            UserKnownHostsFile
            <select
              id="connection-user-known-hosts-file"
              name="userKnownHostsFile"
              value={draft.userKnownHostsFile}
              onChange={(event) => set('userKnownHostsFile', event.target.value)}
            >
              <option value="">default</option>
              <option value="/dev/null">/dev/null</option>
            </select>
          </label>
        </div>
        <details className="advanced-options">
          <summary>Advanced connection options</summary>
          {!optionsAvailable && <small className="field-help" role="alert">Roaminal tmux and FileSystem options are unavailable. Refresh the source before editing them.</small>}
          <label className="checkbox-row">
            <input
              id="connection-tmux-enabled"
              name="tmuxEnabled"
              disabled={!optionsAvailable}
              type="checkbox"
              checked={draft.tmuxEnabled}
              onChange={(event) => onDraft({ ...draft, tmuxEnabled: event.target.checked })}
            />{' '}
            Enable tmux connection
          </label>
          <label>
            Tmux session name
            <input
              id="connection-tmux-session-name"
              name="tmuxSessionName"
              disabled={!optionsAvailable || !draft.tmuxEnabled}
              required={draft.tmuxEnabled}
              pattern={'[A-Za-z][A-Za-z0-9_\\-]{0,63}'}
              maxLength={64}
              value={draft.tmuxSessionName}
              onChange={(event) => set('tmuxSessionName', event.target.value)}
              placeholder="for example t"
            />
          </label>
          <small className="field-help">The name is used by OpenSSH to attach or create the remote tmux session.</small>
          <label>
            FileSystem fallback pwd
            <input
              id="connection-filesystem-pwd"
              name="filesystemPwd"
              disabled={!optionsAvailable}
              required
              value={draft.filesystemPwd}
              onChange={(event) => set('filesystemPwd', event.target.value)}
              placeholder="$HOME"
            />
          </label>
          <small className="field-help">Used when the active tmux pane directory cannot be detected. The default is $HOME.</small>
        </details>
        {(draft.strictHostKeyChecking === 'no' || draft.userKnownHostsFile === '/dev/null') && (
          <div className="risk-warning" role="alert">
            <ShieldAlert size={16} aria-hidden="true" /> Host verification is weakened for this connection.
          </div>
        )}
        <footer>
          <button className="text-button" type="button" onClick={onClose}>
            Cancel
          </button>
          <button className="primary" type="submit" disabled={busy || !optionsAvailable}>
            {busy ? 'Saving...' : 'Save connection'}
          </button>
        </footer>
      </form>
    </Modal>
  );
}
