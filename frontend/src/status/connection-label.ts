import type { ConnectionInstanceSummary } from '../terminal/terminal-protocol';

export function connectionDisplayName(active: ConnectionInstanceSummary | null, sessions: ConnectionInstanceSummary[]): string {
  if (!active) return 'Roaminal';
  const base = connectionBaseName(active);
  const peers = sessions.filter((session) => sameConnection(session, active)).sort(compareCreatedAt);
  if (peers.length < 2) return base;
  const index = peers.findIndex((session) => session.id === active.id);
  return `${base}-${index >= 0 ? index + 1 : 1}`;
}

function connectionBaseName(session: ConnectionInstanceSummary): string {
  if (session.type === 'ssh') return session.sourceHostAlias || 'Remote';
  if (session.purpose === 'ssh_key_generation') return session.title || 'SSH key generation';
  return 'Local';
}

function sameConnection(left: ConnectionInstanceSummary, right: ConnectionInstanceSummary): boolean {
  if (left.type !== right.type || left.purpose !== right.purpose) return false;
  if (left.connectionDefinitionId && right.connectionDefinitionId) return left.connectionDefinitionId === right.connectionDefinitionId;
  return left.sourceHostAlias === right.sourceHostAlias;
}

function compareCreatedAt(left: ConnectionInstanceSummary, right: ConnectionInstanceSummary): number {
  const leftTime = Date.parse(left.createdAt);
  const rightTime = Date.parse(right.createdAt);
  if (Number.isFinite(leftTime) && Number.isFinite(rightTime) && leftTime !== rightTime) return leftTime - rightTime;
  return left.id.localeCompare(right.id);
}
