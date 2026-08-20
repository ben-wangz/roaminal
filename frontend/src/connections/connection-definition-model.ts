import type { ConnectionDefinition } from './connection-api';
import type { SSHKey } from './connection-api';

export type ConnectionDraft = {
  hostAlias: string;
  hostName: string;
  user: string;
  port: string;
  identities: string[];
  identitiesOnly: string;
  strictHostKeyChecking: string;
  userKnownHostsFile: string;
  serverAliveInterval: string;
  tmuxEnabled: boolean;
  tmuxSessionName: string;
  filesystemPwd: string;
};

export type ConnectionEditor = { mode: 'create' | 'edit'; definition?: ConnectionDefinition } | null;

export const emptyDraft: ConnectionDraft = {
  hostAlias: '',
  hostName: '',
  user: 'root',
  port: '22',
  identities: [],
  identitiesOnly: '',
  strictHostKeyChecking: '',
  userKnownHostsFile: '',
  serverAliveInterval: '15',
  tmuxEnabled: false,
  tmuxSessionName: '',
  filesystemPwd: '$HOME',
};

export function draftFrom(definition?: ConnectionDefinition, keys: SSHKey[] = []): ConnectionDraft {
  if (!definition) {
    const identities = keys
      .filter((key) => key.algorithm === 'ed25519' && key.status === 'available')
      .slice(0, 1)
      .map((key) => key.fileName);
    return { ...emptyDraft, identities, identitiesOnly: identities.length ? 'yes' : '' };
  }
  return {
    hostAlias: definition.hostAlias || '',
    hostName: definition.hostName || '',
    user: definition.user || '',
    port: definition.port ? String(definition.port) : '',
    identities: [...definition.identityFileNames],
    identitiesOnly: definition.identitiesOnly || '',
    strictHostKeyChecking: definition.strictHostKeyChecking || '',
    userKnownHostsFile: definition.userKnownHostsFile || '',
    serverAliveInterval: definition.serverAliveInterval ? String(definition.serverAliveInterval) : '',
    tmuxEnabled: Boolean(definition.tmux?.enabled),
    tmuxSessionName: definition.tmux?.sessionName || '',
    filesystemPwd: definition.filesystem?.pwd || '$HOME',
  };
}

export function bodyFrom(draft: ConnectionDraft): Record<string, unknown> {
  return {
    type: 'ssh',
    hostAlias: draft.hostAlias.trim(),
    hostName: draft.hostName.trim() || null,
    user: draft.user.trim() || null,
    port: draft.port ? Number(draft.port) : null,
    identityFileNames: draft.identities,
    identitiesOnly: draft.identitiesOnly || null,
    strictHostKeyChecking: draft.strictHostKeyChecking || null,
    userKnownHostsFile: draft.userKnownHostsFile || null,
    serverAliveInterval: draft.serverAliveInterval ? Number(draft.serverAliveInterval) : null,
    tmux: draft.tmuxEnabled
      ? { enabled: true, sessionName: draft.tmuxSessionName }
      : { enabled: false, sessionName: '' },
    filesystem: { pwd: draft.filesystemPwd.trim() || '$HOME' },
  };
}
