import { useCallback, useState, type DragEvent, type KeyboardEvent } from 'react';
import type { ConnectionOrderPlacement } from '../app/connection-view';
import type { ConnectionInstanceSummary } from '../terminal/terminal-protocol';

type DropTarget = { id: string; placement: ConnectionOrderPlacement };

type Params = {
  connections: ConnectionInstanceSummary[];
  onReorder: (draggedID: string, targetID: string, placement: ConnectionOrderPlacement) => Promise<void>;
  onPreviewEnd: (id: string) => void;
};

function placementFor(event: DragEvent<HTMLElement>): ConnectionOrderPlacement {
  const bounds = event.currentTarget.getBoundingClientRect();
  return event.clientY < bounds.top + bounds.height / 2 ? 'before' : 'after';
}

export function useConnectionReorder({ connections, onReorder, onPreviewEnd }: Params) {
  const [draggedConnectionInstanceId, setDraggedConnectionInstanceId] = useState<string | null>(null);
  const [dropTarget, setDropTarget] = useState<DropTarget | null>(null);
  const [reorderPending, setReorderPending] = useState(false);

  const clearDrag = useCallback(() => {
    setDraggedConnectionInstanceId(null);
    setDropTarget(null);
  }, []);

  const submitReorder = useCallback((draggedID: string, targetID: string, placement: ConnectionOrderPlacement) => {
    if (reorderPending || draggedID === targetID) return;
    clearDrag();
    setReorderPending(true);
    void Promise.resolve()
      .then(() => onReorder(draggedID, targetID, placement))
      .catch(() => undefined)
      .finally(() => setReorderPending(false));
  }, [clearDrag, onReorder, reorderPending]);

  const dragOver = useCallback((event: DragEvent<HTMLElement>, targetID: string) => {
    if (!draggedConnectionInstanceId || draggedConnectionInstanceId === targetID) return;
    event.preventDefault();
    event.dataTransfer.dropEffect = 'move';
    const placement = placementFor(event);
    setDropTarget((current) => (current?.id === targetID && current.placement === placement ? current : { id: targetID, placement }));
  }, [draggedConnectionInstanceId]);

  const dragLeave = useCallback((event: DragEvent<HTMLElement>, targetID: string) => {
    if (event.currentTarget.contains(event.relatedTarget as Node | null)) return;
    setDropTarget((current) => (current?.id === targetID ? null : current));
  }, []);

  const drop = useCallback((event: DragEvent<HTMLElement>, targetID: string) => {
    event.preventDefault();
    const draggedID = draggedConnectionInstanceId || event.dataTransfer.getData('text/plain');
    if (!draggedID) {
      clearDrag();
      return;
    }
    submitReorder(draggedID, targetID, placementFor(event));
  }, [clearDrag, draggedConnectionInstanceId, submitReorder]);

  const startDrag = useCallback((event: DragEvent<HTMLElement>, id: string) => {
    if (reorderPending) {
      event.preventDefault();
      return;
    }
    event.dataTransfer.effectAllowed = 'move';
    event.dataTransfer.setData('text/plain', id);
    setDraggedConnectionInstanceId(id);
    setDropTarget(null);
    onPreviewEnd(id);
  }, [onPreviewEnd, reorderPending]);

  const moveWithKeyboard = useCallback((event: KeyboardEvent<HTMLElement>, id: string) => {
    if (reorderPending || (event.key !== 'ArrowUp' && event.key !== 'ArrowDown')) return;
    const currentIndex = connections.findIndex((connection) => connection.connectionInstanceId === id);
    const target = connections[currentIndex + (event.key === 'ArrowUp' ? -1 : 1)];
    if (!target) return;
    event.preventDefault();
    submitReorder(id, target.connectionInstanceId, event.key === 'ArrowUp' ? 'before' : 'after');
  }, [connections, reorderPending, submitReorder]);

  return {
    clearDrag,
    draggedConnectionInstanceId,
    dragLeave,
    dragOver,
    drop,
    dropTarget,
    moveWithKeyboard,
    reorderPending,
    startDrag,
  };
}
