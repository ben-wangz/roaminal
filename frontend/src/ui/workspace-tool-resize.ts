export const WORKSPACE_TOOL_WIDTH_STORAGE_KEY = 'roaminal.workspace-tool-width';
export const WORKSPACE_TOOL_MIN_WIDTH = 300;
export const WORKSPACE_TOOL_MAX_WIDTH = 560;
export const WORKSPACE_TOOL_RAIL_WIDTH = 76;
export const WORKSPACE_MAIN_MIN_WIDTH = 320;
export const DEFAULT_WORKSPACE_TOOL_WIDTH = 360;

export type WorkspaceToolWidthBounds = {
  min: number;
  max: number;
};

export function workspaceToolWidthBounds(availableWidth: number): WorkspaceToolWidthBounds {
  const safeAvailableWidth = Number.isFinite(availableWidth) ? Math.max(0, availableWidth) : 0;
  const max = Math.max(
    WORKSPACE_TOOL_MIN_WIDTH,
    Math.min(WORKSPACE_TOOL_MAX_WIDTH, safeAvailableWidth - WORKSPACE_TOOL_RAIL_WIDTH - WORKSPACE_MAIN_MIN_WIDTH),
  );
  return { min: WORKSPACE_TOOL_MIN_WIDTH, max };
}

export function clampWorkspaceToolWidth(width: number, availableWidth: number): number {
  const bounds = workspaceToolWidthBounds(availableWidth);
  return Math.min(bounds.max, Math.max(bounds.min, Math.round(width)));
}

export function workspaceToolWidthFromPointer(startWidth: number, startX: number, clientX: number, availableWidth: number): number {
  return clampWorkspaceToolWidth(startWidth + clientX - startX, availableWidth);
}

export function loadWorkspaceToolWidth(storage: Storage | null): number | null {
  if (!storage) return null;
  try {
    const value = Number.parseInt(storage.getItem(WORKSPACE_TOOL_WIDTH_STORAGE_KEY) || '', 10);
    return Number.isFinite(value) && value > 0 ? value : null;
  } catch {
    return null;
  }
}

export function saveWorkspaceToolWidth(storage: Storage | null, width: number): boolean {
  if (!storage || !Number.isFinite(width)) return false;
  try {
    storage.setItem(WORKSPACE_TOOL_WIDTH_STORAGE_KEY, String(Math.round(width)));
    return true;
  } catch {
    return false;
  }
}
