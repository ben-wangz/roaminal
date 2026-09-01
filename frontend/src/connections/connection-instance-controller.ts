import { useRef, useSyncExternalStore } from 'react';
import type { Heartbeat } from '../status/heartbeat';
import {
  flattenConnectionInstanceLayout,
  normalizeConnectionInstanceLayout,
  type ConnectionInstanceLayout,
} from './connection-instance-groups';
import type { ConnectionInstanceSummary } from '../terminal/terminal-protocol';
import { orderConnectionInstances, reconcileConnections, type ConnectionView } from '../app/connection-view';
import { defaultContextualMode, type ContextualMode } from '../input/contextual-keyboard-model';

export type ConnectionHeartbeatReconciliation = {
  serverLayout: ConnectionInstanceLayout;
  effectiveConnections: ConnectionInstanceSummary[];
  activeView: ConnectionView;
  order: string[];
};

export type ConnectionInstanceControllerState = {
  connections: ConnectionInstanceSummary[];
  layout: ConnectionInstanceLayout | null;
  heartbeat: Heartbeat | null;
  heartbeatLatency: number | null;
  heartbeatConnected: boolean;
  order: string[];
  pendingOrder: string[] | null;
  pendingLayout: ConnectionInstanceLayout | null;
  revision: number;
  hydrated: boolean;
  bootId: string | null;
  syncing: boolean;
  contextualModes: Record<string, ContextualMode>;
};

type Listener = () => void;

const initialConnectionInstanceControllerState: ConnectionInstanceControllerState = {
  connections: [],
  layout: null,
  heartbeat: null,
  heartbeatLatency: null,
  heartbeatConnected: true,
  order: [],
  pendingOrder: null,
  pendingLayout: null,
  revision: 0,
  hydrated: false,
  bootId: null,
  syncing: false,
  contextualModes: {},
};

export class ConnectionInstanceController {
  private state: ConnectionInstanceControllerState = initialConnectionInstanceControllerState;
  private readonly listeners = new Set<Listener>();

  getSnapshot = (): ConnectionInstanceControllerState => this.state;

  subscribe = (listener: Listener): (() => void) => {
    this.listeners.add(listener);
    return () => this.listeners.delete(listener);
  };

  private update(update: (current: ConnectionInstanceControllerState) => ConnectionInstanceControllerState): void {
    const next = update(this.state);
    if (next === this.state) return;
    this.state = next;
    for (const listener of this.listeners) listener();
  }

  setConnections(next: ConnectionInstanceSummary[] | ((current: ConnectionInstanceSummary[]) => ConnectionInstanceSummary[])): void {
    this.update((current) => {
      const connections = typeof next === 'function' ? next(current.connections) : next;
      if (connections === current.connections) return current;
      return {
        ...current,
        connections,
        order: current.pendingOrder ? current.order : connections.map((item) => item.connectionInstanceId),
      };
    });
  }

  setLayout(layout: ConnectionInstanceLayout | null): void {
    this.update((current) => current.layout === layout ? current : { ...current, layout });
  }

  setHeartbeat(heartbeat: Heartbeat | null): void {
    this.update((current) => current.heartbeat === heartbeat ? current : { ...current, heartbeat });
  }

  setHeartbeatLatency(heartbeatLatency: number | null): void {
    this.update((current) => current.heartbeatLatency === heartbeatLatency ? current : { ...current, heartbeatLatency });
  }

  setHeartbeatConnected(heartbeatConnected: boolean): void {
    this.update((current) => current.heartbeatConnected === heartbeatConnected ? current : { ...current, heartbeatConnected });
  }

  contextualMode(instance: ConnectionInstanceSummary | null): ContextualMode {
    if (!instance) return 'codex';
    return this.state.contextualModes[instance.connectionInstanceId] || defaultContextualMode(instance);
  }

  setContextualMode(instance: ConnectionInstanceSummary | null, mode: ContextualMode): void {
    if (!instance) return;
    this.update((current) => {
      if (current.contextualModes[instance.connectionInstanceId] === mode) return current;
      return {
        ...current,
        contextualModes: { ...current.contextualModes, [instance.connectionInstanceId]: mode },
      };
    });
  }

  markRevision(): void { this.update((current) => ({ ...current, revision: current.revision + 1 })); }

  beginOrder(order: string[]): void {
    this.update((current) => ({ ...current, revision: current.revision + 1, pendingOrder: [...order], order: [...order] }));
  }

  resolveOrder(connections: ConnectionInstanceSummary[]): void {
    this.update((current) => ({
      ...current,
      revision: current.revision + 1,
      pendingOrder: null,
      order: connections.map((item) => item.connectionInstanceId),
      connections,
    }));
  }

  rollbackOrder(connections: ConnectionInstanceSummary[]): void {
    this.update((current) => ({
      ...current,
      revision: current.revision + 1,
      pendingOrder: null,
      order: connections.map((item) => item.connectionInstanceId),
      connections,
    }));
  }

  beginLayout(layout: ConnectionInstanceLayout): void {
    this.update((current) => ({ ...current, revision: current.revision + 1, pendingLayout: layout, layout }));
  }

  resolveLayout(layout: ConnectionInstanceLayout): void {
    this.update((current) => ({ ...current, revision: current.revision + 1, pendingLayout: null, layout }));
  }

  rollbackLayout(layout: ConnectionInstanceLayout): void {
    this.update((current) => ({ ...current, revision: current.revision + 1, pendingLayout: null, layout }));
  }

  currentLayout(): ConnectionInstanceLayout {
    return normalizeConnectionInstanceLayout(this.state.layout, this.state.connections);
  }

  beginSync(): boolean {
    if (this.state.syncing) return false;
    this.update((current) => ({ ...current, syncing: true }));
    return true;
  }

  endSync(): void { this.update((current) => ({ ...current, syncing: false })); }
  setBootId(bootId: string | null): void { this.update((current) => ({ ...current, bootId })); }
  markHydrated(): void { this.update((current) => ({ ...current, hydrated: true })); }

  applyHeartbeat(next: Heartbeat, currentView: ConnectionView): ConnectionHeartbeatReconciliation {
    const current = this.state;
    const reconciled = reconcileConnectionHeartbeat({
      heartbeat: next,
      currentView,
      previousOrder: current.order,
      pendingOrder: current.pendingOrder,
      pendingLayout: current.pendingLayout,
    });
    const effectiveLayout = current.pendingLayout || reconciled.serverLayout;
    this.update(() => ({
      ...this.state,
      heartbeat: this.state.heartbeat && sameHeartbeat(this.state.heartbeat, next) ? this.state.heartbeat : next,
      layout: effectiveLayout,
      connections: sameConnectionSummaries(this.state.connections, reconciled.effectiveConnections)
        ? this.state.connections
        : reconciled.effectiveConnections,
      order: reconciled.order,
    }));
    return reconciled;
  }

  reset(): void {
    this.update(() => ({ ...initialConnectionInstanceControllerState }));
  }
}

export function useConnectionInstanceController(): {
  controller: ConnectionInstanceController;
  state: ConnectionInstanceControllerState;
} {
  const controllerRef = useRef<ConnectionInstanceController | null>(null);
  if (!controllerRef.current) controllerRef.current = new ConnectionInstanceController();
  const controller = controllerRef.current;
  const state = useSyncExternalStore(controller.subscribe, controller.getSnapshot, controller.getSnapshot);
  return { controller, state };
}

export function sameConnectionSummaries(left: ConnectionInstanceSummary[], right: ConnectionInstanceSummary[]): boolean {
  if (left === right) return true;
  if (left.length !== right.length) return false;
  for (let index = 0; index < left.length; index += 1) {
    if (!sameConnectionSummary(left[index], right[index])) return false;
  }
  return true;
}

export function sameHeartbeat(left: Heartbeat, right: Heartbeat): boolean {
  return sameConnectionSummaries(left.connectionInstances, right.connectionInstances)
    && sameLayout(left.connectionInstanceLayout, right.connectionInstanceLayout)
    && left.runtime.bootId === right.runtime.bootId
    && left.runtime.persistenceDegraded === right.runtime.persistenceDegraded
    && left.runtime.scrollbackLines === right.runtime.scrollbackLines
    && (left.messageState?.revision || 0) === (right.messageState?.revision || 0)
    && (left.messageState?.latestSequence || 0) === (right.messageState?.latestSequence || 0)
    && (left.messageState?.unreadCount || 0) === (right.messageState?.unreadCount || 0)
    && sameSystem(left.system, right.system);
}

function sameConnectionSummary(left: ConnectionInstanceSummary, right: ConnectionInstanceSummary): boolean {
  const scalarKeys: (keyof ConnectionInstanceSummary)[] = [
    'connectionInstanceId', 'connectionDefinitionId', 'type', 'purpose', 'lifecycle', 'sourceState',
    'sourceHostAlias', 'createdAt', 'updatedAt', 'title', 'titleMode', 'cwd', 'cols', 'rows',
    'attention', 'generationStatus', 'generationError', 'terminalType', 'tmuxEnabled', 'tmuxSessionName', 'tmuxPrefixKey', 'tmuxPrefixSource',
  ];
  if (scalarKeys.some((key) => left[key] !== right[key])) return false;
  if (left.endpoint?.user !== right.endpoint?.user || left.endpoint?.host !== right.endpoint?.host || left.endpoint?.port !== right.endpoint?.port) return false;
  if (left.remoteCapability?.status !== right.remoteCapability?.status
    || left.remoteCapability?.retryable !== right.remoteCapability?.retryable
    || left.remoteCapability?.reason !== right.remoteCapability?.reason) return false;
  const a = left.agent;
  const b = right.agent;
  if (a === b) return true;
  if (!a || !b) return false;
  return a.agentType === b.agentType && a.support === b.support && a.supportReason === b.supportReason
    && a.component === b.component && a.componentVersion === b.componentVersion && a.activity === b.activity
    && a.activityLabel === b.activityLabel && a.lastEventName === b.lastEventName && a.lastEventAt === b.lastEventAt
    && a.initializationId === b.initializationId && a.errorCode === b.errorCode && a.errorMessage === b.errorMessage
    && a.state === b.state && a.stateLabel === b.stateLabel && a.stateIndex === b.stateIndex
    && a.stateUpdatedAt === b.stateUpdatedAt && a.syncStatus === b.syncStatus && a.lastSyncedAt === b.lastSyncedAt
    && a.syncError === b.syncError;
}

function sameLayout(left: ConnectionInstanceLayout, right: ConnectionInstanceLayout): boolean {
  if (left.revision !== right.revision || left.groupOrder.length !== right.groupOrder.length || left.groups.length !== right.groups.length || left.ungroupedConnectionInstanceIds.length !== right.ungroupedConnectionInstanceIds.length) return false;
  if (left.groupOrder.some((id, index) => id !== right.groupOrder[index]) || left.ungroupedConnectionInstanceIds.some((id, index) => id !== right.ungroupedConnectionInstanceIds[index])) return false;
  return left.groups.every((group, index) => {
    const other = right.groups[index];
    return group.groupId === other.groupId && group.name === other.name && group.connectionInstanceIds.length === other.connectionInstanceIds.length
      && group.connectionInstanceIds.every((id, memberIndex) => id === other.connectionInstanceIds[memberIndex]);
  });
}

function sameSystem(left: Heartbeat['system'], right: Heartbeat['system']): boolean {
  return left.hostname === right.hostname && left.kernel === right.kernel && left.ip === right.ip
    && left.resourceScope === right.resourceScope && left.resourcesAvailable === right.resourcesAvailable
    && left.processUptimeSeconds === right.processUptimeSeconds && sameMetric(left.cpu, right.cpu) && sameMetric(left.memory, right.memory);
}

function sameMetric(left: Record<string, unknown>, right: Record<string, unknown>): boolean {
  const keys = new Set([...Object.keys(left), ...Object.keys(right)]);
  for (const key of keys) if (left[key] !== right[key]) return false;
  return true;
}

export function reconcileConnectionHeartbeat({
  heartbeat,
  currentView,
  previousOrder,
  pendingOrder,
  pendingLayout,
}: {
  heartbeat: { connectionInstances: ConnectionInstanceSummary[]; connectionInstanceLayout: ConnectionInstanceLayout | null };
  currentView: ConnectionView;
  previousOrder: string[];
  pendingOrder: string[] | null;
  pendingLayout: ConnectionInstanceLayout | null;
}): ConnectionHeartbeatReconciliation {
  const serverLayout = normalizeConnectionInstanceLayout(heartbeat.connectionInstanceLayout, heartbeat.connectionInstances);
  const effectiveLayout = pendingLayout
    ? normalizeConnectionInstanceLayout(pendingLayout, heartbeat.connectionInstances)
    : serverLayout;
  const layoutConnections = flattenConnectionInstanceLayout(effectiveLayout, heartbeat.connectionInstances);
  const effectiveConnections = pendingOrder
    ? orderConnectionInstances(layoutConnections, pendingOrder)
    : layoutConnections;
  const activeView = reconcileConnections(effectiveConnections, currentView, previousOrder);
  return {
    serverLayout,
    effectiveConnections,
    activeView,
    order: effectiveConnections.map((connection) => connection.connectionInstanceId),
  };
}
