import { useEffect, useRef, useState } from 'react';
import { ArrowLeft, Bell, Monitor, RotateCcw, Save } from 'lucide-react';
import type { Terminal } from '@xterm/xterm';
import { APPEARANCE_SCHEMA_VERSION, DEFAULT_APPEARANCE, FONT_CATALOG, MAX_FONT_SIZE, MIN_FONT_SIZE, type TerminalAppearance, fontFamily, normalizeFontSize } from './appearance-model';
import type { NotificationState } from '../status/notification-service';

type Props = {
  appearance: TerminalAppearance;
  onSave: (appearance: TerminalAppearance) => void;
  onBack: () => void;
  onWorkspace: () => void;
  hasWorkspace: boolean;
  notificationState: NotificationState;
  onEnableNotifications: () => Promise<void>;
  onDisableNotifications: () => Promise<void>;
};

function AppearanceSample({ appearance }: { appearance: TerminalAppearance }) {
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
        fontFamily: fontFamily(appearance.fontId),
        fontSize: appearance.fontSize,
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
  }, [appearance]);
  return <div className="appearance-sample" ref={elementRef} aria-label="Terminal appearance preview" />;
}

export function AppearanceSettings({ appearance, onSave, onBack, onWorkspace, hasWorkspace, notificationState, onEnableNotifications, onDisableNotifications }: Props) {
  const [draft, setDraft] = useState(appearance);
  useEffect(() => setDraft(appearance), [appearance]);
  const validSize = normalizeFontSize(draft.fontSize) !== null;
  const changed = draft.fontId !== appearance.fontId || draft.fontSize !== appearance.fontSize;
  function updateSize(value: string) {
    setDraft((current) => ({ ...current, fontSize: value === '' ? Number.NaN : Number(value) }));
  }
  function save(event: React.FormEvent) {
    event.preventDefault();
    const fontSize = normalizeFontSize(draft.fontSize);
    if (fontSize === null) return;
    onSave({ schemaVersion: APPEARANCE_SCHEMA_VERSION, fontId: draft.fontId, fontSize });
  }
  return (
    <section className="appearance-page" aria-labelledby="appearance-title">
      <header className="appearance-header">
        <div>
          <p className="eyebrow">ROAMINAL</p>
          <h1 id="appearance-title">Appearance</h1>
          <p className="appearance-subtitle">Choose how terminal text is rendered in this browser.</p>
        </div>
        <div className="appearance-header-actions">
          {hasWorkspace && <button className="text-button" type="button" onClick={onWorkspace}><Monitor size={15} aria-hidden="true" /> Workspace</button>}
          <button className="text-button" type="button" onClick={onBack}><ArrowLeft size={15} aria-hidden="true" /> Connections</button>
        </div>
      </header>
      <form className="appearance-content" onSubmit={save}>
        <div className="appearance-controls">
          <label>
            <span>Terminal font</span>
            <select value={draft.fontId} onChange={(event) => setDraft((current) => ({ ...current, fontId: event.target.value as TerminalAppearance['fontId'] }))}>
              {(Object.keys(FONT_CATALOG) as TerminalAppearance['fontId'][]).map((id) => <option key={id} value={id}>{FONT_CATALOG[id].label}</option>)}
            </select>
          </label>
          <label className="appearance-size-field">
            <span>Terminal size</span>
            <div className="appearance-size-controls">
              <input aria-label="Terminal font size" type="range" min={MIN_FONT_SIZE} max={MAX_FONT_SIZE} step={1} value={validSize ? draft.fontSize : MIN_FONT_SIZE} onChange={(event) => updateSize(event.target.value)} />
              <input aria-label="Terminal font size in pixels" type="number" min={MIN_FONT_SIZE} max={MAX_FONT_SIZE} step={1} value={Number.isNaN(draft.fontSize) ? '' : draft.fontSize} onChange={(event) => updateSize(event.target.value)} />
              <span aria-hidden="true">px</span>
            </div>
            {!validSize && <small className="appearance-error" role="alert">Enter an integer from {MIN_FONT_SIZE} to {MAX_FONT_SIZE}.</small>}
          </label>
          <div className="appearance-actions">
            <button className="primary" type="submit" disabled={!changed || !validSize}><Save size={15} aria-hidden="true" /> Save</button>
            <button className="text-button" type="button" disabled={!changed} onClick={() => setDraft(DEFAULT_APPEARANCE)}><RotateCcw size={15} aria-hidden="true" /> Reset to defaults</button>
          </div>
          <section className="appearance-notification-settings" aria-labelledby="notification-settings-title">
            <header>
              <div><Bell size={15} aria-hidden="true" /><strong id="notification-settings-title">System notifications</strong></div>
              <span className={`notification-state notification-state-${notificationState.status}`}>{notificationState.status === 'foreground-only' ? 'Foreground only' : notificationState.status[0].toUpperCase() + notificationState.status.slice(1)}</span>
            </header>
            {notificationState.status === 'enable' && <>
              <p>Show Codex completion and failure messages outside the page.</p>
              <button className="notification-toggle" type="button" role="switch" aria-checked="false" onClick={() => void onEnableNotifications()}>
                <span className="notification-toggle-track" aria-hidden="true"><span /></span><span>Off</span>
              </button>
            </>}
            {notificationState.status === 'enabled' && <>
              <p>Codex completion and failure messages can appear as browser notifications.</p>
              <button className="notification-toggle" type="button" role="switch" aria-checked="true" onClick={() => void onDisableNotifications()}>
                <span className="notification-toggle-track" aria-hidden="true"><span /></span><span>On</span>
              </button>
            </>}
            {notificationState.status === 'foreground-only' && <>
              <p>Browser notifications are enabled while this page is running. Background delivery is unavailable.</p>
              <button className="notification-toggle" type="button" role="switch" aria-checked="true" onClick={() => void onDisableNotifications()}>
                <span className="notification-toggle-track" aria-hidden="true"><span /></span><span>On</span>
              </button>
            </>}
            {notificationState.status === 'blocked' && <p>Notifications are blocked. Change this site's permission in browser settings.</p>}
            {notificationState.status === 'unavailable' && <p>This browser or connection does not provide secure system notifications.</p>}
          </section>
        </div>
        <div className="appearance-preview-region">
          <div className="appearance-preview-heading"><strong>Preview</strong><span>{FONT_CATALOG[draft.fontId].label} · {validSize ? draft.fontSize : MIN_FONT_SIZE}px</span></div>
          <AppearanceSample appearance={{ ...draft, fontSize: validSize ? draft.fontSize : MIN_FONT_SIZE }} />
        </div>
      </form>
    </section>
  );
}
