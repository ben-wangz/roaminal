export type BrowserRuntimeStatus = 'idle' | 'connecting' | 'connected' | 'reconnecting' | 'error' | 'closed';

export type BrowserFrame = {
  data: ArrayBuffer;
  width: number;
  height: number;
  sequence: number;
};

export type BrowserDialog = {
  kind: 'alert' | 'confirm' | 'prompt' | 'beforeunload';
  message: string;
  defaultPrompt: string;
};

export type BrowserRuntimeState = {
  status: BrowserRuntimeStatus;
  title: string;
  url: string;
  error: string | null;
  viewport: { width: number; height: number } | null;
  frame: BrowserFrame | null;
  dialog: BrowserDialog | null;
  generation: string | null;
  isPrimaryClient: boolean;
  primaryError: string | null;
  takeoverPending: boolean;
};

export type BrowserMessage = {
  type?: string;
  status?: string;
  title?: string;
  url?: string;
  error?: string;
  code?: string;
  width?: number;
  height?: number;
  sequence?: number;
  data?: string;
  generation?: string;
  primary?: boolean;
  takeover?: boolean;
  kind?: BrowserDialog['kind'];
  message?: string;
  defaultPrompt?: string;
};

export type ViewportSize = { width: number; height: number };

const MIN_VIEWPORT_WIDTH = 1;
const MAX_VIEWPORT_WIDTH = 3840;
const MIN_VIEWPORT_HEIGHT = 1;
const MAX_VIEWPORT_HEIGHT = 2160;
export const RESIZE_COALESCE_MS = 16;

export function requestId(): string {
  return globalThis.crypto?.randomUUID?.() || `browser-${Date.now()}-${Math.random().toString(16).slice(2)}`;
}

export function decodeBase64(value: string): ArrayBuffer {
  const binary = atob(value);
  const output = new Uint8Array(binary.length);
  for (let index = 0; index < binary.length; index += 1) output[index] = binary.charCodeAt(index);
  return output.buffer;
}

export function validAddress(value: string): string | null {
  try {
    const parsed = new URL(value.trim());
    if (parsed.protocol !== 'http:' && parsed.protocol !== 'https:') return null;
    if (parsed.username || parsed.password || !parsed.hostname) return null;
    return parsed.toString();
  } catch {
    return null;
  }
}

export function normalizeViewport(width: number, height: number): ViewportSize | null {
  if (!Number.isFinite(width) || !Number.isFinite(height)) return null;
  const normalized = { width: Math.round(width), height: Math.round(height) };
  if (normalized.width < MIN_VIEWPORT_WIDTH || normalized.height < MIN_VIEWPORT_HEIGHT) return null;
  return {
    width: Math.min(MAX_VIEWPORT_WIDTH, normalized.width),
    height: Math.min(MAX_VIEWPORT_HEIGHT, normalized.height),
  };
}

export function viewportKey(size: ViewportSize): string {
  return `${size.width}x${size.height}`;
}
