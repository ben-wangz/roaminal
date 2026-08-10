import { api, apiWithMeta } from '../auth/auth-client';
import type { ConnectionInstanceSummary } from '../terminal/terminal-protocol';

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
