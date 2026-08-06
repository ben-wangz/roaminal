import test from 'node:test';
import assert from 'node:assert/strict';

test('worker protocol fixture identifies the required operations', async () => {
  const fixture = await import('../../testdata/worker/worker-fixtures.json', { with: { type: 'json' } });
  assert.equal(fixture.default.protocol, 'roaminal-terminal-worker/1');
  assert.ok(fixture.default.operations.includes('snapshot'));
});
