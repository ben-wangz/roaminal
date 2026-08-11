import { describe, expect, it } from 'vitest';
import { serializeConsoleArguments } from './diagnostic-serialize';

describe('diagnostic serialization', () => {
  it('does not inspect arbitrary object properties', () => {
    let inspected = false;
    const value = Object.create(null) as { secret?: string };
    Object.defineProperty(value, 'secret', { get: () => { inspected = true; throw new Error('accessed'); } });
    const result = serializeConsoleArguments([new Error('boom'), value, documentNodePlaceholder()]);
    expect(result.message).toContain('Error: boom');
    expect(result.message).toContain('[Object Object]');
    expect(inspected).toBe(false);
  });

  it('bounds arguments and preserves primitive forms', () => {
    const result = serializeConsoleArguments(['text', 3, true, null, undefined, Symbol('x'), () => undefined]);
    expect(result.message).toContain('text 3 true null undefined [Symbol] [Function]');
  });
});

function documentNodePlaceholder(): unknown {
  return { constructor: { name: 'Object' } };
}
