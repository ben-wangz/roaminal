import { afterEach, describe, expect, it, vi } from 'vitest';
import { readContent } from './filesystem-api';

describe('filesystem content API', () => {
	afterEach(() => vi.unstubAllGlobals());

	it('encodes preview variants and byte ranges without positional options', async () => {
		const calls: Array<{ input: RequestInfo | URL; init?: RequestInit }> = [];
		vi.stubGlobal('fetch', vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
			calls.push({ input, init });
			return new Response(new Uint8Array([1, 2, 3]), { status: 200 });
		}));
		const controller = new AbortController();

		await readContent('instance/1', 'image.png', 'revision/1', { variant: 'preview', range: { start: 2, end: 5 }, signal: controller.signal });

		expect(calls[0].input).toBe('/api/v2/connection-instances/instance%2F1/filesystem/content?path=image.png&rootRevision=revision%2F1&variant=preview');
		const headers = calls[0].init?.headers as Headers;
		expect(headers.get('Range')).toBe('bytes=2-5');
		expect(calls[0].init?.signal).toBe(controller.signal);
	});

	it('makes downloads explicit and independent of the displayed variant', async () => {
		const calls: Array<RequestInfo | URL> = [];
		vi.stubGlobal('fetch', vi.fn(async (input: RequestInfo | URL) => {
			calls.push(input);
			return new Response(new Uint8Array([1]), { status: 200 });
		}));

		await readContent('instance-1', 'image.png', 'revision-1', { variant: 'preview', download: true });

		expect(calls[0]).toBe('/api/v2/connection-instances/instance-1/filesystem/content?path=image.png&rootRevision=revision-1&variant=preview&download=1');
	});
});
