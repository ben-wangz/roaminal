import type { SearchAddon } from '@xterm/addon-search';

export type TerminalSearchOptions = { regex?: boolean; wholeWord?: boolean; caseSensitive?: boolean };
export type TerminalSearchAddon = Pick<SearchAddon, 'findNext' | 'findPrevious'>;

export function findTerminalMatch(
  search: TerminalSearchAddon | undefined,
  query: string,
  options: TerminalSearchOptions,
  previous: boolean,
): boolean {
  try {
    return previous ? search?.findPrevious(query, options) ?? false : search?.findNext(query, options) ?? false;
  } catch {
    return false;
  }
}
