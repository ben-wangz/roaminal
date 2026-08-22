import { api, apiWithMeta } from '../auth/auth-client';
import type { ConnectionInstanceSummary } from '../terminal/terminal-protocol';
import type { ConnectionInstanceLayout } from './connection-instance-groups';

export type Warning = { directive: string; line: number; class: string };
export type ConnectionDefinition = {
  connectionDefinitionId: string;
  type: 'local' | 'ssh';
  hostAlias?: string;
  hostName: string | null;
  user: string | null;
  port: number | null;
  identityFileNames: string[];
  identitiesOnly: string | null;
  strictHostKeyChecking: string | null;
  userKnownHostsFile: string | null;
  serverAliveInterval: number | null;
  advancedDirectiveCount: number;
  unmanagedIdentityCount: number;
  warnings: Warning[];
  capabilities: Record<string, boolean>;
  hostVerificationAssessment: 'default' | 'weakened' | 'unknown';
  tmux?: { enabled: boolean; sessionName: string };
  filesystem?: { pwd: string };
};
export type ConfigSource = { status: string; readable: boolean; writable: boolean; warnings?: Warning[]; blockers?: string[]; reason?: string };
export type DefinitionCollection = { configSource: ConfigSource; tmuxOptionsSource?: ConfigSource; definitions: ConnectionDefinition[] };
export type SSHKey = { keyId: string; fileName: string; algorithm: string; bits: number; fingerprint: string; publicKeyAvailable: boolean; readOnly: boolean; status: string };
export type KeyCollection = { keys: SSHKey[] };
export type GenerationRequest = { algorithm: 'ed25519' | 'rsa'; rsaBits: number | null; fileName: string; comment: string };

export async function loadDefinitions(): Promise<{ data: DefinitionCollection; etag: string | null }> {
  return apiWithMeta<DefinitionCollection>('/api/connection-definitions');
}
export async function loadKeys(): Promise<KeyCollection> { return api<KeyCollection>('/api/ssh-keys'); }
export async function createDefinition(body: Partial<ConnectionDefinition>, etag: string): Promise<{ data: DefinitionCollection; etag: string | null }> {
  return apiWithMeta<DefinitionCollection>('/api/connection-definitions', { method: 'POST', headers: { 'If-Match': etag }, body: JSON.stringify(body) });
}
export async function updateDefinition(id: string, body: Partial<ConnectionDefinition>, etag: string): Promise<{ data: DefinitionCollection; etag: string | null }> {
  return apiWithMeta<DefinitionCollection>(`/api/connection-definitions/${encodeURIComponent(id)}`, { method: 'PUT', headers: { 'If-Match': etag }, body: JSON.stringify(body) });
}
export async function duplicateDefinition(id: string, hostAlias: string, etag: string): Promise<{ data: DefinitionCollection; etag: string | null }> {
  return apiWithMeta<DefinitionCollection>(`/api/connection-definitions/${encodeURIComponent(id)}/duplicate`, { method: 'POST', headers: { 'If-Match': etag }, body: JSON.stringify({ hostAlias }) });
}
export async function deleteDefinition(id: string, etag: string): Promise<{ data: DefinitionCollection; etag: string | null }> {
  return apiWithMeta<DefinitionCollection>(`/api/connection-definitions/${encodeURIComponent(id)}`, { method: 'DELETE', headers: { 'If-Match': etag } });
}
export async function startConnectionLaunch(connectionDefinitionId: string, reuseFromConnectionInstanceId?: string): Promise<{ launchId: string; connectionDefinitionId: string; lifecycle: 'pending'; tmuxSessionName: string }> {
  return api('/api/connection-launches', { method: 'POST', body: JSON.stringify({ connectionDefinitionId, reuseFromConnectionInstanceId: reuseFromConnectionInstanceId || null }) });
}

export async function saveConnectionInstanceOrder(connectionInstanceIds: string[]): Promise<ConnectionInstanceSummary[]> {
  const result = await api<{ connectionInstances: ConnectionInstanceSummary[] }>('/api/connection-instances/order', {
    method: 'PUT',
    body: JSON.stringify({ connectionInstanceIds }),
  });
  return result.connectionInstances;
}

export async function loadConnectionInstanceLayout(): Promise<ConnectionInstanceLayout> {
  const result = await api<{ layout: ConnectionInstanceLayout }>('/api/connection-instance-groups');
  return result.layout;
}

export async function createConnectionInstanceGroup(name: string, revision: number): Promise<ConnectionInstanceLayout> {
  const result = await api<{ layout: ConnectionInstanceLayout }>('/api/connection-instance-groups', {
    method: 'POST',
    body: JSON.stringify({ name, revision }),
  });
  return result.layout;
}

export async function renameConnectionInstanceGroup(groupId: string, name: string, revision: number): Promise<ConnectionInstanceLayout> {
  const result = await api<{ layout: ConnectionInstanceLayout }>(`/api/connection-instance-groups/${encodeURIComponent(groupId)}`, {
    method: 'PATCH',
    body: JSON.stringify({ name, revision }),
  });
  return result.layout;
}

export async function deleteConnectionInstanceGroup(groupId: string, revision: number): Promise<ConnectionInstanceLayout> {
  const result = await api<{ layout: ConnectionInstanceLayout }>(`/api/connection-instance-groups/${encodeURIComponent(groupId)}`, {
    method: 'DELETE',
    body: JSON.stringify({ revision }),
  });
  return result.layout;
}

export async function saveConnectionInstanceLayout(layout: ConnectionInstanceLayout): Promise<ConnectionInstanceLayout> {
  const result = await api<{ layout: ConnectionInstanceLayout }>('/api/connection-instance-groups/layout', {
    method: 'PUT',
    body: JSON.stringify(layout),
  });
  return result.layout;
}

// A page refresh can happen before the launch websocket completes its
// handshake. Keep the cancellation request small and allow the browser to
// finish it while the document is being unloaded.
export function abortConnectionLaunch(id: string, auth: { accessToken: string } | null = null): void {
  const token = auth?.accessToken;
  if (!token) return;
  void fetch(`/api/connection-launches/${encodeURIComponent(id)}`, {
    method: 'DELETE',
    headers: { Authorization: `Bearer ${token}` },
    keepalive: true,
  }).catch(() => undefined);
}

export async function generateKey(body: GenerationRequest): Promise<ConnectionInstanceSummary> {
  return api<ConnectionInstanceSummary>('/api/ssh-key-generations', { method: 'POST', body: JSON.stringify(body) });
}
export async function publicKey(id: string): Promise<string> {
  const result = await api<{ publicKey: string }>(`/api/ssh-keys/${encodeURIComponent(id)}/public-key`);
  return result.publicKey;
}
export async function deleteKey(id: string): Promise<void> {
  await api<void>(`/api/ssh-keys/${encodeURIComponent(id)}`, { method: 'DELETE' });
}
