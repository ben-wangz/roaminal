import { loadEntries } from './filesystem-api';
import type { FileEntry } from './filesystem-types';

export async function fetchAllEntries(
  instanceId: string,
  pathValue: string,
  revision: string,
  signal?: AbortSignal,
): Promise<FileEntry[]> {
  const all: FileEntry[] = [];
  let cursor: string | undefined;
  do {
    const result = await loadEntries(instanceId, pathValue, revision, cursor, signal);
    all.push(...result.entries);
    cursor = result.nextCursor || undefined;
  } while (cursor);
  return all;
}

export async function runWithConcurrency<T>(
  items: string[],
  limit: number,
  worker: (item: string) => Promise<T>,
): Promise<T[]> {
  const results = new Array<T>(items.length);
  let nextIndex = 0;
  const run = async () => {
    while (nextIndex < items.length) {
      const index = nextIndex;
      nextIndex += 1;
      results[index] = await worker(items[index]);
    }
  };
  await Promise.all(Array.from({ length: Math.min(limit, items.length) }, () => run()));
  return results;
}
