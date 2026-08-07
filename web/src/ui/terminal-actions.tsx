import { useEffect, useRef, useState } from 'react';
import type { SessionSummary } from '../terminal/terminal-protocol';

type Props = {
  session: SessionSummary;
  canCloseTab: boolean;
  onRename: () => void;
  onAutomaticTitle: () => void;
  onCloseTab: () => void;
  onTerminate: () => void;
};

export function TerminalActions({ session, canCloseTab, onRename, onAutomaticTitle, onCloseTab, onTerminate }: Props) {
  const [open, setOpen] = useState(false);
  const [position, setPosition] = useState({ top: 0, left: 0 });
  const root = useRef<HTMLDivElement>(null);
  const firstItem = useRef<HTMLButtonElement>(null);

  useEffect(() => {
    if (!open) return;
    const close = (event: PointerEvent) => { if (root.current && !root.current.contains(event.target as Node)) setOpen(false); };
    const keydown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') { event.preventDefault(); setOpen(false); }
      if (event.key === 'ArrowDown') { event.preventDefault(); firstItem.current?.focus(); }
    };
    document.addEventListener('pointerdown', close);
    document.addEventListener('keydown', keydown);
    firstItem.current?.focus();
    return () => { document.removeEventListener('pointerdown', close); document.removeEventListener('keydown', keydown); };
  }, [open]);

  function run(action: () => void) { setOpen(false); action(); }
  return <div className="terminal-actions" ref={root}>
    <button className="tab-menu-trigger icon-button" type="button" aria-label="Terminal actions" title="Terminal actions" aria-haspopup="menu" aria-expanded={open} onClick={(event) => { event.stopPropagation(); const rect = event.currentTarget.getBoundingClientRect(); setPosition({ top: rect.bottom + 4, left: Math.max(8, rect.right - 190) }); setOpen((value) => !value); }}>⋮</button>
    {open && <div className="terminal-action-menu" style={{ top: position.top, left: position.left }} role="menu" aria-label={`${session.title || 'Terminal'} actions`} onClick={(event) => event.stopPropagation()}>
      <button ref={firstItem} type="button" role="menuitem" onClick={() => run(onRename)}>Rename title...</button>
      {session.titleMode === 'custom' && <button type="button" role="menuitem" onClick={() => run(onAutomaticTitle)}>Use automatic title</button>}
      {canCloseTab && <button type="button" role="menuitem" onClick={() => run(onCloseTab)}>Close tab</button>}
      <button className="destructive" type="button" role="menuitem" onClick={() => run(onTerminate)}>Terminate terminal...</button>
    </div>}
  </div>;
}
