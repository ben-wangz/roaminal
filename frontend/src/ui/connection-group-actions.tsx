import { EllipsisVertical } from 'lucide-react';
import { useEffect, useRef, useState } from 'react';

type Props = {
  nonEmpty: boolean;
  onRename: () => void;
  onMoveAll: () => void;
  onDelete: () => void;
};

export function ConnectionGroupActions({ nonEmpty, onRename, onMoveAll, onDelete }: Props) {
  const [open, setOpen] = useState(false);
  const root = useRef<HTMLDivElement>(null);
  useEffect(() => {
    if (!open) return undefined;
    const close = (event: PointerEvent) => {
      if (!root.current?.contains(event.target as Node)) setOpen(false);
    };
    document.addEventListener('pointerdown', close);
    return () => document.removeEventListener('pointerdown', close);
  }, [open]);
  const run = (action: () => void) => {
    setOpen(false);
    action();
  };
  return (
    <div ref={root} className="connection-group-actions" onClick={(event) => event.stopPropagation()}>
      <button className="icon-button" type="button" aria-label="Group actions" title="Group actions" aria-haspopup="menu" aria-expanded={open} onClick={() => setOpen((current) => !current)}>
        <EllipsisVertical size={15} aria-hidden="true" />
      </button>
      {open && <div className="connection-group-menu" role="menu">
        <button type="button" role="menuitem" onClick={() => run(onRename)}>Rename group...</button>
        <button type="button" role="menuitem" disabled={!nonEmpty} title={nonEmpty ? 'Move all instances to Ungrouped before deleting.' : undefined} onClick={() => run(onMoveAll)}>Move all to Ungrouped</button>
        <button type="button" role="menuitem" disabled={nonEmpty} aria-label={nonEmpty ? 'Delete group disabled: move all instances to Ungrouped first' : 'Delete group'} title={nonEmpty ? 'A non-empty group cannot be deleted.' : undefined} onClick={() => run(() => { if (window.confirm('Delete this empty group?')) onDelete(); })}>Delete group</button>
      </div>}
    </div>
  );
}
