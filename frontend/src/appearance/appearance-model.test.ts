import { describe, expect, it } from 'vitest';
import {
  APPEARANCE_STORAGE_KEY,
  DEFAULT_APPEARANCE,
  MAX_FONT_SIZE,
  MIN_FONT_SIZE,
  appearanceFromStorageEvent,
  deserializeAppearance,
  fontFamily,
  loadAppearance,
  normalizeFontSize,
  saveAppearance,
  serializeAppearance,
  validateAppearance,
  xtermFontOptions,
} from './appearance-model';

function storage(): Storage {
  const values = new Map<string, string>();
  return {
    get length() { return values.size; },
    clear: () => values.clear(),
    getItem: (key) => values.get(key) ?? null,
    key: (index) => [...values.keys()][index] ?? null,
    removeItem: (key) => values.delete(key),
    setItem: (key, value) => values.set(key, value),
  };
}

describe('appearance model', () => {
  it('round trips validated preferences and shares font options with xterm', () => {
    const value = { ...DEFAULT_APPEARANCE, fontId: 'jetbrains-mono' as const, fontSize: 22 };
    expect(deserializeAppearance(serializeAppearance(value))).toEqual(value);
    expect(fontFamily(value.fontId)).toContain('JetBrains Mono');
    expect(xtermFontOptions(value)).toEqual({ fontFamily: 'JetBrains Mono, monospace', fontSize: 22 });
    expect(xtermFontOptions(value, 10).fontSize).toBe(10);
  });

  it('rejects invalid schema, font IDs, and sizes', () => {
    expect(validateAppearance(null)).toBeNull();
    expect(validateAppearance({ ...DEFAULT_APPEARANCE, schemaVersion: 1 })).toBeNull();
    expect(validateAppearance({ ...DEFAULT_APPEARANCE, fontId: 'Comic Sans' })).toBeNull();
    expect(validateAppearance({ ...DEFAULT_APPEARANCE, fontSize: 15.5 })).toBeNull();
    expect(validateAppearance({ ...DEFAULT_APPEARANCE, fontSize: MIN_FONT_SIZE - 1 })).toBeNull();
    expect(validateAppearance({ ...DEFAULT_APPEARANCE, fontSize: MAX_FONT_SIZE + 1 })).toBeNull();
    expect(normalizeFontSize('18')).toBe(18);
    expect(normalizeFontSize('')).toBeNull();
    expect(normalizeFontSize('not-a-number')).toBeNull();
  });

  it('falls back for malformed storage and handles storage failures', () => {
    expect(deserializeAppearance('{')).toEqual(DEFAULT_APPEARANCE);
    const store = storage();
    const value = { ...DEFAULT_APPEARANCE, fontId: 'source-code-pro' as const, fontSize: 11 };
    expect(saveAppearance(store, value)).toBe(true);
    expect(loadAppearance(store)).toEqual(value);
    expect(store.getItem(APPEARANCE_STORAGE_KEY)).toContain('source-code-pro');
    const broken: Storage = { ...storage(), setItem: () => { throw new Error('quota'); }, getItem: () => { throw new Error('denied'); } };
    expect(saveAppearance(broken, value)).toBe(false);
    expect(loadAppearance(broken)).toEqual(DEFAULT_APPEARANCE);
  });

  it('parses only appearance storage events and resets on removal', () => {
    const event = (key: string, newValue: string | null) => ({ key, newValue }) as StorageEvent;
    expect(appearanceFromStorageEvent(event('other', '{}'))).toBeNull();
    expect(appearanceFromStorageEvent(event(APPEARANCE_STORAGE_KEY, null))).toEqual(DEFAULT_APPEARANCE);
    const value = { ...DEFAULT_APPEARANCE, fontId: 'system-monospace' as const, fontSize: 19 };
    expect(appearanceFromStorageEvent(event(APPEARANCE_STORAGE_KEY, serializeAppearance(value)))).toEqual(value);
  });
});
