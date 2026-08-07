import type { SessionSummary } from './terminal-protocol';
import { TerminalActions } from '../ui/terminal-actions';

type Props = {
  sessions: SessionSummary[];
  active: string | null;
  onSelect: (id: string) => void;
  onCloseTab: (id: string) => void;
  onRename: (id: string) => void;
  onAutomaticTitle: (id: string) => void;
  onTerminate: (id: string) => void;
  onCreate: () => void;
};

export function TerminalTabs({ sessions, active, onSelect, onCloseTab, onRename, onAutomaticTitle, onTerminate, onCreate }: Props) {
  return <div className="terminal-tabs"><div className="tab-list">{sessions.map((session) => <div className={`terminal-tab ${session.id === active ? 'active' : ''}`} data-session-id={session.id} key={session.id}>
    <button className="terminal-tab-select" type="button" onClick={() => onSelect(session.id)} aria-current={session.id === active ? 'page' : undefined}><span className="tab-dot" /><span className="tab-label">{session.title || session.cwd.split('/').pop() || 'terminal'}</span><span className="tab-id">{session.id.slice(0, 6)}</span></button>
    <TerminalActions session={session} canCloseTab onRename={() => onRename(session.id)} onAutomaticTitle={() => onAutomaticTitle(session.id)} onCloseTab={() => onCloseTab(session.id)} onTerminate={() => onTerminate(session.id)} />
  </div>)}</div><button className="icon-button" onClick={onCreate} aria-label="New terminal" title="New terminal">+</button></div>;
}
