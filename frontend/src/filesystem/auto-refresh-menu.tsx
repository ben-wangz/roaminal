import { Check, Timer } from 'lucide-react';
import { useEffect, useRef, useState } from 'react';
import { AUTO_REFRESH_OPTIONS, autoRefreshLabel } from './auto-refresh-settings';

type Props = {
  value: number;
  disabled?: boolean;
  degraded?: boolean;
  onChange: (seconds: number) => void;
};

export function AutoRefreshMenu({ value, disabled, degraded, onChange }: Props) {
  const [open, setOpen] = useState(false);
  const container = useRef<HTMLDivElement>(null);
  useEffect(() => {
    if (!open) return undefined;
    const close = (event: MouseEvent) => {
      if (!container.current?.contains(event.target as Node)) setOpen(false);
    };
    document.addEventListener('mousedown', close);
    return () => document.removeEventListener('mousedown', close);
  }, [open]);
  return (
    <div ref={container} className="filesystem-auto-refresh">
      <button
        className={`icon-button ${degraded ? 'degraded' : ''}`}
        type="button"
        disabled={disabled}
        aria-label={`Auto refresh: ${autoRefreshLabel(value)}`}
        aria-haspopup="menu"
        aria-expanded={open}
        title={`Auto refresh: ${autoRefreshLabel(value)}${degraded ? ' (degraded)' : ''}`}
        onClick={() => setOpen((current) => !current)}
      >
        <Timer size={14} aria-hidden="true" />
      </button>
      {open && <div className="filesystem-auto-refresh-menu" role="menu" aria-label="Auto refresh interval">
        {AUTO_REFRESH_OPTIONS.map((option) => (
          <button
            key={option.seconds}
            type="button"
            role="menuitemradio"
            aria-checked={value === option.seconds}
            onClick={() => { onChange(option.seconds); setOpen(false); }}
          >
            <span>{option.label}</span>
            {value === option.seconds && <Check size={14} aria-hidden="true" />}
          </button>
        ))}
      </div>}
    </div>
  );
}
