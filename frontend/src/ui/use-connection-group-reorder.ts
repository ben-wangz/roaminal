import { useCallback, useRef, useState, type DragEvent, type KeyboardEvent } from 'react';
import type { ConnectionInstanceLayout, InstanceMovePlacement } from '../connections/connection-instance-groups';

export type GroupDropTarget = { kind: 'instance' | 'group'; id: string; groupId: string; placement: InstanceMovePlacement };

type Props = {
  layout: ConnectionInstanceLayout;
  disabled?: boolean;
  onMoveInstance: (id: string, groupId: string, targetId: string | null, placement: InstanceMovePlacement) => Promise<void>;
  onReorderGroup: (id: string, targetId: string, placement: InstanceMovePlacement) => Promise<void>;
  onPreviewEnd: (id: string) => void;
  onExpandGroup?: (id: string) => void;
};

function placementFor(event: DragEvent<HTMLElement>): InstanceMovePlacement {
  const bounds = event.currentTarget.getBoundingClientRect();
  return event.clientY < bounds.top + bounds.height / 2 ? 'before' : 'after';
}

export function useConnectionGroupReorder({ layout, disabled = false, onMoveInstance, onReorderGroup, onPreviewEnd, onExpandGroup }: Props) {
  const [draggedConnectionInstanceId, setDraggedConnectionInstanceId] = useState<string | null>(null);
  const [draggedGroupId, setDraggedGroupId] = useState<string | null>(null);
  const [dropTarget, setDropTarget] = useState<GroupDropTarget | null>(null);
  const [capacityBlockedGroupId, setCapacityBlockedGroupId] = useState<string | null>(null);
  const [reorderPending, setReorderPending] = useState(false);
  const groupHoverTimer = useRef<number | null>(null);
  const groupHoverId = useRef<string | null>(null);

  const clearGroupHover = useCallback(() => {
    if (groupHoverTimer.current !== null) window.clearTimeout(groupHoverTimer.current);
    groupHoverTimer.current = null;
    groupHoverId.current = null;
  }, []);

  const scheduleGroupExpansion = useCallback((groupId: string) => {
    if (!onExpandGroup || groupHoverId.current === groupId) return;
    clearGroupHover();
    groupHoverId.current = groupId;
    groupHoverTimer.current = window.setTimeout(() => {
      groupHoverTimer.current = null;
      onExpandGroup(groupId);
    }, 600);
  }, [clearGroupHover, onExpandGroup]);

  const clearDrag = useCallback(() => {
    clearGroupHover();
    setDraggedConnectionInstanceId(null);
    setDraggedGroupId(null);
    setDropTarget(null);
    setCapacityBlockedGroupId(null);
  }, [clearGroupHover]);

  const groupIsFull = useCallback((sourceInstanceId: string | null, targetGroupId: string) => {
    if (!sourceInstanceId || targetGroupId === 'ungrouped') return false;
    const sourceGroupId = layout.ungroupedConnectionInstanceIds.includes(sourceInstanceId)
      ? 'ungrouped'
      : layout.groups.find((group) => group.connectionInstanceIds.includes(sourceInstanceId))?.groupId;
    if (!sourceGroupId || sourceGroupId === targetGroupId) return false;
    const target = layout.groups.find((group) => group.groupId === targetGroupId);
    return Boolean(target && target.connectionInstanceIds.length >= 10);
  }, [layout]);

  const submit = useCallback((action: () => Promise<void>) => {
    if (reorderPending) return;
    clearDrag();
    setReorderPending(true);
    void action().catch(() => undefined).finally(() => setReorderPending(false));
  }, [clearDrag, reorderPending]);

  const dragOverInstance = useCallback((event: DragEvent<HTMLElement>, id: string, groupId: string) => {
    if (disabled || !draggedConnectionInstanceId || draggedConnectionInstanceId === id) return;
    event.preventDefault();
    event.stopPropagation();
    const blocked = groupIsFull(draggedConnectionInstanceId, groupId);
    setCapacityBlockedGroupId(blocked ? groupId : null);
    event.dataTransfer.dropEffect = blocked ? 'none' : 'move';
    const placement = placementFor(event);
    setDropTarget((current) => current?.kind === 'instance' && current.id === id && current.placement === placement ? current : { kind: 'instance', id, groupId, placement });
  }, [disabled, draggedConnectionInstanceId, groupIsFull]);

  const dragOverGroup = useCallback((event: DragEvent<HTMLElement>, groupId: string) => {
    if (disabled || !draggedConnectionInstanceId && !draggedGroupId) return;
    if (draggedGroupId === groupId) return;
    event.preventDefault();
    event.stopPropagation();
    const blocked = groupIsFull(draggedConnectionInstanceId, groupId);
    setCapacityBlockedGroupId(blocked ? groupId : null);
    event.dataTransfer.dropEffect = blocked ? 'none' : 'move';
    scheduleGroupExpansion(groupId);
    const placement = placementFor(event);
    setDropTarget((current) => current?.kind === 'group' && current.id === groupId && current.placement === placement ? current : { kind: 'group', id: groupId, groupId, placement });
  }, [disabled, draggedConnectionInstanceId, draggedGroupId, groupIsFull, scheduleGroupExpansion]);

  const dropInstance = useCallback((event: DragEvent<HTMLElement>, id: string, groupId: string) => {
    event.preventDefault();
    event.stopPropagation();
    if (disabled) return clearDrag();
    if (capacityBlockedGroupId === groupId) return clearDrag();
    const draggedId = draggedConnectionInstanceId || event.dataTransfer.getData('text/plain').replace(/^instance:/, '');
    if (!draggedId || draggedId === id) return clearDrag();
    submit(() => onMoveInstance(draggedId, groupId, id, placementFor(event)));
  }, [capacityBlockedGroupId, clearDrag, disabled, draggedConnectionInstanceId, onMoveInstance, submit]);

  const dropGroup = useCallback((event: DragEvent<HTMLElement>, groupId: string) => {
    if (disabled) return clearDrag();
    event.preventDefault();
    event.stopPropagation();
    if (capacityBlockedGroupId === groupId) return clearDrag();
    if (draggedGroupId) {
      if (draggedGroupId !== groupId) submit(() => onReorderGroup(draggedGroupId, groupId, placementFor(event)));
      else clearDrag();
      return;
    }
    const draggedId = draggedConnectionInstanceId || event.dataTransfer.getData('text/plain').replace(/^instance:/, '');
    if (draggedId) submit(() => onMoveInstance(draggedId, groupId, null, 'after'));
  }, [capacityBlockedGroupId, clearDrag, disabled, draggedConnectionInstanceId, draggedGroupId, onMoveInstance, onReorderGroup, submit]);

  const startInstanceDrag = useCallback((event: DragEvent<HTMLElement>, id: string) => {
    if (disabled || reorderPending) {
      event.preventDefault();
      return;
    }
    event.dataTransfer.effectAllowed = 'move';
    event.dataTransfer.setData('text/plain', `instance:${id}`);
    setDraggedConnectionInstanceId(id);
    setDraggedGroupId(null);
    setDropTarget(null);
    onPreviewEnd(id);
  }, [disabled, onPreviewEnd, reorderPending]);

  const startGroupDrag = useCallback((event: DragEvent<HTMLElement>, id: string) => {
    if (disabled || reorderPending) {
      event.preventDefault();
      return;
    }
    event.dataTransfer.effectAllowed = 'move';
    event.dataTransfer.setData('text/plain', `group:${id}`);
    setDraggedGroupId(id);
    setDraggedConnectionInstanceId(null);
    setDropTarget(null);
  }, [disabled, reorderPending]);

  const moveInstanceWithKeyboard = useCallback((event: KeyboardEvent<HTMLElement>, id: string, groupId: string) => {
    if (disabled || reorderPending || (event.key !== 'ArrowUp' && event.key !== 'ArrowDown')) return;
    const members = groupId === 'ungrouped' ? layout.ungroupedConnectionInstanceIds : layout.groups.find((group) => group.groupId === groupId)?.connectionInstanceIds || [];
    const index = members.indexOf(id);
    const target = members[index + (event.key === 'ArrowUp' ? -1 : 1)];
    if (!target) return;
    event.preventDefault();
    submit(() => onMoveInstance(id, groupId, target, event.key === 'ArrowUp' ? 'before' : 'after'));
  }, [disabled, layout, onMoveInstance, reorderPending, submit]);

  const moveGroupWithKeyboard = useCallback((event: KeyboardEvent<HTMLElement>, id: string) => {
    if (disabled || reorderPending || (event.key !== 'ArrowLeft' && event.key !== 'ArrowRight')) return;
    const index = layout.groupOrder.indexOf(id);
    const target = layout.groupOrder[index + (event.key === 'ArrowLeft' ? -1 : 1)];
    if (!target || target === 'ungrouped' && event.key === 'ArrowRight' && index === layout.groupOrder.length - 1) return;
    event.preventDefault();
    submit(() => onReorderGroup(id, target, event.key === 'ArrowLeft' ? 'before' : 'after'));
  }, [disabled, layout.groupOrder, onReorderGroup, reorderPending, submit]);

  const dragLeave = useCallback((event: DragEvent<HTMLElement>, id: string) => {
    if (event.currentTarget.contains(event.relatedTarget as Node | null)) return;
    if (groupHoverId.current === id) clearGroupHover();
    setDropTarget((current) => (current?.id === id ? null : current));
  }, [clearGroupHover]);

  return { capacityBlockedGroupId, clearDrag, draggedConnectionInstanceId, draggedGroupId, dragLeave, dragOverGroup, dragOverInstance, dropGroup, dropInstance, dropTarget, moveGroupWithKeyboard, moveInstanceWithKeyboard, reorderPending, startGroupDrag, startInstanceDrag };
}
