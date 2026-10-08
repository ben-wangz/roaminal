import { browserDisplaySettingsFromMessage, type BrowserMessage, type BrowserRuntimeState } from './browser-runtime-model';

type ResizeRejectionEffects = {
  clearResizeTimer: boolean;
  runtimeGeneration?: string | null;
  resetLastResizeKey: boolean;
  synchronize: boolean;
  state: Partial<BrowserRuntimeState>;
};

export function resizeRejectionEffects(
  message: BrowserMessage,
  currentGeneration: string | null,
  current: BrowserRuntimeState,
): ResizeRejectionEffects {
  const code = message.code || '';
  const viewport = message.width && message.height
    ? { width: message.width, height: message.height }
    : current.viewport;
  const displaySettings = browserDisplaySettingsFromMessage(message, current.displaySettings);
  let runtimeGeneration: string | null | undefined;
  let resetLastResizeKey = false;
  if (message.generation && message.generation !== currentGeneration) {
    runtimeGeneration = message.generation;
    resetLastResizeKey = true;
  }

  if (code === 'not_primary_client' || code === 'primary_client_required') {
    return {
      clearResizeTimer: true,
      runtimeGeneration,
      resetLastResizeKey,
      synchronize: false,
      state: {
        isPrimaryClient: false,
        generation: message.generation || current.generation,
        viewport,
        displaySettings,
        primaryError: message.error || 'Another browser client controls the viewport.',
        takeoverPending: false,
      },
    };
  }
  if (code === 'stale_browser_generation') {
    return {
      clearResizeTimer: true,
      runtimeGeneration: null,
      resetLastResizeKey,
      synchronize: true,
      state: {
        generation: null,
        viewport,
        displaySettings,
        takeoverPending: false,
        primaryError: message.error || 'The remote browser generation has changed.',
      },
    };
  }
  return {
    clearResizeTimer: false,
    runtimeGeneration,
    resetLastResizeKey,
    synchronize: false,
    state: {
      viewport,
      displaySettings,
      takeoverPending: false,
      primaryError: message.error || 'The remote browser rejected the viewport change.',
    },
  };
}
