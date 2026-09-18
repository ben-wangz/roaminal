// Chromium writes crash reports and profile metadata outside --user-data-dir;
// keep those writes inside the chart-provided writable /tmp volume.
process.env.HOME = '/tmp';
process.env.XDG_CONFIG_HOME = '/tmp/roaminal-browser-config';
process.env.XDG_CACHE_HOME = '/tmp/roaminal-browser-cache';

export const chromiumPath = process.env.ROAMINAL_BROWSER_CHROMIUM_PATH || 'chromium';
export const viewport = { width: 1280, height: 720 };
export const state = {
  browserProcess: null,
  profilePath: null,
  cdp: null,
  target: null,
  pageSession: null,
  mainFrameId: '',
  activeLoaderId: '',
  lifecycleEventsEnabled: false,
  sequence: 0,
  lastFrameAt: 0,
  screencasting: false,
  blockedNavigation: false,
  desiredVisibility: false,
  pageGeneration: '',
  pageOperation: 0,
  pageRevision: 0,
  pageStatus: 'none',
  pageURL: '',
  pageTitle: '',
  pageError: null,
  dialogState: null,
  eventUnsubscribe: null,
};

export function emit(message) {
  process.stdout.write(`${JSON.stringify(message)}\n`);
}

export function operationValue(value) {
  const parsed = Number(value);
  return Number.isInteger(parsed) && parsed >= 0 ? parsed : null;
}

export function acceptOperation(message, advance = false) {
  const requested = operationValue(message.pageOperation);
  if (requested !== null) {
    if (requested < state.pageOperation) return false;
    state.pageOperation = requested;
    return true;
  }
  if (advance) state.pageOperation += 1;
  return true;
}

export function modifiers(value = {}) {
  return (value.alt ? 1 : 0) | (value.ctrl ? 2 : 0) | (value.meta ? 4 : 0) | (value.shift ? 8 : 0);
}

export function allowedDocumentURL(value) {
  try {
    const parsed = new URL(value);
    if (parsed.protocol === 'http:' || parsed.protocol === 'https:' || parsed.protocol === 'data:' || parsed.protocol === 'blob:') return true;
    return parsed.protocol === 'about:' && parsed.pathname === 'blank';
  } catch {
    return false;
  }
}
