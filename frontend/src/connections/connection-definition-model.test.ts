import { describe, expect, it } from 'vitest';
import { bodyFrom, draftFrom, emptyDraft } from './connection-definition-model';

describe('connection FileSystem settings', () => {
  it('defaults the fallback pwd to $HOME and sends it with a definition', () => {
    expect(emptyDraft.filesystemPwd).toBe('$HOME');
    expect(bodyFrom(emptyDraft).filesystem).toEqual({ pwd: '$HOME' });
  });

  it('round trips a configured fallback pwd', () => {
    const draft = draftFrom({
      connectionDefinitionId: 'id',
      type: 'ssh',
      hostName: 'host',
      user: 'root',
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
      filesystem: { pwd: '/srv/project' },
    });
    expect(draft.filesystemPwd).toBe('/srv/project');
    expect(bodyFrom(draft).filesystem).toEqual({ pwd: '/srv/project' });
  });
});
