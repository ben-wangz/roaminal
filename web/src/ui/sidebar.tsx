import { useEffect, useRef } from 'react';
import type { SessionSummary } from '../terminal/terminal-protocol';
import { TerminalActions } from './terminal-actions';

type Props = { id: string; sessions: SessionSummary[]; active: string | null; open: boolean; onToggle: () => void; onSelect: (id: string) => void; onRename: (id: string) => void; onAutomaticTitle: (id: string) => void; onTerminate: (id: string) => void; onCreate: () => void };

export function Sidebar({ id, sessions, active, open, onToggle, onSelect, onRename, onAutomaticTitle, onTerminate, onCreate }: Props) {
  const aside = useRef<HTMLElement>(null);
  const toggle = useRef<HTMLButtonElement>(null);
  const mounted = useRef(false);
  useEffect(() => {
    if (mounted.current && open) toggle.current?.focus();
    mounted.current = true;
  }, [open]);
  useEffect(() => {
    if (!open) return;
    const handleKeyboard = (event: KeyboardEvent) => {
      if (event.key === 'Escape' && window.matchMedia('(max-width: 800px)').matches) {
        event.preventDefault();
        onToggle();
        return;
      }
      if (event.key !== 'Tab' || !window.matchMedia('(max-width: 800px)').matches || !aside.current) return;
      const focusable = Array.from(aside.current.querySelectorAll<HTMLElement>('button:not([disabled]), [href], input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])'));
      if (!focusable.length) return;
      const first = focusable[0];
      const last = focusable[focusable.length - 1];
      if (!aside.current.contains(document.activeElement)) {
        event.preventDefault();
        first.focus();
      } else if (event.shiftKey && document.activeElement === first) {
        event.preventDefault();
        last.focus();
      } else if (!event.shiftKey && document.activeElement === last) {
        event.preventDefault();
        first.focus();
      }
    };
    document.addEventListener('keydown', handleKeyboard);
    return () => document.removeEventListener('keydown', handleKeyboard);
  }, [onToggle, open]);
  return <>
    {open && <button className="sidebar-backdrop" type="button" aria-label="Close sidebar" onClick={onToggle} />}
    <aside ref={aside} id={id} className={`sidebar ${open ? 'open' : 'closed'}`} aria-hidden={!open} inert={!open || undefined}>
      <div className="sidebar-header"><div className="brand-mark small">r<span>&gt;</span></div><strong>Roaminal</strong><button ref={toggle} className="icon-button sidebar-toggle" type="button" onClick={onToggle} aria-label="Toggle sidebar" title="Toggle sidebar" aria-expanded={open} aria-controls={id}>{open ? '‹' : '›'}</button></div>
      <div className="sidebar-actions"><button className="primary full" onClick={onCreate}>+ New terminal</button></div>
      <div className="session-list">{sessions.map((session) => <div className={`session-row ${session.id === active ? 'active' : ''}`} data-session-id={session.id} key={session.id}>
        <button className="session-select" type="button" onClick={() => onSelect(session.id)}><span className="session-indicator" /><span><b>{session.title || 'Terminal'}</b><small>{session.cwd}</small></span></button>
        <TerminalActions session={session} canCloseTab={false} onRename={() => onRename(session.id)} onAutomaticTitle={() => onAutomaticTitle(session.id)} onCloseTab={() => undefined} onTerminate={() => onTerminate(session.id)} />
      </div>)}</div>
      <div className="sidebar-footer">Single instance · Bash</div>
    </aside>
  </>;
}
