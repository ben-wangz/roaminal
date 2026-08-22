import { useEffect, useId, useRef, useState } from 'react';
import { ChevronRight, EllipsisVertical } from 'lucide-react';
import type { ConnectionInstanceSummary } from '../terminal/terminal-protocol';

type Props = {
  connection: ConnectionInstanceSummary;
  moveTargets?: ConnectionGroupMoveTarget[];
  onMoveToGroup?: (groupId: string) => void;
  onRename: () => void;
  onAutomaticTitle: () => void;
  onTerminate: () => void;
};

export type ConnectionGroupMoveTarget = {
  groupId: string;
  name: string;
  count: number;
  current: boolean;
  full: boolean;
};

export function ConnectionActions({ connection, moveTargets = [], onMoveToGroup, onRename, onAutomaticTitle, onTerminate }: Props) {
  const [open, setOpen] = useState(false);
  const [moveOpen, setMoveOpen] = useState(false);
  const [position, setPosition] = useState({ top: 0, left: 0 });
  const root = useRef<HTMLDivElement>(null);
  const trigger = useRef<HTMLButtonElement>(null);
  const menu = useRef<HTMLDivElement>(null);
  const menuId = `connection-actions-menu-${useId().replace(/:/g, '')}`;

  function focusItem(index: number) {
    const items = menu.current ? Array.from(menu.current.querySelectorAll<HTMLButtonElement>('[role="menuitem"]')) : [];
    if (items.length) items[(index + items.length) % items.length]?.focus();
  }

  function closeMenu(restoreFocus = false) {
    setOpen(false);
    setMoveOpen(false);
    if (restoreFocus) trigger.current?.focus();
  }

  useEffect(() => {
    if (!open) return;
    const close = (event: PointerEvent) => {
      if (root.current && !root.current.contains(event.target as Node)) closeMenu();
    };
    document.addEventListener('pointerdown', close);
    window.setTimeout(() => focusItem(0), 0);
    return () => document.removeEventListener('pointerdown', close);
  }, [open]);

  function run(action: () => void) {
    // Restore focus to the trigger before acting: the menu item unmounts in
    // the same commit, and a dialog opened while document.activeElement is a
    // detached node leaves Base UI's focus trap and outside-inert marking in
    // an inconsistent state.
    closeMenu(true);
    action();
  }

  return (
    <div className="connection-actions-menu" ref={root} onClick={(event) => event.stopPropagation()}>
      <button
        ref={trigger}
        className="connection-action-trigger icon-button"
        type="button"
        aria-label="Connection actions"
        title="Connection actions"
        aria-haspopup="menu"
        aria-expanded={open}
        aria-controls={menuId}
        onClick={(event) => {
          event.stopPropagation();
          const rect = event.currentTarget.getBoundingClientRect();
          setPosition({ top: rect.bottom + 4, left: Math.max(8, rect.right - 190) });
          if (open) closeMenu(true);
          else setOpen(true);
        }}
      >
        <EllipsisVertical aria-hidden="true" size={16} strokeWidth={1.8} />
      </button>
      {open && (
        <div
          id={menuId}
          ref={menu}
          className="connection-action-menu"
          style={{ top: position.top, left: position.left }}
          role="menu"
          tabIndex={-1}
          aria-label={`${connection.title || 'Connection'} actions`}
          onClick={(event) => event.stopPropagation()}
          onKeyDown={(event) => {
            const items = menu.current
              ? Array.from(menu.current.querySelectorAll<HTMLButtonElement>('[role="menuitem"]'))
              : [];
            const current = items.indexOf(document.activeElement as HTMLButtonElement);
            if (event.key === 'Escape') {
              event.preventDefault();
              // Consume the key so the mobile sidebar drawer under the menu
              // does not also close on the same press.
              event.stopPropagation();
              closeMenu(true);
            } else if (event.key === 'ArrowDown') {
              event.preventDefault();
              focusItem(current + 1);
            } else if (event.key === 'ArrowUp') {
              event.preventDefault();
              focusItem(current - 1);
            } else if (event.key === 'Home') {
              event.preventDefault();
              focusItem(0);
            } else if (event.key === 'End') {
              event.preventDefault();
              focusItem(items.length - 1);
            }
          }}
        >
          <button type="button" role="menuitem" onClick={() => run(onRename)}>
            Rename title...
          </button>
          {connection.titleMode === 'custom' && (
            <button type="button" role="menuitem" onClick={() => run(onAutomaticTitle)}>
              Use automatic title
            </button>
          )}
          {onMoveToGroup && moveTargets.length > 1 && <>
            <button type="button" role="menuitem" aria-haspopup="menu" aria-expanded={moveOpen} onClick={() => setMoveOpen((current) => !current)}>
              Move to group <ChevronRight size={14} aria-hidden="true" />
            </button>
            {moveOpen && <div className="connection-action-submenu" role="group" aria-label="Move to group">
              {moveTargets.map((target) => (
                <button key={target.groupId} type="button" role="menuitem" disabled={target.current || target.full} title={target.full ? 'Group limit reached (10).' : undefined} onClick={() => run(() => onMoveToGroup(target.groupId))}>
                  <span>{target.name}</span><small>{target.groupId === 'ungrouped' ? target.count : `${target.count}/10`}</small>
                </button>
              ))}
            </div>}
          </>}
          <button className="destructive" type="button" role="menuitem" onClick={() => run(onTerminate)}>
            Close connection...
          </button>
        </div>
      )}
    </div>
  );
}
