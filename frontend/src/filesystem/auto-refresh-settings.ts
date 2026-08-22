export const AUTO_REFRESH_OPTIONS = [
  { seconds: 0, label: 'Off' },
  { seconds: 30, label: '30 seconds' },
  { seconds: 60, label: '60 seconds' },
  { seconds: 120, label: '2 minutes' },
  { seconds: 300, label: '5 minutes' },
] as const;

const AUTO_REFRESH_STORAGE_KEY = 'roaminal.filesystem.auto-refresh-seconds';
const DEFAULT_AUTO_REFRESH_SECONDS = 60;

export function autoRefreshLabel(seconds: number): string {
  return AUTO_REFRESH_OPTIONS.find((option) => option.seconds === seconds)?.label || `${DEFAULT_AUTO_REFRESH_SECONDS} seconds`;
}

export function readAutoRefreshSeconds(): number {
  if (typeof window === 'undefined') return DEFAULT_AUTO_REFRESH_SECONDS;
  const value = Number.parseInt(window.localStorage.getItem(AUTO_REFRESH_STORAGE_KEY) || '', 10);
  return AUTO_REFRESH_OPTIONS.some((option) => option.seconds === value) ? value : DEFAULT_AUTO_REFRESH_SECONDS;
}

export function writeAutoRefreshSeconds(seconds: number): void {
  if (typeof window === 'undefined') return;
  window.localStorage.setItem(AUTO_REFRESH_STORAGE_KEY, String(seconds));
}
