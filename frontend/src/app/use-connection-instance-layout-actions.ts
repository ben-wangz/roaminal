import { useCallback } from 'react';
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
  reorderConnectionGroup,
  type InstanceMovePlacement,
  type ConnectionInstanceLayout,
} from '../connections/connection-instance-groups';
import { ConnectionInstanceController } from '../connections/connection-instance-controller';
import { RoaminalApiError } from '../api/http-client';
import type { ToastKind } from '../ui/toast';

type Params = {
  controller: ConnectionInstanceController;
  showToast: (message: string, kind?: ToastKind) => void;
};

function authoritativeLayoutFromError(error: unknown): ConnectionInstanceLayout | null {
  if (!(error instanceof RoaminalApiError) || error.code !== 'connection_instance_layout_conflict') return null;
  const details = error.details;
  if (!details || typeof details !== 'object' || !('layout' in details)) return null;
  const layout = (details as { layout?: unknown }).layout;
  if (!layout || typeof layout !== 'object') return null;
  return layout as ConnectionInstanceLayout;
}

export function useConnectionInstanceLayoutActions({ controller, showToast }: Params) {
  const acceptConnectionInstanceLayout = useCallback((next: ConnectionInstanceLayout) => {
    controller.setLayout(next);
    controller.setConnections((current) => flattenConnectionInstanceLayout(next, current));
  }, [controller]);

  const persistConnectionInstanceLayout = useCallback(async (next: ConnectionInstanceLayout, previous: ConnectionInstanceLayout): Promise<boolean> => {
    controller.beginLayout(next);
    acceptConnectionInstanceLayout(next);
    try {
      const persisted = await saveConnectionInstanceLayout(next);
      controller.resolveLayout(persisted);
      acceptConnectionInstanceLayout(persisted);
      return true;
    } catch (err) {
      const authoritative = authoritativeLayoutFromError(err);
      const rollback = authoritative || previous;
      controller.rollbackLayout(rollback);
      acceptConnectionInstanceLayout(rollback);
      showToast((err as Error).message, 'error');
      return false;
    }
  }, [acceptConnectionInstanceLayout, controller, showToast]);

  const currentConnectionInstanceLayout = useCallback(() => controller.currentLayout(), [controller]);

  const moveConnectionInstanceToGroup = useCallback(async (
    id: string,
    groupId: string,
    targetId: string | null,
    placement: InstanceMovePlacement,
  ): Promise<boolean> => {
    const previous = currentConnectionInstanceLayout();
    const next = moveGroupedConnectionInstance(previous, id, groupId, targetId, placement);
    if (!next) {
      showToast('Group limit reached (10 connections).', 'error');
      return false;
    }
    return persistConnectionInstanceLayout(next, previous);
  }, [currentConnectionInstanceLayout, persistConnectionInstanceLayout, showToast]);

  const reorderConnectionInstanceGroup = useCallback(async (
    id: string,
    targetId: string,
    placement: InstanceMovePlacement,
  ): Promise<boolean> => {
    const previous = currentConnectionInstanceLayout();
    const next = reorderConnectionGroup(previous, id, targetId, placement);
    return next ? persistConnectionInstanceLayout(next, previous) : false;
  }, [currentConnectionInstanceLayout, persistConnectionInstanceLayout]);

  const createConnectionInstanceGroup = useCallback(async (name: string): Promise<boolean> => {
    const current = currentConnectionInstanceLayout();
    controller.markRevision();
    try {
      const next = await createConnectionInstanceGroupRequest(name, current.revision);
      acceptConnectionInstanceLayout(next);
      return true;
    } catch (err) {
      controller.markRevision();
      showToast((err as Error).message, 'error');
      return false;
    }
  }, [acceptConnectionInstanceLayout, controller, currentConnectionInstanceLayout, showToast]);

  const renameConnectionInstanceGroup = useCallback(async (id: string, name: string): Promise<boolean> => {
    const current = currentConnectionInstanceLayout();
    controller.markRevision();
    try {
      const next = await renameConnectionInstanceGroupRequest(id, name, current.revision);
      acceptConnectionInstanceLayout(next);
      return true;
    } catch (err) {
      controller.markRevision();
      showToast((err as Error).message, 'error');
      return false;
    }
  }, [acceptConnectionInstanceLayout, controller, currentConnectionInstanceLayout, showToast]);

  const deleteConnectionInstanceGroup = useCallback(async (id: string): Promise<boolean> => {
    const current = currentConnectionInstanceLayout();
    controller.markRevision();
    try {
      const next = await deleteConnectionInstanceGroupRequest(id, current.revision);
      acceptConnectionInstanceLayout(next);
      return true;
    } catch (err) {
      if ((err as Error).message === 'connection instance layout changed') {
        try {
          const latest = await loadConnectionInstanceLayout();
          const group = latest.groups.find((item) => item.groupId === id);
          if (group && group.connectionInstanceIds.length === 0) {
            const retried = await deleteConnectionInstanceGroupRequest(id, latest.revision);
            acceptConnectionInstanceLayout(retried);
            return true;
          }
        } catch {
          // Show the original error when the retry cannot run.
        }
      }
      controller.markRevision();
      showToast((err as Error).message, 'error');
      return false;
    }
  }, [acceptConnectionInstanceLayout, controller, currentConnectionInstanceLayout, showToast]);

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
