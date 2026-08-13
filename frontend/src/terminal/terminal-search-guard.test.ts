import { describe, expect, it } from 'vitest';
import { findTerminalMatch, type TerminalSearchAddon, type TerminalSearchOptions } from './terminal-search-guard';

describe('terminal search guard', () => {
  it('delegates forward and backward searches with their options', () => {
    const calls: Array<[string, TerminalSearchOptions]> = [];
    const search: TerminalSearchAddon = {
      findNext: (query: string, options: TerminalSearchOptions = {}) => {
        calls.push([`next:${query}`, options]);
        return true;
      },
      findPrevious: (query: string, options: TerminalSearchOptions = {}) => {
        calls.push([`previous:${query}`, options]);
        return true;
      },
    };
    const options = { regex: true, wholeWord: false, caseSensitive: true };

    expect(findTerminalMatch(search, 'alpha', options, false)).toBe(true);
    expect(findTerminalMatch(search, 'alpha', options, true)).toBe(true);
    expect(calls).toEqual([
      ['next:alpha', options],
      ['previous:alpha', options],
    ]);
  });

  it('treats an unavailable or failing addon as no match', () => {
    expect(findTerminalMatch(undefined, 'alpha', {}, false)).toBe(false);
    const search: TerminalSearchAddon = {
      findNext: () => { throw new SyntaxError('invalid regex'); },
      findPrevious: () => { throw new SyntaxError('invalid regex'); },
    };
    expect(findTerminalMatch(search, '[', { regex: true }, false)).toBe(false);
    expect(findTerminalMatch(search, '[', { regex: true }, true)).toBe(false);
  });
});
