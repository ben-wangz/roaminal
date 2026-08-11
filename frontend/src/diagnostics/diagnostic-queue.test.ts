import { describe, expect, it } from 'vitest';
import { DiagnosticQueue, type DiagnosticEvent } from './diagnostic-queue';

const event = (id: string, message = 'error', occurredAt = new Date(1_000).toISOString()): DiagnosticEvent => ({
  eventId: id,
  occurredAt,
  kind: 'console_error',
  message,
});

describe('DiagnosticQueue', () => {
  it('deduplicates recent events and increments queued repeat count', () => {
    const queue = new DiagnosticQueue();
    expect(queue.enqueue(event('one'), 1_000)).toBe(true);
    expect(queue.enqueue(event('two'), 2_000)).toBe(false);
    const batch = queue.take(20, 'page')!;
    expect(batch.events).toHaveLength(1);
    expect(batch.events[0].repeatCount).toBe(2);
  });

  it('returns batches and restores them after transport failure', () => {
    const queue = new DiagnosticQueue();
    queue.enqueue(event('one', 'one', new Date(Date.now()).toISOString()));
    const batch = queue.take(20, 'page')!;
    expect(queue.size()).toBe(0);
    queue.restore(batch);
    expect(queue.size()).toBe(1);
    expect(queue.take(20, 'page')?.events[0].eventId).toBe('one');
  });

  it('bounds event count and records dropped events', () => {
    const queue = new DiagnosticQueue();
    const now = Date.now();
    for (let index = 0; index < 110; index += 1) {
      queue.enqueue(event(`id-${index}`, `error-${index}`, new Date(now).toISOString()), now + index);
    }
    const batch = queue.take(20, 'page')!;
    expect(batch.events).toHaveLength(20);
    expect(batch.droppedCount).toBeGreaterThan(0);
  });

  it('expires old events', () => {
    const queue = new DiagnosticQueue();
    queue.enqueue(event('old', 'old', new Date(1_000).toISOString()), 1_000);
    queue.enqueue(event('new', 'new', new Date(700_000).toISOString()), 700_000);
    const batch = queue.take(20, 'page')!;
    expect(batch.events.map((value) => value.eventId)).toEqual(['new']);
  });
});
