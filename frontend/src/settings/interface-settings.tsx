import { useEffect, useState } from 'react';
import { Clock3 } from 'lucide-react';
import type { TerminalAppearance } from '../appearance/appearance-model';
import { AppearanceControls } from '../appearance/appearance-settings';
import { AUTO_REFRESH_OPTIONS, autoRefreshLabel, readAutoRefreshSeconds, writeAutoRefreshSeconds } from '../filesystem/auto-refresh-settings';

type Props = {
  appearance: TerminalAppearance;
  appearanceDraft: TerminalAppearance;
  onAppearanceDraftChange: (appearance: TerminalAppearance) => void;
  onSaveAppearance: (appearance: TerminalAppearance) => void;
};

export function InterfaceSettings({ appearance, appearanceDraft, onAppearanceDraftChange, onSaveAppearance }: Props) {
  const [autoRefreshSeconds, setAutoRefreshSeconds] = useState(readAutoRefreshSeconds);

  useEffect(() => {
    const onStorage = (event: StorageEvent) => {
      if (event.key === 'roaminal.filesystem.auto-refresh-seconds') setAutoRefreshSeconds(readAutoRefreshSeconds());
    };
    window.addEventListener('storage', onStorage);
    return () => window.removeEventListener('storage', onStorage);
  }, []);

  function changeAutoRefresh(value: number) {
    writeAutoRefreshSeconds(value);
    setAutoRefreshSeconds(value);
  }

  return (
    <div className="settings-interface-layout">
      <section className="settings-panel settings-appearance-panel" aria-labelledby="settings-interface-appearance-title">
        <header className="settings-panel-heading">
          <div>
            <p className="eyebrow">TERMINAL</p>
            <h2 id="settings-interface-appearance-title">Terminal appearance</h2>
            <p>Choose how terminal text is rendered in this browser.</p>
          </div>
        </header>
        <AppearanceControls
          appearance={appearance}
          draft={appearanceDraft}
          onDraftChange={onAppearanceDraftChange}
          onSave={onSaveAppearance}
        />
      </section>
      <section className="settings-panel settings-filesystem-panel" aria-labelledby="settings-interface-filesystem-title">
        <header className="settings-panel-heading">
          <div>
            <p className="eyebrow">FILESYSTEM</p>
            <h2 id="settings-interface-filesystem-title">FileSystem tree refresh</h2>
            <p>Refresh the active FileSystem tree while this page is visible.</p>
          </div>
          <Clock3 size={20} aria-hidden="true" />
        </header>
        <label className="settings-select-field" htmlFor="settings-filesystem-auto-refresh">
          <span>Automatic refresh interval</span>
          <select
            id="settings-filesystem-auto-refresh"
            name="filesystemAutoRefreshSeconds"
            value={autoRefreshSeconds}
            onChange={(event) => changeAutoRefresh(Number(event.target.value))}
          >
            {AUTO_REFRESH_OPTIONS.map((option) => <option key={option.seconds} value={option.seconds}>{option.label}</option>)}
          </select>
          <small>Current value: {autoRefreshLabel(autoRefreshSeconds)}. Off disables automatic refresh but keeps manual refresh available.</small>
        </label>
      </section>
    </div>
  );
}
