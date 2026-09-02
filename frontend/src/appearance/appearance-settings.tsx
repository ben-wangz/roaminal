import { memo, useEffect, useRef } from 'react';
import { RotateCcw, Save } from 'lucide-react';
import type { Terminal } from '@xterm/xterm';
import { APPEARANCE_SCHEMA_VERSION, DEFAULT_APPEARANCE, FONT_CATALOG, MAX_FONT_SIZE, MIN_FONT_SIZE, type TerminalAppearance, fontFamily, normalizeFontSize } from './appearance-model';

const AppearanceSample = memo(function AppearanceSample({ fontId, fontSize }: Pick<TerminalAppearance, 'fontId' | 'fontSize'>) {
  const elementRef = useRef<HTMLDivElement>(null);
  const terminalRef = useRef<Terminal | null>(null);
  useEffect(() => {
    let disposed = false;
    void import('@xterm/xterm').then(({ Terminal: XtermTerminal }) => {
      if (disposed || !elementRef.current) return;
      const terminal = new XtermTerminal({
        convertEol: true,
        disableStdin: true,
        cursorBlink: false,
        rows: 9,
        cols: 72,
        fontFamily: fontFamily(fontId),
        fontSize,
        theme: { background: '#002b36', foreground: '#93a1a1', cursor: 'transparent', selectionBackground: '#586e75' },
      });
      terminal.open(elementRef.current);
      terminal.write('\x1b[1;36mroaminal\x1b[0m $ appearance --preview\r\n');
      terminal.write('AaBbCc 0123456789  []{}() <> | /\\\r\n');
      terminal.write('\x1b[32mconnected\x1b[0m  \x1b[33mfont metrics ready\x1b[0m\r\n');
      terminal.write('┌──────────────┐  ┌──────────────┐\r\n│  live sample  │  │  xterm ANSI  │\r\n└──────────────┘  └──────────────┘');
      terminalRef.current = terminal;
    });
    return () => {
      disposed = true;
      terminalRef.current?.dispose();
      terminalRef.current = null;
    };
  }, [fontId, fontSize]);
  return <div className="appearance-sample" ref={elementRef} aria-label="Terminal appearance preview" />;
});

type AppearanceControlsProps = {
  appearance: TerminalAppearance;
  draft: TerminalAppearance;
  onDraftChange: (draft: TerminalAppearance) => void;
  onSave: (appearance: TerminalAppearance) => void;
};

export function AppearanceControls({ appearance, draft, onDraftChange, onSave }: AppearanceControlsProps) {
  const validSize = normalizeFontSize(draft.fontSize) !== null;
  const changed = draft.fontId !== appearance.fontId || draft.fontSize !== appearance.fontSize;
  function updateSize(value: string) {
    onDraftChange({ ...draft, fontSize: value === '' ? Number.NaN : Number(value) });
  }
  function save(event: React.FormEvent) {
    event.preventDefault();
    const fontSize = normalizeFontSize(draft.fontSize);
    if (fontSize === null) return;
    onSave({ schemaVersion: APPEARANCE_SCHEMA_VERSION, fontId: draft.fontId, fontSize });
  }
  return (
      <form className="appearance-content" onSubmit={save}>
        <div className="appearance-controls">
          <label>
            <span>Terminal font</span>
            <select id="appearance-font" name="fontId" value={draft.fontId} onChange={(event) => onDraftChange({ ...draft, fontId: event.target.value as TerminalAppearance['fontId'] })}>
              {(Object.keys(FONT_CATALOG) as TerminalAppearance['fontId'][]).map((id) => <option key={id} value={id}>{FONT_CATALOG[id].label}</option>)}
            </select>
          </label>
          <label className="appearance-size-field">
            <span>Terminal size</span>
            <div className="appearance-size-controls">
              <input id="appearance-font-size-range" name="fontSizeRange" aria-label="Terminal font size" type="range" min={MIN_FONT_SIZE} max={MAX_FONT_SIZE} step={1} value={validSize ? draft.fontSize : MIN_FONT_SIZE} onChange={(event) => updateSize(event.target.value)} />
              <input id="appearance-font-size-pixels" name="fontSizePixels" aria-label="Terminal font size in pixels" type="number" min={MIN_FONT_SIZE} max={MAX_FONT_SIZE} step={1} value={Number.isNaN(draft.fontSize) ? '' : draft.fontSize} onChange={(event) => updateSize(event.target.value)} />
              <span aria-hidden="true">px</span>
            </div>
            {!validSize && <small className="appearance-error" role="alert">Enter an integer from {MIN_FONT_SIZE} to {MAX_FONT_SIZE}.</small>}
          </label>
          <div className="appearance-actions">
            <button className="primary" type="submit" disabled={!changed || !validSize}><Save size={15} aria-hidden="true" /> Save</button>
            <button className="text-button" type="button" disabled={!changed} onClick={() => onDraftChange(DEFAULT_APPEARANCE)}><RotateCcw size={15} aria-hidden="true" /> Reset to defaults</button>
          </div>
        </div>
        <div className="appearance-preview-region">
          <div className="appearance-preview-heading"><strong>Preview</strong><span>{FONT_CATALOG[draft.fontId].label} · {validSize ? draft.fontSize : MIN_FONT_SIZE}px</span></div>
          <AppearanceSample fontId={draft.fontId} fontSize={validSize ? draft.fontSize : MIN_FONT_SIZE} />
        </div>
      </form>
  );
}
