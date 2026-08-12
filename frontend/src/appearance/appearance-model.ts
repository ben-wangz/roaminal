export const APPEARANCE_STORAGE_KEY = 'roaminal.appearance.v1';
export const APPEARANCE_SCHEMA_VERSION = 1 as const;
export const DEFAULT_FONT_SIZE = 15;
export const MIN_FONT_SIZE = 10;
export const MAX_FONT_SIZE = 32;

export const FONT_CATALOG = {
  'monaspace-neon': {
    label: 'Monaspace Neon',
    family: 'Monaspace Neon, monospace',
    bundled: true,
  },
  'jetbrains-mono': {
    label: 'JetBrains Mono',
    family: 'JetBrains Mono, monospace',
    bundled: true,
  },
  'source-code-pro': {
    label: 'Source Code Pro',
    family: 'Source Code Pro, monospace',
    bundled: true,
  },
  'system-monospace': {
    label: 'System Monospace',
    family: 'ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace',
    bundled: false,
  },
} as const;

export type TerminalFontId = keyof typeof FONT_CATALOG;

export type TerminalAppearance = {
  schemaVersion: typeof APPEARANCE_SCHEMA_VERSION;
  fontId: TerminalFontId;
  fontSize: number;
};

export const DEFAULT_APPEARANCE: TerminalAppearance = {
  schemaVersion: APPEARANCE_SCHEMA_VERSION,
  fontId: 'monaspace-neon',
  fontSize: DEFAULT_FONT_SIZE,
};

export function fontFamily(fontId: TerminalFontId): string {
  return FONT_CATALOG[fontId].family;
}

export function xtermFontOptions(appearance: TerminalAppearance, fixedFontSize?: number): Pick<TerminalAppearance, 'fontSize'> & { fontFamily: string } {
  return { fontFamily: fontFamily(appearance.fontId), fontSize: fixedFontSize ?? appearance.fontSize };
}

export function isTerminalFontId(value: unknown): value is TerminalFontId {
  return typeof value === 'string' && Object.prototype.hasOwnProperty.call(FONT_CATALOG, value);
}

export function normalizeFontSize(value: unknown): number | null {
  if (typeof value === 'string' && value.trim() !== '') value = Number(value);
  if (typeof value !== 'number' || !Number.isInteger(value) || value < MIN_FONT_SIZE || value > MAX_FONT_SIZE) {
    return null;
  }
  return value;
}

export function validateAppearance(value: unknown): TerminalAppearance | null {
  if (!value || typeof value !== 'object') return null;
  const record = value as Record<string, unknown>;
  const fontSize = normalizeFontSize(record.fontSize);
  if (record.schemaVersion !== APPEARANCE_SCHEMA_VERSION || !isTerminalFontId(record.fontId) || fontSize === null) {
    return null;
  }
  return { schemaVersion: APPEARANCE_SCHEMA_VERSION, fontId: record.fontId, fontSize };
}

export function deserializeAppearance(value: string | null): TerminalAppearance {
  if (!value) return DEFAULT_APPEARANCE;
  try {
    return validateAppearance(JSON.parse(value)) || DEFAULT_APPEARANCE;
  } catch {
    return DEFAULT_APPEARANCE;
  }
}

export function serializeAppearance(appearance: TerminalAppearance): string {
  const valid = validateAppearance(appearance) || DEFAULT_APPEARANCE;
  return JSON.stringify(valid);
}

export function loadAppearance(storage: Storage | null): TerminalAppearance {
  if (!storage) return DEFAULT_APPEARANCE;
  try {
    return deserializeAppearance(storage.getItem(APPEARANCE_STORAGE_KEY));
  } catch {
    return DEFAULT_APPEARANCE;
  }
}

export function browserAppearanceStorage(): Storage | null {
  try {
    return typeof window === 'undefined' ? null : window.localStorage;
  } catch {
    return null;
  }
}

export function saveAppearance(storage: Storage | null, appearance: TerminalAppearance): boolean {
  if (!storage) return false;
  try {
    storage.setItem(APPEARANCE_STORAGE_KEY, serializeAppearance(appearance));
    return true;
  } catch {
    return false;
  }
}

export function appearanceFromStorageEvent(event: StorageEvent): TerminalAppearance | null {
  if (event.key !== APPEARANCE_STORAGE_KEY) return null;
  if (event.newValue === null) return DEFAULT_APPEARANCE;
  return deserializeAppearance(event.newValue);
}
