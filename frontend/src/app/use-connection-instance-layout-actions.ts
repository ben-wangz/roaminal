import { useCallback, type Dispatch, type MutableRefObject, type SetStateAction } from 'react';
import {
  createConnectionInstanceGroup as createConnectionInstanceGroupRequest,
  deleteConnectionInstanceGroup as deleteConnectionInstanceGroupRequest,
  loadConnectionInstanceLayout,
  renameConnectionInstanceGroup as renameConnectionInstanceGroupRequest,
  saveConnectionInstanceLayout,
} from '../connections/connection-api';
import {
  flattenConnectionInstanceLayout,
  moveGroupMembersToUngrouped as moveGroupMembersToUngroupedModel,
  moveConnectionInstance as moveGroupedConnectionInstance,
  normalizeConnectionInstanceLayout,
  reorderConnectionGroup,
  type ConnectionInstanceLayout,
  type InstanceMovePlacement,
} from '../connections/connection-instance-groups';
import type { ConnectionInstanceSummary } from '../terminal/terminal-protocol';
import type { ToastKind } from '../ui/toast';

type Params = {
  connections: ConnectionInstanceSummary[];
  setConnections: Dispatch<SetStateAction<ConnectionInstanceSummary[]>>;
  setConnectionInstanceLayout: Dispatch<SetStateAction<ConnectionInstanceLayout | null>>;
  connectionInstanceLayoutRef: MutableRefObject<ConnectionInstanceLayout | null>;
  pendingConnectionInstanceLayout: MutableRefObject<ConnectionInstanceLayout | null>;
  connectionOrder: MutableRefObject<string[]>;
  stateRevision: MutableRefObject<number>;
  showToast: (message: string, kind?: ToastKind) => void;
};

export function useConnectionInstanceLayoutActions({
  connections,
  setConnections,
  setConnectionInstanceLayout,
  connectionInstanceLayoutRef,
  pendingConnectionInstanceLayout,
  connectionOrder,
  stateRevision,
  showToast,
}: Params) {
  const acceptConnectionInstanceLayout = useCallback((next: ConnectionInstanceLayout) => {
    connectionInstanceLayoutRef.current = next;
    setConnectionInstanceLayout(next);
    setConnections((current) => {
      const ordered = flattenConnectionInstanceLayout(next, current);
      connectionOrder.current = ordered.map((connection) => connection.connectionInstanceId);
      return ordered;
    });
  }, [connectionInstanceLayoutRef, connectionOrder, setConnectionInstanceLayout, setConnections]);

  const persistConnectionInstanceLayout = useCallback(async (next: ConnectionInstanceLayout, previous: ConnectionInstanceLayout) => {
    stateRevision.current += 1;
    pendingConnectionInstanceLayout.current = next;
    acceptConnectionInstanceLayout(next);
    try {
      const persisted = await saveConnectionInstanceLayout(next);
      stateRevision.current += 1;
      pendingConnectionInstanceLayout.current = null;
      acceptConnectionInstanceLayout(persisted);
    } catch (err) {
      stateRevision.current += 1;
      pendingConnectionInstanceLayout.current = null;
      acceptConnectionInstanceLayout(previous);
      showToast((err as Error).message, 'error');
    }
  }, [acceptConnectionInstanceLayout, pendingConnectionInstanceLayout, showToast, stateRevision]);

  const currentConnectionInstanceLayout = useCallback(
    () => normalizeConnectionInstanceLayout(connectionInstanceLayoutRef.current, connections),
    [connectionInstanceLayoutRef, connections],
  );

  const moveConnectionInstanceToGroup = useCallback(async (
    id: string,
    groupId: string,
    targetId: string | null,
    placement: InstanceMovePlacement,
  ) => {
    const previous = currentConnectionInstanceLayout();
    const next = moveGroupedConnectionInstance(previous, id, groupId, targetId, placement);
    if (!next) {
      showToast('Group limit reached (10 connections).', 'error');
      return;
    }
    await persistConnectionInstanceLayout(next, previous);
  }, [currentConnectionInstanceLayout, persistConnectionInstanceLayout, showToast]);

  const reorderConnectionInstanceGroup = useCallback(async (
    id: string,
    targetId: string,
    placement: InstanceMovePlacement,
  ) => {
    const previous = currentConnectionInstanceLayout();
    const next = reorderConnectionGroup(previous, id, targetId, placement);
    if (next) await persistConnectionInstanceLayout(next, previous);
  }, [currentConnectionInstanceLayout, persistConnectionInstanceLayout]);

  const createConnectionInstanceGroup = useCallback(async (name: string): Promise<boolean> => {
    const current = currentConnectionInstanceLayout();
    stateRevision.current += 1;
    try {
      const next = await createConnectionInstanceGroupRequest(name, current.revision);
      stateRevision.current += 1;
      acceptConnectionInstanceLayout(next);
      return true;
    } catch (err) {
      stateRevision.current += 1;
      showToast((err as Error).message, 'error');
      return false;
    }
  }, [acceptConnectionInstanceLayout, currentConnectionInstanceLayout, showToast, stateRevision]);

  const renameConnectionInstanceGroup = useCallback(async (id: string, name: string): Promise<boolean> => {
    const current = currentConnectionInstanceLayout();
    stateRevision.current += 1;
    try {
      const next = await renameConnectionInstanceGroupRequest(id, name, current.revision);
      stateRevision.current += 1;
      acceptConnectionInstanceLayout(next);
      return true;
    } catch (err) {
      stateRevision.current += 1;
      showToast((err as Error).message, 'error');
      return false;
    }
  }, [acceptConnectionInstanceLayout, currentConnectionInstanceLayout, showToast, stateRevision]);

  const deleteConnectionInstanceGroup = useCallback(async (id: string): Promise<boolean> => {
    const current = currentConnectionInstanceLayout();
    stateRevision.current += 1;
    try {
      const next = await deleteConnectionInstanceGroupRequest(id, current.revision);
      stateRevision.current += 1;
      acceptConnectionInstanceLayout(next);
      return true;
    } catch (err) {
      // Heartbeat may add an instance between render and this request. Retry
      // only when the latest server layout still shows an empty group.
      if ((err as Error).message === 'connection instance layout changed') {
        try {
          const latest = await loadConnectionInstanceLayout();
          const group = latest.groups.find((item) => item.groupId === id);
          if (group && group.connectionInstanceIds.length === 0) {
            const retried = await deleteConnectionInstanceGroupRequest(id, latest.revision);
            stateRevision.current += 1;
            acceptConnectionInstanceLayout(retried);
            return true;
          }
        } catch {
          // Show the original error when the retry cannot run.
        }
      }
      stateRevision.current += 1;
      showToast((err as Error).message, 'error');
      return false;
    }
  }, [acceptConnectionInstanceLayout, currentConnectionInstanceLayout, showToast, stateRevision]);

  const moveGroupMembersToUngrouped = useCallback(async (id: string) => {
    const previous = currentConnectionInstanceLayout();
    const next = moveGroupMembersToUngroupedModel(previous, id);
    if (next) await persistConnectionInstanceLayout(next, previous);
  }, [currentConnectionInstanceLayout, persistConnectionInstanceLayout]);

  return {
    moveConnectionInstanceToGroup,
    reorderConnectionInstanceGroup,
    createConnectionInstanceGroup,
    renameConnectionInstanceGroup,
    deleteConnectionInstanceGroup,
    moveGroupMembersToUngrouped,
  };
}
