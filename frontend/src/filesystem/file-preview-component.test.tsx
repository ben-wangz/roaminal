import { createElement } from 'react';
import { act, create, type ReactTestRenderer } from 'react-test-renderer';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { FilePreview } from './file-preview';
import type { FileEntry, FileMetadata, RootContext } from './filesystem-types';

const filesystemMocks = vi.hoisted(() => ({
	loadMetadata: vi.fn(),
	readContent: vi.fn(),
}));

vi.mock('./filesystem-api', () => filesystemMocks);

const root: RootContext = {
	connectionInstanceId: 'instance-1', absolutePath: '/workspace', relativePath: '.', source: 'configured',
	status: 'fallback', revision: 'revision-1', resolvedAt: '2026-09-01T00:00:00Z',
};

const entry: FileEntry = {
	name: 'image.png', relativePath: 'image.png', absolutePath: '/workspace/image.png', type: 'file', size: 12,
	modifiedAt: null, mode: 0o644, symlink: false,
};

const metadata: FileMetadata = {
	...entry, mimeType: 'image/png', encoding: 'utf-8',
	capabilities: { read: true, range: true, stream: true, download: true }, consistencyToken: 'token-1',
};

const textMetadata: FileMetadata = {
	...entry, name: 'README', relativePath: 'README', absolutePath: '/workspace/README', mimeType: 'text/plain', encoding: 'utf-8',
	capabilities: { read: true, range: true, stream: true, download: true }, consistencyToken: 'token-text',
};

function content(data: string, mimeType: string) {
	return { response: { headers: new Headers({ 'Content-Type': mimeType }) }, data: new TextEncoder().encode(data).buffer };
}

function props(onToast = vi.fn()) {
	return {
		instanceId: 'instance-1', root, entry, onBackToTerminal: vi.fn(), onToast, onRootChanged: vi.fn(),
	};
}

function textProps() {
	return {
		instanceId: 'instance-1', root, entry: { ...entry, name: 'README', relativePath: 'README', absolutePath: '/workspace/README' },
		onBackToTerminal: vi.fn(), onToast: vi.fn(), onRootChanged: vi.fn(),
	};
}

async function flushEffects() {
	await Promise.resolve();
	await Promise.resolve();
}

describe('FilePreview image lifecycle', () => {
	beforeEach(() => {
		vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true);
		let nextURL = 0;
		vi.stubGlobal('URL', {
			createObjectURL: vi.fn(() => `blob:image-${++nextURL}`),
			revokeObjectURL: vi.fn(),
		});
		filesystemMocks.loadMetadata.mockResolvedValue(metadata);
		filesystemMocks.readContent.mockResolvedValue(content('preview', 'image/webp'));
	});

	afterEach(() => {
		vi.clearAllMocks();
		vi.unstubAllGlobals();
	});

	it('loads the preview first and only requests the original from the icon action', async () => {
		let renderer: ReactTestRenderer | null = null;
		await act(async () => {
			renderer = create(createElement(FilePreview, props()));
			await flushEffects();
		});

		expect(filesystemMocks.readContent).toHaveBeenCalledWith('instance-1', 'image.png', 'revision-1', { variant: 'preview', signal: expect.any(AbortSignal) });
		expect(filesystemMocks.readContent).toHaveBeenCalledTimes(1);
		const image = renderer!.root.findByType('img');
		expect(image.props.src).toBe('blob:image-1');
		const viewOriginal = renderer!.root.findByProps({ 'data-testid': 'file-preview-view-original' });
		expect(viewOriginal.props['aria-label']).toBe('View original');
		expect(viewOriginal.props.disabled).toBe(false);
		expect(image.props.onClick).toBeUndefined();

		await act(async () => {
			viewOriginal.props.onClick();
			await flushEffects();
		});
		expect(filesystemMocks.readContent).toHaveBeenCalledTimes(2);
		expect(filesystemMocks.readContent.mock.calls[1][3]).toEqual({ variant: 'original', signal: expect.any(AbortSignal) });
		expect(renderer!.root.findByType('img').props.src).toBe('blob:image-2');
		expect(renderer!.root.findByProps({ 'data-testid': 'file-preview-view-original' }).props).toEqual(expect.objectContaining({ disabled: true, 'aria-label': 'Original loaded' }));
		await act(async () => renderer?.unmount());
	});

	it('supports zoom ratio changes and left-button image dragging', async () => {
		let renderer: ReactTestRenderer | null = null;
		await act(async () => {
			renderer = create(createElement(FilePreview, props()));
			await flushEffects();
		});

		const zoom = renderer!.root.findByProps({ 'data-testid': 'file-preview-image-zoom' });
		expect(zoom.props.value).toBe(100);
		await act(async () => zoom.props.onChange({ currentTarget: { value: '200' } }));
		expect(renderer!.root.findByProps({ 'data-testid': 'file-preview-image-zoom' }).props.value).toBe(200);
		expect(renderer!.root.findByType('img').props.style.transform).toContain('scale(2)');

		const pointerTarget = {
			setPointerCapture: vi.fn(),
			hasPointerCapture: vi.fn(() => true),
			releasePointerCapture: vi.fn(),
		};
		const preventDefault = vi.fn();
		const image = renderer!.root.findByType('img');
		await act(async () => image.props.onPointerDown({ button: 0, pointerId: 7, clientX: 10, clientY: 20, currentTarget: pointerTarget, preventDefault }));
		expect(renderer!.root.findByType('img').props.className).toContain('dragging');
		await act(async () => image.props.onPointerMove({ pointerId: 7, clientX: 30, clientY: 60, currentTarget: pointerTarget, preventDefault }));
		expect(renderer!.root.findByType('img').props.style.transform).toContain('translate3d(20px, 40px, 0) scale(2)');
		await act(async () => renderer!.root.findByType('img').props.onPointerUp({ pointerId: 7, currentTarget: pointerTarget }));
		expect(renderer!.root.findByType('img').props.className).not.toContain('dragging');
		expect(pointerTarget.setPointerCapture).toHaveBeenCalledWith(7);
		expect(pointerTarget.releasePointerCapture).toHaveBeenCalledWith(7);
		await act(async () => renderer?.unmount());
	});

	it('falls back to the original once when preview generation fails', async () => {
		filesystemMocks.readContent
			.mockRejectedValueOnce(new Error('preview unavailable'))
			.mockResolvedValueOnce(content('original', 'image/png'));
		let renderer: ReactTestRenderer | null = null;
		await act(async () => {
			renderer = create(createElement(FilePreview, props()));
			await flushEffects();
		});

		expect(filesystemMocks.readContent).toHaveBeenCalledTimes(2);
		expect(filesystemMocks.readContent.mock.calls.map((call) => call[3].variant)).toEqual(['preview', 'original']);
		expect(renderer!.root.findByType('img').props.src).toBe('blob:image-1');
		expect(renderer!.root.findByProps({ 'data-testid': 'file-preview-view-original' }).props['aria-label']).toBe('Original loaded');
		await act(async () => renderer?.unmount());
	});

	it('keeps the preview visible when loading the original fails', async () => {
		const onToast = vi.fn();
		filesystemMocks.readContent.mockResolvedValueOnce(content('preview', 'image/webp')).mockRejectedValueOnce(new Error('original unavailable'));
		let renderer: ReactTestRenderer | null = null;
		await act(async () => {
			renderer = create(createElement(FilePreview, props(onToast)));
			await flushEffects();
		});
		const viewOriginal = renderer!.root.findByProps({ 'data-testid': 'file-preview-view-original' });
		await act(async () => {
			viewOriginal.props.onClick();
			await flushEffects();
		});

		expect(renderer!.root.findByType('img').props.src).toBe('blob:image-1');
		expect(onToast).toHaveBeenCalledWith('Unable to load original image.', 'error');
		expect(renderer!.root.findByProps({ 'data-testid': 'file-preview-view-original' }).props.disabled).toBe(false);
		await act(async () => renderer?.unmount());
	});
});

describe('FilePreview detected text', () => {
	beforeEach(() => {
		vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true);
		filesystemMocks.loadMetadata.mockResolvedValue(textMetadata);
		filesystemMocks.readContent.mockResolvedValue(content('detected text', 'text/plain'));
	});

	afterEach(() => {
		vi.clearAllMocks();
		vi.unstubAllGlobals();
	});

	it('renders text detected by MIME for an extensionless file', async () => {
		let renderer: ReactTestRenderer | null = null;
		await act(async () => {
			renderer = create(createElement(FilePreview, textProps()));
			await flushEffects();
		});

		expect(filesystemMocks.readContent).toHaveBeenCalledWith('instance-1', 'README', 'revision-1', { signal: expect.any(AbortSignal) });
		expect(renderer!.root.findByType('pre').props.children).toBe('detected text');
		await act(async () => renderer?.unmount());
	});
});
