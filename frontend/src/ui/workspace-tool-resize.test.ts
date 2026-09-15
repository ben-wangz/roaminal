import { describe, expect, it } from 'vitest';
import {
  clampWorkspaceToolWidth,
  loadWorkspaceToolWidth,
  saveWorkspaceToolWidth,
  workspaceToolWidthFromPointer,
  workspaceToolWidthBounds,
  WORKSPACE_TOOL_WIDTH_STORAGE_KEY,
} from './workspace-tool-resize';

function storage(initial: Record<string, string> = {}): Storage {
  const values = new Map(Object.entries(initial));
  return {
    getItem: (key) => values.get(key) || null,
    setItem: (key, value) => { values.set(key, value); },
    removeItem: (key) => { values.delete(key); },
    clear: () => { values.clear(); },
    key: (index) => [...values.keys()][index] || null,
    get length() { return values.size; },
  };
}

describe('workspace tool resize', () => {
  it('keeps a desktop panel within its minimum and available maximum', () => {
    expect(workspaceToolWidthBounds(1440)).toEqual({ min: 300, max: 560 });
    expect(workspaceToolWidthBounds(801)).toEqual({ min: 300, max: 405 });
    expect(clampWorkspaceToolWidth(250, 1440)).toBe(300);
    expect(clampWorkspaceToolWidth(700, 1440)).toBe(560);
    expect(clampWorkspaceToolWidth(404.6, 801)).toBe(405);
    expect(workspaceToolWidthFromPointer(360, 100, 140, 1440)).toBe(400);
    expect(workspaceToolWidthFromPointer(400, 100, 160, 1440)).toBe(460);
  });

  it('loads and persists a shared panel width', () => {
    const browserStorage = storage({ [WORKSPACE_TOOL_WIDTH_STORAGE_KEY]: '417' });
    expect(loadWorkspaceToolWidth(browserStorage)).toBe(417);
    expect(saveWorkspaceToolWidth(browserStorage, 421.6)).toBe(true);
    expect(browserStorage.getItem(WORKSPACE_TOOL_WIDTH_STORAGE_KEY)).toBe('422');
  });

  it('ignores invalid or unavailable storage values', () => {
    expect(loadWorkspaceToolWidth(storage({ [WORKSPACE_TOOL_WIDTH_STORAGE_KEY]: 'nope' }))).toBeNull();
    expect(loadWorkspaceToolWidth(null)).toBeNull();
    expect(saveWorkspaceToolWidth(null, 400)).toBe(false);
  });
});
