import { describe, expect, it } from 'vitest';
import { validAddress } from './browser-runtime';

describe('browser address validation', () => {
  it('accepts cluster HTTP(S) addresses and normalizes them', () => {
    expect(validAddress('  http://service.namespace:8080/path  ')).toBe('http://service.namespace:8080/path');
    expect(validAddress('https://service.namespace')).toBe('https://service.namespace/');
  });

  it('rejects schemes and credentials that must not reach Chromium', () => {
    expect(validAddress('service.namespace:8080')).toBeNull();
    expect(validAddress('file:///tmp/page')).toBeNull();
    expect(validAddress('https://user:password@service.namespace')).toBeNull();
  });
});
