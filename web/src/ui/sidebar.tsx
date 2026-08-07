import { useEffect, useRef } from 'react';
import { Bot, FolderOpen, PanelLeftClose, PanelLeftOpen, Plus } from 'lucide-react';
import type { SessionSummary } from '../terminal/terminal-protocol';
import { TerminalActions } from './terminal-actions';
import { TerminalPreview, type TerminalPreviewRuntime } from '../terminal/terminal-preview';

type Props = {
  id: string;
  sessions: SessionSummary[];
  active: string | null;
  open: boolean;
  previewSessionId: string | null;
  previewRuntime: TerminalPreviewRuntime | null;
  onToggle: () => void;
  onSelect: (id: string) => void;
  onPreviewStart: (id: string) => void;
  onPreviewEnd: (id: string) => void;
  onUnavailableExtension: (name: 'Agent' | 'Files') => void;
  onRename: (id: string) => void;
  onAutomaticTitle: (id: string) => void;
  onTerminate: (id: string) => void;
  onCreate: () => void;
};

function shortId(id: string): string {
  const part = id.split('-').pop();
  return part && part.length >= 12 ? part.slice(-12) : id.slice(0, 12);
}

function sinceLabel(createdAt: string): string {
  const date = new Date(createdAt);
  if (Number.isNaN(date.getTime())) return 'Unknown';
  const pad = (value: number) => String(value).padStart(2, '0');
  const hour = date.getHours();
  return `${pad(date.getMonth() + 1)}-${pad(date.getDate())} ${pad(hour % 12 || 12)}:${pad(date.getMinutes())} ${hour >= 12 ? 'PM' : 'AM'}`;
}

function canPreview(): boolean {
  return window.matchMedia('(pointer: fine)').matches && window.innerWidth > 800;
}

export function Sidebar({ id, sessions, active, open, previewSessionId, previewRuntime, onToggle, onSelect, onPreviewStart, onPreviewEnd, onUnavailableExtension, onRename, onAutomaticTitle, onTerminate, onCreate }: Props) {
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
      <div className="sidebar-header"><div className="brand-mark small">r<span>&gt;</span></div><strong>Roaminal</strong><button ref={toggle} className="icon-button sidebar-toggle" type="button" onClick={onToggle} aria-label="Toggle sidebar" title="Toggle sidebar" aria-expanded={open} aria-controls={id}>{open ? <PanelLeftClose aria-hidden="true" size={18} /> : <PanelLeftOpen aria-hidden="true" size={18} />}</button></div>
      <div className="sidebar-actions"><button className="primary full" onClick={onCreate}><Plus aria-hidden="true" size={16} /> New terminal</button></div>
      <div className="session-list">{sessions.map((session) => {
        const previewing = previewSessionId === session.id && previewRuntime;
        const startPreview = () => { if (canPreview()) onPreviewStart(session.id); };
        const stopPreview = () => onPreviewEnd(session.id);
        return <article
          className={`session-card ${session.id === active ? 'active' : ''} ${previewing ? 'previewing' : ''}`}
          data-session-id={session.id}
          key={session.id}
          onMouseEnter={startPreview}
          onMouseLeave={stopPreview}
          onClick={() => onSelect(session.id)}
          onFocus={(event) => { if (!event.currentTarget.contains(event.relatedTarget as Node | null)) startPreview(); }}
          onBlur={(event) => { if (!event.currentTarget.contains(event.relatedTarget as Node | null)) stopPreview(); }}
        >
          <div className="session-card-preview">{previewing && <TerminalPreview runtime={previewRuntime} />}</div>
          <div className="session-card-overlay">
            <button className="session-select" type="button" onClick={() => onSelect(session.id)} aria-current={session.id === active ? 'page' : undefined} title={session.id}>
              <span className="session-indicator" />
              <span className="session-title-wrap"><b>{session.title || 'Terminal'}</b><small>{session.closed ? 'Terminated' : 'Bash session'}</small></span>
            </button>
            <div className="session-metadata">
              <span>ID: {shortId(session.id)}</span>
              <span className="session-path" title={session.cwd}>PWD: {session.cwd}</span>
              <time dateTime={session.createdAt} title={session.createdAt}>SINCE: {sinceLabel(session.createdAt)}</time>
            </div>
          </div>
          <div className="session-actions" aria-label="Session extensions and actions">
            <button className="extension-button" type="button" aria-label="Agent extension" aria-disabled="true" title="Agent extension unavailable" onClick={(event) => { event.stopPropagation(); onUnavailableExtension('Agent'); }}><Bot aria-hidden="true" size={15} /></button>
            <button className="extension-button" type="button" aria-label="Files extension" aria-disabled="true" title="Files extension unavailable" onClick={(event) => { event.stopPropagation(); onUnavailableExtension('Files'); }}><FolderOpen aria-hidden="true" size={15} /></button>
            <TerminalActions session={session} onRename={() => onRename(session.id)} onAutomaticTitle={() => onAutomaticTitle(session.id)} onTerminate={() => onTerminate(session.id)} />
          </div>
        </article>;
      })}</div>
      <div className="sidebar-footer">Single instance · Bash</div>
    </aside>
  </>;
}
