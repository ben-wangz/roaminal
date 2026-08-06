import type { SessionSummary } from './terminal-protocol';

export function TerminalTabs({ sessions, active, onSelect, onClose, onCreate }: { sessions: SessionSummary[]; active: string | null; onSelect: (id: string) => void; onClose: (id: string) => void; onCreate: () => void }) {
  return <div className="terminal-tabs"><div className="tab-list">{sessions.map((session) => <button className={`terminal-tab ${session.id === active ? 'active' : ''}`} key={session.id} onClick={() => onSelect(session.id)}><span className="tab-dot" /> <span className="tab-label">{session.title || session.cwd.split('/').pop() || 'terminal'}</span><span className="tab-id">{session.id.slice(0, 6)}</span><span className="tab-close" onClick={(event) => { event.stopPropagation(); onClose(session.id); }} aria-label="Close terminal">×</span></button>)}</div><button className="icon-button" onClick={onCreate} aria-label="New terminal" title="New terminal">+</button></div>;
}
