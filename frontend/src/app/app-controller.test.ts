import { describe, expect, it } from 'vitest';
import { createAppController } from './app-controller';

describe('app controller', () => {
  it('publishes workspace state transitions through one subscription', () => {
    const controller = createAppController();
    const snapshots: string[] = [];
    const unsubscribe = controller.subscribe(() => snapshots.push(controller.getSnapshot().page));

    controller.setPage('workspace');
    controller.setWorkspaceTool('keyboard');
    controller.setWorkspaceToolOpen(true);
    controller.setPreviewConnectionInstanceId('instance-1');

    expect(controller.getSnapshot()).toMatchObject({
      page: 'workspace',
      workspaceTool: 'keyboard',
      workspaceToolOpen: true,
      previewConnectionInstanceId: 'instance-1',
    });
    expect(snapshots).toEqual(['workspace', 'workspace', 'workspace', 'workspace']);
    unsubscribe();
    controller.setSearch(true);
    expect(snapshots).toHaveLength(4);
  });

  it('supports functional field updates without exposing mutable state', () => {
    const controller = createAppController();
    const before = controller.getSnapshot();
    controller.setState((current) => ({ ...current, search: !current.search }));
    expect(before.search).toBe(false);
    expect(controller.getSnapshot().search).toBe(true);
  });
});
