import { useCallback, useEffect, useRef, useState, type DragEvent, type KeyboardEvent as ReactKeyboardEvent } from 'react';
import { UNGROUPED_GROUP_ID, type ConnectionInstanceLayout, type InstanceMovePlacement } from '../connections/connection-instance-groups';

export type GroupDropPlacement = InstanceMovePlacement | 'end';
export type GroupDropTarget = { kind: 'instance' | 'group' | 'group-members'; id: string; groupId: string; placement: GroupDropPlacement };
export type DragState =
  | { kind: 'idle'; target: null }
  | { kind: 'group'; sourceId: string; target: GroupDropTarget | null }
  | { kind: 'instance'; sourceId: string; sourceGroupId: string; target: GroupDropTarget | null };

type Props = {
  layout: ConnectionInstanceLayout;
  disabled?: boolean;
  dragEnabled?: boolean;
  onMoveInstance: (id: string, groupId: string, targetId: string | null, placement: InstanceMovePlacement) => Promise<boolean>;
  onReorderGroup: (id: string, targetId: string, placement: InstanceMovePlacement) => Promise<boolean>;
  onPreviewEnd: (id: string) => void;
  onExpandGroup?: (id: string) => void;
  onFocusGroup?: (id: string) => void;
};

const idleDrag: DragState = { kind: 'idle', target: null };

export function placementForRect(clientY: number, bounds: Pick<DOMRect, 'top' | 'height'>): InstanceMovePlacement {
  const midpoint = bounds.top + Math.max(bounds.height, 28) / 2;
  return clientY < midpoint ? 'before' : 'after';
}

function groupName(layout: ConnectionInstanceLayout, id: string): string {
  if (id === UNGROUPED_GROUP_ID) return 'Ungrouped';
  return layout.groups.find((group) => group.groupId === id)?.name || id;
}

function sourceGroupId(layout: ConnectionInstanceLayout, instanceId: string): string | null {
  if (layout.ungroupedConnectionInstanceIds.includes(instanceId)) return UNGROUPED_GROUP_ID;
  return layout.groups.find((group) => group.connectionInstanceIds.includes(instanceId))?.groupId || null;
}

function payload(event: DragEvent<HTMLElement>): { kind: 'group' | 'instance'; id: string } | null {
  const value = event.dataTransfer.getData('text/plain');
  if (value.startsWith('group:')) return { kind: 'group', id: value.slice('group:'.length) };
  if (value.startsWith('instance:')) return { kind: 'instance', id: value.slice('instance:'.length) };
  return null;
}

function sameTarget(left: GroupDropTarget | null, right: GroupDropTarget | null): boolean {
  return left?.kind === right?.kind && left?.id === right?.id && left?.groupId === right?.groupId && left?.placement === right?.placement;
}

export function useConnectionGroupReorder({ layout, disabled = false, dragEnabled = true, onMoveInstance, onReorderGroup, onPreviewEnd, onExpandGroup, onFocusGroup }: Props) {
  const [dragState, setDragState] = useState<DragState>(idleDrag);
  const dragStateRef = useRef<DragState>(idleDrag);
  const [capacityBlockedGroupId, setCapacityBlockedGroupId] = useState<string | null>(null);
  const [reorderPending, setReorderPending] = useState(false);
  const [statusMessage, setStatusMessage] = useState('');
  const groupHoverTimer = useRef<number | null>(null);
  const groupHoverId = useRef<string | null>(null);

  const updateDragState = useCallback((next: DragState) => {
    dragStateRef.current = next;
    setDragState(next);
  }, []);

  const clearGroupHover = useCallback(() => {
    if (groupHoverTimer.current !== null) window.clearTimeout(groupHoverTimer.current);
    groupHoverTimer.current = null;
    groupHoverId.current = null;
  }, []);

  const clearDrag = useCallback(() => {
    clearGroupHover();
    setCapacityBlockedGroupId(null);
    updateDragState(idleDrag);
  }, [clearGroupHover, updateDragState]);

  const clearTarget = useCallback((targetId?: string) => {
    const current = dragStateRef.current;
    if (current.kind === 'idle' || targetId && current.target?.id !== targetId) return;
    clearGroupHover();
    setCapacityBlockedGroupId(null);
    if (current.target) updateDragState({ ...current, target: null });
  }, [clearGroupHover, updateDragState]);

  const scheduleGroupExpansion = useCallback((groupId: string) => {
    if (!onExpandGroup || groupHoverId.current === groupId) return;
    clearGroupHover();
    groupHoverId.current = groupId;
    groupHoverTimer.current = window.setTimeout(() => {
      groupHoverTimer.current = null;
      onExpandGroup(groupId);
    }, 600);
  }, [clearGroupHover, onExpandGroup]);

  const groupIsFull = useCallback((instanceId: string, targetGroupId: string) => {
    const currentGroupId = sourceGroupId(layout, instanceId);
    if (!currentGroupId || targetGroupId === UNGROUPED_GROUP_ID || targetGroupId === currentGroupId) return false;
    const target = layout.groups.find((group) => group.groupId === targetGroupId);
    return Boolean(target && target.connectionInstanceIds.length >= 10);
  }, [layout]);

  const submit = useCallback((action: () => Promise<boolean>, success: string, failure: string, focusGroupId?: string) => {
    if (reorderPending) return;
    clearDrag();
    setReorderPending(true);
    void Promise.resolve()
      .then(action)
      .then((saved) => setStatusMessage(saved ? success : failure))
      .catch(() => setStatusMessage(failure))
      .finally(() => {
        setReorderPending(false);
        if (focusGroupId) window.requestAnimationFrame(() => onFocusGroup?.(focusGroupId));
      });
  }, [clearDrag, onFocusGroup, reorderPending]);

  const dragOverInstance = useCallback((event: DragEvent<HTMLElement>, id: string, groupId: string) => {
    const current = dragStateRef.current;
    if (disabled || current.kind !== 'instance' || current.sourceId === id) {
      if (current.kind === 'instance' && current.sourceId === id) clearTarget();
      return;
    }
    event.preventDefault();
    event.stopPropagation();
    const blocked = groupIsFull(current.sourceId, groupId);
    setCapacityBlockedGroupId(blocked ? groupId : null);
    event.dataTransfer.dropEffect = blocked ? 'none' : 'move';
    const target: GroupDropTarget = { kind: 'instance', id, groupId, placement: placementForRect(event.clientY, event.currentTarget.getBoundingClientRect()) };
    if (!sameTarget(current.target, target)) {
      updateDragState({ ...current, target });
      setStatusMessage(blocked ? 'The target group is full.' : `Move connection instance ${target.placement} the selected connection.`);
    }
  }, [clearTarget, disabled, groupIsFull, updateDragState]);

  const dragOverGroup = useCallback((event: DragEvent<HTMLElement>, groupId: string) => {
    const current = dragStateRef.current;
    if (disabled || current.kind === 'idle') return;
    if (current.kind === 'group' && current.sourceId === groupId) {
      clearTarget();
      return;
    }
    event.preventDefault();
    event.stopPropagation();
    if (current.kind === 'group') {
      const target: GroupDropTarget = { kind: 'group', id: groupId, groupId, placement: placementForRect(event.clientY, event.currentTarget.getBoundingClientRect()) };
      event.dataTransfer.dropEffect = 'move';
      if (!sameTarget(current.target, target)) {
        updateDragState({ ...current, target });
        setStatusMessage(`Move group ${target.placement} ${groupName(layout, groupId)}.`);
      }
      return;
    }
    const blocked = groupIsFull(current.sourceId, groupId);
    setCapacityBlockedGroupId(blocked ? groupId : null);
    event.dataTransfer.dropEffect = blocked ? 'none' : 'move';
    if (!blocked) scheduleGroupExpansion(groupId);
    const target: GroupDropTarget = { kind: 'group-members', id: groupId, groupId, placement: 'end' };
    if (!sameTarget(current.target, target)) {
      updateDragState({ ...current, target });
      setStatusMessage(blocked ? 'The target group is full.' : `Move connection instance to ${groupName(layout, groupId)}.`);
    }
  }, [clearTarget, disabled, groupIsFull, layout, scheduleGroupExpansion, updateDragState]);

  const dropInstance = useCallback((event: DragEvent<HTMLElement>, id: string, groupId: string) => {
    event.preventDefault();
    event.stopPropagation();
    const current = dragStateRef.current;
    if (disabled) return clearDrag();
    if (current.kind !== 'instance') return;
    if (capacityBlockedGroupId === groupId) {
      setStatusMessage('The connection instance was not moved because the target group is full.');
      return clearDrag();
    }
    const draggedId = current.sourceId || payload(event)?.id;
    if (!draggedId || draggedId === id) return clearDrag();
    const placement = placementForRect(event.clientY, event.currentTarget.getBoundingClientRect());
    submit(() => onMoveInstance(draggedId, groupId, id, placement), 'Connection instance moved.', 'Connection instance move was not saved.');
  }, [capacityBlockedGroupId, clearDrag, disabled, onMoveInstance, submit]);

  const dropGroup = useCallback((event: DragEvent<HTMLElement>, groupId: string) => {
    event.preventDefault();
    event.stopPropagation();
    const current = dragStateRef.current;
    if (disabled || current.kind === 'idle') return clearDrag();
    if (current.kind === 'group') {
      if (current.sourceId === groupId) return clearDrag();
      const placement = placementForRect(event.clientY, event.currentTarget.getBoundingClientRect());
      submit(() => onReorderGroup(current.sourceId, groupId, placement), `Group moved ${placement} ${groupName(layout, groupId)}.`, 'Group reorder was not saved.', current.sourceId);
      return;
    }
    if (capacityBlockedGroupId === groupId) {
      setStatusMessage('The connection instance was not moved because the target group is full.');
      return clearDrag();
    }
    submit(() => onMoveInstance(current.sourceId, groupId, null, 'after'), `Connection instance moved to ${groupName(layout, groupId)}.`, 'Connection instance move was not saved.');
  }, [capacityBlockedGroupId, clearDrag, disabled, layout, onMoveInstance, onReorderGroup, submit]);

  const startInstanceDrag = useCallback((event: DragEvent<HTMLElement>, id: string) => {
    if (disabled || !dragEnabled || reorderPending) {
      event.preventDefault();
      return;
    }
    event.dataTransfer.effectAllowed = 'move';
    event.dataTransfer.setData('text/plain', `instance:${id}`);
    setCapacityBlockedGroupId(null);
    updateDragState({ kind: 'instance', sourceId: id, sourceGroupId: sourceGroupId(layout, id) || UNGROUPED_GROUP_ID, target: null });
    setStatusMessage('Dragging connection instance.');
    onPreviewEnd(id);
  }, [disabled, dragEnabled, layout, onPreviewEnd, reorderPending, updateDragState]);

  const startGroupDrag = useCallback((event: DragEvent<HTMLElement>, id: string) => {
    if (disabled || !dragEnabled || reorderPending) {
      event.preventDefault();
      return;
    }
    event.dataTransfer.effectAllowed = 'move';
    event.dataTransfer.setData('text/plain', `group:${id}`);
    setCapacityBlockedGroupId(null);
    updateDragState({ kind: 'group', sourceId: id, target: null });
    setStatusMessage(`Dragging ${groupName(layout, id)} group. Choose a position and release.`);
  }, [disabled, dragEnabled, layout, reorderPending, updateDragState]);

  const moveInstanceWithKeyboard = useCallback((event: ReactKeyboardEvent<HTMLElement>, id: string, groupId: string) => {
    if (disabled || reorderPending || (event.key !== 'ArrowUp' && event.key !== 'ArrowDown')) return;
    const members = groupId === UNGROUPED_GROUP_ID ? layout.ungroupedConnectionInstanceIds : layout.groups.find((group) => group.groupId === groupId)?.connectionInstanceIds || [];
    const index = members.indexOf(id);
    const target = members[index + (event.key === 'ArrowUp' ? -1 : 1)];
    event.preventDefault();
    if (!target) {
      setStatusMessage(event.key === 'ArrowUp' ? 'Already first connection instance.' : 'Already last connection instance.');
      return;
    }
    submit(() => onMoveInstance(id, groupId, target, event.key === 'ArrowUp' ? 'before' : 'after'), 'Connection instance moved.', 'Connection instance move was not saved.');
  }, [disabled, layout, onMoveInstance, reorderPending, submit]);

  const moveGroupWithKeyboard = useCallback((event: ReactKeyboardEvent<HTMLElement>, id: string) => {
    if (disabled || reorderPending || !['ArrowLeft', 'ArrowRight', 'ArrowUp', 'ArrowDown', 'Home', 'End'].includes(event.key)) return;
    const index = layout.groupOrder.indexOf(id);
    if (index < 0) return;
    const towardStart = event.key === 'ArrowLeft' || event.key === 'ArrowUp' || event.key === 'Home';
    const targetIndex = event.key === 'Home' ? 0 : event.key === 'End' ? layout.groupOrder.length - 1 : index + (towardStart ? -1 : 1);
    event.preventDefault();
    if (targetIndex < 0 || targetIndex >= layout.groupOrder.length || targetIndex === index) {
      setStatusMessage(towardStart ? 'Already first group.' : 'Already last group.');
      return;
    }
    const targetId = layout.groupOrder[targetIndex];
    const placement = targetIndex < index ? 'before' : 'after';
    submit(() => onReorderGroup(id, targetId, placement), `${groupName(layout, id)} moved to position ${targetIndex + 1} of ${layout.groupOrder.length}.`, 'Group reorder was not saved.', id);
  }, [disabled, layout, onReorderGroup, reorderPending, submit]);

  const dragLeave = useCallback((event: DragEvent<HTMLElement>, id: string) => {
    if (event.currentTarget.contains(event.relatedTarget as Node | null)) return;
    clearTarget(id);
  }, [clearTarget]);

  useEffect(() => {
    if (disabled || !dragEnabled) clearDrag();
  }, [clearDrag, disabled, dragEnabled]);

  useEffect(() => {
    if (dragState.kind === 'idle') return undefined;
    const cancel = (event: globalThis.KeyboardEvent) => {
      if (event.key !== 'Escape') return;
      event.preventDefault();
      clearDrag();
      setStatusMessage('Reorder canceled.');
    };
    const visibility = () => { if (document.hidden) clearDrag(); };
    window.addEventListener('keydown', cancel);
    document.addEventListener('visibilitychange', visibility);
    return () => {
      window.removeEventListener('keydown', cancel);
      document.removeEventListener('visibilitychange', visibility);
    };
  }, [clearDrag, dragState.kind]);

  useEffect(() => () => clearGroupHover(), [clearGroupHover]);

  return {
    capacityBlockedGroupId,
    clearDrag,
    dragState,
    draggedConnectionInstanceId: dragState.kind === 'instance' ? dragState.sourceId : null,
    draggedGroupId: dragState.kind === 'group' ? dragState.sourceId : null,
    dragLeave,
    dragOverGroup,
    dragOverInstance,
    dropGroup,
    dropInstance,
    dropTarget: dragState.kind === 'idle' ? null : dragState.target,
    moveGroupWithKeyboard,
    moveInstanceWithKeyboard,
    reorderPending,
    startGroupDrag,
    startInstanceDrag,
    statusMessage,
  };
}
