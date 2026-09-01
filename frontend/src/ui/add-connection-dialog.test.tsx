import { createElement } from 'react';
import { act, create, type ReactTestRenderer } from 'react-test-renderer';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { AddConnectionDialog } from './add-connection-dialog';
import type { ConnectionInstanceSummary } from '../terminal/terminal-protocol';

const connectionApiMocks = vi.hoisted(() => ({ loadDefinitions: vi.fn() }));
vi.mock('../connections/connection-api', () => connectionApiMocks);

const connection = {
  connectionInstanceId: 'instance-1',
  connectionDefinitionId: 'definition-1',
  type: 'ssh',
  lifecycle: 'live',
  sourceState: 'current',
  sourceHostAlias: 'dev',
  createdAt: '2026-08-31T00:00:00Z',
  updatedAt: '2026-08-31T00:00:00Z',
  title: 'dev',
  titleMode: 'automatic',
  cwd: '/workspace',
  cols: 80,
  rows: 24,
  attention: false,
} satisfies ConnectionInstanceSummary;

const definitions = {
  configSource: { status: 'available', readable: true, writable: true },
  definitions: [{
    connectionDefinitionId: 'definition-1',
    type: 'ssh',
    hostAlias: 'dev',
    hostName: 'dev.example.test',
    user: 'coder',
    port: 22,
    identityFileNames: [],
    identitiesOnly: null,
    strictHostKeyChecking: null,
    userKnownHostsFile: null,
    serverAliveInterval: null,
    advancedDirectiveCount: 0,
    unmanagedIdentityCount: 0,
    warnings: [],
    capabilities: {},
    hostVerificationAssessment: 'default',
    tmux: { enabled: true, sessionName: 'roaminal' },
  }],
};

describe('AddConnectionDialog', () => {
  beforeEach(() => {
    vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true);
    connectionApiMocks.loadDefinitions.mockResolvedValue({ data: definitions, etag: null });
  });

  afterEach(() => {
    vi.clearAllMocks();
    vi.unstubAllGlobals();
  });

  it('loads definitions and confirms through the existing launch callback', async () => {
    const onCreateConnection = vi.fn(async () => true);
    const onClose = vi.fn();
    let renderer: ReactTestRenderer | null = null;

    await act(async () => {
      renderer = create(createElement(AddConnectionDialog, {
        connections: [connection],
        onCreateConnection,
        onClose,
      }));
    });

    expect(connectionApiMocks.loadDefinitions).toHaveBeenCalledOnce();
    const select = renderer!.root.findByType('select');
    expect(select.props.name).toBe('connectionDefinition');
    expect(renderer!.root.findAllByType('option').map((option) => option.props.value)).toEqual(['', 'local', 'definition-1']);
    expect(renderer!.root.findAllByType('option')[2].props.children).toBe('dev');
    expect(renderer!.root.findAllByType('option')[2].props.children).not.toContain('dev.example.test');

    await act(async () => {
      select.props.onChange({ target: { value: 'definition-1' } });
    });
    const form = renderer!.root.findByType('form');
    await act(async () => {
      await form.props.onSubmit({ preventDefault: vi.fn() });
    });

    expect(onCreateConnection).toHaveBeenCalledWith('definition-1', 'instance-1', true);
    expect(onClose).toHaveBeenCalledOnce();
    await act(async () => renderer?.unmount());
  });

  it('keeps the dialog open when connection creation fails', async () => {
    const onCreateConnection = vi.fn(async () => false);
    const onClose = vi.fn();
    let renderer: ReactTestRenderer | null = null;

    await act(async () => {
      renderer = create(createElement(AddConnectionDialog, {
        connections: [],
        onCreateConnection,
        onClose,
      }));
    });
    const select = renderer!.root.findByType('select');
    await act(async () => select.props.onChange({ target: { value: 'local' } }));
    await act(async () => {
      await renderer!.root.findByType('form').props.onSubmit({ preventDefault: vi.fn() });
    });

    expect(onCreateConnection).toHaveBeenCalledWith('local', undefined, false);
    expect(onClose).not.toHaveBeenCalled();
    expect(renderer!.root.findAllByProps({ role: 'alert' }).length).toBeGreaterThan(0);
    await act(async () => renderer?.unmount());
  });
});
