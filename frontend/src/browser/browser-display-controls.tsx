import { SlidersHorizontal } from 'lucide-react';
import type { BrowserDisplaySettings, ViewportSize } from './browser-runtime-model';

type Props = {
  settings: BrowserDisplaySettings;
  viewport: ViewportSize | null;
  disabled: boolean;
  onPageSizeChange: (value: string) => void;
  onFrameRateChange: (value: string) => void;
  onQualityChange: (value: string) => void;
};

export function BrowserDisplayControls({
  settings,
  viewport,
  disabled,
  onPageSizeChange,
  onFrameRateChange,
  onQualityChange,
}: Props) {
  return (
    <details className="browser-display-menu">
      <summary className="icon-button" aria-label="Browser display settings" title="Browser display settings" data-testid="browser-display-settings"><SlidersHorizontal size={17} /></summary>
      <div className="browser-display-panel">
        <label>
          <span>Page size</span>
          <select aria-label="Page size" value={settings.viewportMode === 'auto' ? 'auto' : `${viewport?.width || 1280}x${viewport?.height || 720}`} onChange={(event) => onPageSizeChange(event.target.value)} disabled={disabled}>
            <option value="auto">Automatic</option>
            <option value="1280x720">1280 × 720</option>
            <option value="1440x900">1440 × 900</option>
            <option value="1600x900">1600 × 900</option>
            <option value="1920x1080">1920 × 1080</option>
          </select>
        </label>
        <label>
          <span>Frame rate</span>
          <select aria-label="Frame rate" value={settings.frameRate} onChange={(event) => onFrameRateChange(event.target.value)} disabled={disabled}>
            <option value="5">5 fps</option>
            <option value="10">10 fps</option>
            <option value="15">15 fps</option>
          </select>
        </label>
        <label>
          <span>Image quality</span>
          <select aria-label="Image quality" value={settings.quality} onChange={(event) => onQualityChange(event.target.value)} disabled={disabled}>
            <option value="40">Low · 40%</option>
            <option value="55">Balanced · 55%</option>
            <option value="70">High · 70%</option>
          </select>
        </label>
      </div>
    </details>
  );
}
