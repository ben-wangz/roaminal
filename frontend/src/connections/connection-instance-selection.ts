import type { ConnectionInstanceSummary } from '../terminal/terminal-protocol';

export function reusableInstanceForHost(instances: ConnectionInstanceSummary[], hostAlias: string): ConnectionInstanceSummary | null {
  return instances.find((instance) => instance.type === 'ssh' && instance.lifecycle === 'live' && instance.sourceState === 'current' && instance.sourceHostAlias === hostAlias) || null;
}
