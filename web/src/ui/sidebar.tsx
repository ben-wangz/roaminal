import type { SessionSummary } from '../terminal/terminal-protocol';
import { TerminalActions } from './terminal-actions';

type Props = { id: string; sessions: SessionSummary[]; active: string | null; open: boolean; onToggle: () => void; onSelect: (id: string) => void; onRename: (id: string) => void; onAutomaticTitle: (id: string) => void; onTerminate: (id: string) => void; onCreate: () => void };

export function Sidebar({ id, sessions, active, open, onToggle, onSelect, onRename, onAutomaticTitle, onTerminate, onCreate }: Props) {
  return <>
    {open && <button className="sidebar-backdrop" type="button" aria-label="Close sidebar" onClick={onToggle} />}
    <aside id={id} className={`sidebar ${open ? 'open' : 'closed'}`} aria-hidden={!open}>
      <div className="sidebar-header"><div className="brand-mark small">r<span>&gt;</span></div><strong>Roaminal</strong><button className="icon-button sidebar-toggle" onClick={onToggle} aria-label="Toggle sidebar" title="Toggle sidebar" aria-expanded={open} aria-controls={id}>{open ? '‹' : '›'}</button></div>
      <div className="sidebar-actions"><button className="primary full" onClick={onCreate}>+ New terminal</button></div>
      <div className="session-list">{sessions.map((session) => <div className={`session-row ${session.id === active ? 'active' : ''}`} data-session-id={session.id} key={session.id}>
        <button className="session-select" type="button" onClick={() => onSelect(session.id)}><span className="session-indicator" /><span><b>{session.title || 'Terminal'}</b><small>{session.cwd}</small></span></button>
        <TerminalActions session={session} canCloseTab={false} onRename={() => onRename(session.id)} onAutomaticTitle={() => onAutomaticTitle(session.id)} onCloseTab={() => undefined} onTerminate={() => onTerminate(session.id)} />
      </div>)}</div>
      <div className="sidebar-footer">Single instance · Bash</div>
    </aside>
  </>;
}
