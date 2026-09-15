import { act, create, type ReactTestRenderer } from 'react-test-renderer';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { UNGROUPED_GROUP_ID, type ConnectionInstanceLayout } from '../connections/connection-instance-groups';
import { placementForRect, useConnectionGroupReorder } from './use-connection-group-reorder';

const layout: ConnectionInstanceLayout = {
  revision: 4,
  groupOrder: ['alpha', 'beta', UNGROUPED_GROUP_ID],
  groups: [
    { groupId: 'alpha', name: 'Alpha', connectionInstanceIds: ['instance-a'] },
    { groupId: 'beta', name: 'Beta', connectionInstanceIds: ['instance-b'] },
  ],
  ungroupedConnectionInstanceIds: [],
};

type Props = {
  onMoveInstance: (id: string, groupId: string, targetId: string | null, placement: 'before' | 'after') => Promise<boolean>;
  onReorderGroup: (id: string, targetId: string, placement: 'before' | 'after') => Promise<boolean>;
};

function surface() {
  return { getBoundingClientRect: () => ({ top: 0, height: 40 }), contains: () => false };
}

function dragEvent(clientY = 10) {
  return {
    clientY,
    currentTarget: surface(),
    relatedTarget: null,
    preventDefault: vi.fn(),
    stopPropagation: vi.fn(),
    dataTransfer: { effectAllowed: '', dropEffect: '', getData: vi.fn(() => ''), setData: vi.fn() },
  } as unknown as React.DragEvent<HTMLElement>;
}

function Harness({ onMoveInstance, onReorderGroup }: Props) {
  const reorder = useConnectionGroupReorder({
    layout,
    onMoveInstance,
    onReorderGroup,
    onPreviewEnd: vi.fn(),
    dragEnabled: true,
  });
  return (
    <div data-kind={reorder.dragState.kind} data-target={reorder.dropTarget?.id || ''} data-pending={reorder.reorderPending} data-status={reorder.statusMessage}>
      <button data-testid="group-alpha" onDragStart={(event) => reorder.startGroupDrag(event, 'alpha')} onDragOver={(event) => reorder.dragOverGroup(event, 'alpha')} onDragEnd={reorder.clearDrag} onKeyDown={(event) => reorder.moveGroupWithKeyboard(event, 'alpha')} />
      <button data-testid="group-beta" onDragOver={(event) => reorder.dragOverGroup(event, 'beta')} onDragLeave={(event) => reorder.dragLeave(event, 'beta')} onDrop={(event) => reorder.dropGroup(event, 'beta')} />
      <button data-testid="instance-a" onDragOver={(event) => reorder.dragOverInstance(event, 'instance-a', 'alpha')} onDrop={(event) => reorder.dropInstance(event, 'instance-a', 'alpha')} />
      <button data-testid="instance-b" onDragOver={(event) => reorder.dragOverInstance(event, 'instance-b', 'beta')} onDrop={(event) => reorder.dropInstance(event, 'instance-b', 'beta')} />
    </div>
  );
}

async function flush() {
  await Promise.resolve();
  await Promise.resolve();
  await Promise.resolve();
}

describe('useConnectionGroupReorder', () => {
  let renderer: ReactTestRenderer | null = null;
  let windowListeners: Record<string, EventListener>;

  beforeEach(() => {
    vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true);
    windowListeners = {};
    vi.stubGlobal('window', {
      addEventListener: vi.fn((name: string, listener: EventListener) => { windowListeners[name] = listener; }),
      removeEventListener: vi.fn(),
      clearTimeout: vi.fn(),
      setTimeout: vi.fn((callback: () => void) => { callback(); return 1; }),
      requestAnimationFrame: vi.fn((callback: FrameRequestCallback) => { callback(0); return 1; }),
    });
    vi.stubGlobal('document', { hidden: false, addEventListener: vi.fn(), removeEventListener: vi.fn() });
  });

  afterEach(async () => {
    if (renderer) await act(async () => renderer?.unmount());
    renderer = null;
    vi.unstubAllGlobals();
  });

  it('calculates a stable midpoint for short drop surfaces', () => {
    expect(placementForRect(10, { top: 0, height: 10 })).toBe('before');
    expect(placementForRect(22, { top: 0, height: 10 })).toBe('after');
  });

  it('reorders a group before the target and never moves an instance over a card', async () => {
    const onMoveInstance = vi.fn(async () => true);
    const onReorderGroup = vi.fn(async () => true);
    await act(async () => { renderer = create(<Harness onMoveInstance={onMoveInstance} onReorderGroup={onReorderGroup} />); });
    const source = renderer!.root.findByProps({ 'data-testid': 'group-alpha' });
    const target = renderer!.root.findByProps({ 'data-testid': 'group-beta' });
    const card = renderer!.root.findByProps({ 'data-testid': 'instance-b' });

    await act(async () => {
      source.props.onDragStart(dragEvent());
      target.props.onDragOver(dragEvent(10));
      card.props.onDragOver(dragEvent(30));
      card.props.onDrop(dragEvent(30));
      source.props.onDragOver(dragEvent(10));
      expect(renderer!.root.findByType('div').props['data-target']).toBe('');
      target.props.onDragOver(dragEvent(10));
      target.props.onDrop(dragEvent(10));
      await flush();
    });

    expect(onMoveInstance).not.toHaveBeenCalled();
    expect(onReorderGroup).toHaveBeenCalledOnce();
    expect(onReorderGroup).toHaveBeenCalledWith('alpha', 'beta', 'before');
  });

  it('cancels a pointer drag with Escape and ignores a second action while saving', async () => {
    let resolveSave: ((saved: boolean) => void) | undefined;
    const onMoveInstance = vi.fn(async () => true);
    const onReorderGroup = vi.fn(() => new Promise<boolean>((resolve) => { resolveSave = resolve; }));
    await act(async () => { renderer = create(<Harness onMoveInstance={onMoveInstance} onReorderGroup={onReorderGroup} />); });
    const source = renderer!.root.findByProps({ 'data-testid': 'group-alpha' });
    const target = renderer!.root.findByProps({ 'data-testid': 'group-beta' });

    await act(async () => {
      source.props.onDragStart(dragEvent());
      target.props.onDragOver(dragEvent(30));
      target.props.onDrop(dragEvent(30));
    });
    expect(onReorderGroup).toHaveBeenCalledOnce();
    expect(renderer!.root.findByType('div').props['data-pending']).toBe(true);

    await act(async () => {
      source.props.onKeyDown({ key: 'ArrowRight', preventDefault: vi.fn() });
      windowListeners.keydown?.({ key: 'Escape', preventDefault: vi.fn() } as unknown as Event);
      resolveSave?.(true);
      await flush();
    });
    expect(onReorderGroup).toHaveBeenCalledOnce();
    expect(renderer!.root.findByType('div').props['data-kind']).toBe('idle');
  });

  it('announces group boundaries and supports Home/End keyboard movement', async () => {
    const onMoveInstance = vi.fn(async () => true);
    const onReorderGroup = vi.fn(async () => true);
    await act(async () => { renderer = create(<Harness onMoveInstance={onMoveInstance} onReorderGroup={onReorderGroup} />); });
    const first = renderer!.root.findByProps({ 'data-testid': 'group-alpha' });

    await act(async () => {
      first.props.onKeyDown({ key: 'ArrowLeft', preventDefault: vi.fn() });
      await flush();
    });
    expect(renderer!.root.findByType('div').props['data-status']).toBe('Already first group.');

    await act(async () => {
      first.props.onKeyDown({ key: 'End', preventDefault: vi.fn() });
      await flush();
    });
    expect(onReorderGroup).toHaveBeenCalledWith('alpha', UNGROUPED_GROUP_ID, 'after');
  });
});
