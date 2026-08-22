import { Copy, RefreshCw, Upload } from 'lucide-react';
import { useEffect, useRef } from 'react';
import type { FileEntry } from './filesystem-types';

type Props = {
  entry: FileEntry;
  x: number;
  y: number;
  onClose: () => void;
  onUpload: (entry: FileEntry) => void;
  onRefresh: (path: string) => void;
  onToast: (message: string, kind?: 'info' | 'success' | 'error') => void;
};

export function FileContextMenu({ entry, x, y, onClose, onUpload, onRefresh, onToast }: Props) {
  const menu = useRef<HTMLDivElement>(null);
  useEffect(() => {
    const first = menu.current?.querySelector<HTMLButtonElement>('button');
    first?.focus();
    const closeOnEscape = (event: KeyboardEvent) => {
      if (event.key === 'Escape') { event.preventDefault(); onClose(); }
    };
    document.addEventListener('keydown', closeOnEscape);
    return () => document.removeEventListener('keydown', closeOnEscape);
  }, [onClose]);
  const copy = async (value: string, label: string) => {
    try {
      await navigator.clipboard.writeText(value);
      onToast(`${label} copied.`, 'success');
    } catch {
      onToast('Clipboard access is unavailable.', 'error');
    }
    onClose();
  };
  return (
    <div ref={menu} className="filesystem-context-menu" style={{ left: x, top: y }} role="menu" onMouseDown={(event) => event.stopPropagation()}>
      <button type="button" role="menuitem" onClick={() => void copy(entry.absolutePath, 'Absolute path')}><Copy size={14} aria-hidden="true" /> Copy absolute path</button>
      <button type="button" role="menuitem" onClick={() => void copy(entry.relativePath, 'Relative path')}><Copy size={14} aria-hidden="true" /> Copy relative path</button>
      {entry.type === 'directory' && <button type="button" role="menuitem" onClick={() => { onUpload(entry); onClose(); }}><Upload size={14} aria-hidden="true" /> Upload to this directory...</button>}
      {entry.type === 'directory' && <button type="button" role="menuitem" onClick={() => { onRefresh(entry.relativePath); onClose(); }}><RefreshCw size={14} aria-hidden="true" /> Refresh</button>}
    </div>
  );
}
