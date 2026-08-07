import { useEffect, useId, useRef, useState } from 'react';
import { EllipsisVertical } from 'lucide-react';
import type { SessionSummary } from '../terminal/terminal-protocol';

type Props = {
  session: SessionSummary;
  onRename: () => void;
  onAutomaticTitle: () => void;
  onTerminate: () => void;
};

export function TerminalActions({ session, onRename, onAutomaticTitle, onTerminate }: Props) {
  const [open, setOpen] = useState(false);
  const [position, setPosition] = useState({ top: 0, left: 0 });
  const root = useRef<HTMLDivElement>(null);
  const trigger = useRef<HTMLButtonElement>(null);
  const menu = useRef<HTMLDivElement>(null);
  const menuId = `terminal-actions-${useId().replace(/:/g, '')}`;

  function focusItem(index: number) {
    const items = menu.current ? Array.from(menu.current.querySelectorAll<HTMLButtonElement>('[role="menuitem"]')) : [];
    if (items.length) items[(index + items.length) % items.length]?.focus();
  }

  function closeMenu(restoreFocus = false) {
    setOpen(false);
    if (restoreFocus) trigger.current?.focus();
  }

  useEffect(() => {
    if (!open) return;
    const close = (event: PointerEvent) => { if (root.current && !root.current.contains(event.target as Node)) closeMenu(); };
    document.addEventListener('pointerdown', close);
    window.setTimeout(() => focusItem(0), 0);
    return () => document.removeEventListener('pointerdown', close);
  }, [open]);

  function run(action: () => void) { closeMenu(); action(); }

  return <div className="terminal-actions" ref={root} onClick={(event) => event.stopPropagation()}>
    <button
      ref={trigger}
      className="terminal-action-trigger icon-button"
      type="button"
      aria-label="Terminal actions"
      title="Terminal actions"
      aria-haspopup="menu"
      aria-expanded={open}
      aria-controls={menuId}
      onClick={(event) => {
        event.stopPropagation();
        const rect = event.currentTarget.getBoundingClientRect();
        setPosition({ top: rect.bottom + 4, left: Math.max(8, rect.right - 190) });
        if (open) closeMenu(true); else setOpen(true);
      }}
    ><EllipsisVertical aria-hidden="true" size={16} strokeWidth={1.8} /></button>
    {open && <div id={menuId} ref={menu} className="terminal-action-menu" style={{ top: position.top, left: position.left }} role="menu" tabIndex={-1} aria-label={`${session.title || 'Terminal'} actions`} onClick={(event) => event.stopPropagation()} onKeyDown={(event) => {
      const items = menu.current ? Array.from(menu.current.querySelectorAll<HTMLButtonElement>('[role="menuitem"]')) : [];
      const current = items.indexOf(document.activeElement as HTMLButtonElement);
      if (event.key === 'Escape') { event.preventDefault(); closeMenu(true); }
      else if (event.key === 'ArrowDown') { event.preventDefault(); focusItem(current + 1); }
      else if (event.key === 'ArrowUp') { event.preventDefault(); focusItem(current - 1); }
      else if (event.key === 'Home') { event.preventDefault(); focusItem(0); }
      else if (event.key === 'End') { event.preventDefault(); focusItem(items.length - 1); }
    }}>
      <button type="button" role="menuitem" onClick={() => run(onRename)}>Rename title...</button>
      {session.titleMode === 'custom' && <button type="button" role="menuitem" onClick={() => run(onAutomaticTitle)}>Use automatic title</button>}
      <button className="destructive" type="button" role="menuitem" onClick={() => run(onTerminate)}>Terminate terminal...</button>
    </div>}
  </div>;
}
