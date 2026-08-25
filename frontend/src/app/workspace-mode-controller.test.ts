import { describe, expect, it } from 'vitest';
import { WorkspaceModeController } from './workspace-mode-controller';

describe('WorkspaceModeController', () => {
  it('keeps modes scoped to the connection instance', () => {
    const controller = new WorkspaceModeController();
    expect(controller.open('first', 'filesystem', null)).toEqual({ mode: 'filesystem', selectedConnectionInstanceId: 'first' });
    expect(controller.modeFor('first')).toBe('filesystem');
    expect(controller.modeFor('second')).toBe('terminal');
  });

  it('does not reselect an already active connection when changing its mode', () => {
    const controller = new WorkspaceModeController();
    expect(controller.open('first', 'filesystem', 'first')).toEqual({ mode: 'filesystem', selectedConnectionInstanceId: null });
  });
});
