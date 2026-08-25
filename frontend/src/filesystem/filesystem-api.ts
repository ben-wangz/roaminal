import { api, apiResponse } from '../auth/auth-client';
import { RoaminalApiError } from '../api/http-client';
import type {
  DirectoryResult,
  FileMetadata,
  FileSystemError,
  LocalUploadFile,
  RootContext,
  UploadManifest,
  UploadStatus,
} from './filesystem-types';

async function requestFilesystemResponse(path: string, init: RequestInit = {}): Promise<Response> {
	try {
		return await apiResponse(path, init);
	} catch (error) {
		if (error instanceof RoaminalApiError) {
			const filesystemError = new Error(error.message) as FileSystemError;
			filesystemError.code = error.code;
			filesystemError.status = error.status;
			const details = error.details && typeof error.details === 'object' ? error.details as { root?: unknown } : undefined;
			filesystemError.root = details?.root as RootContext | undefined;
			throw filesystemError;
		}
		throw error;
	}
}

async function requestFilesystemJSON<T>(path: string, init: RequestInit = {}): Promise<T> {
	try {
		return await api<T>(path, init);
	} catch (error) {
		if (error instanceof RoaminalApiError) {
			const filesystemError = new Error(error.message) as FileSystemError;
			filesystemError.code = error.code;
			filesystemError.status = error.status;
			const details = error.details && typeof error.details === 'object' ? error.details as { root?: unknown } : undefined;
			filesystemError.root = details?.root as RootContext | undefined;
			throw filesystemError;
		}
		throw error;
	}
}

function query(instanceId: string, endpoint: string, pathValue: string, revision?: string, download = false): string {
  const params = new URLSearchParams();
  params.set('path', pathValue || '.');
  if (revision) params.set('rootRevision', revision);
  if (download) params.set('download', '1');
  return `/connection-instances/${encodeURIComponent(instanceId)}/filesystem/${endpoint}?${params.toString()}`;
}

export async function loadRoot(instanceId: string, signal?: AbortSignal): Promise<RootContext> {
  const body = await requestFilesystemJSON<{ root: RootContext }>(`/connection-instances/${encodeURIComponent(instanceId)}/filesystem/root`, { signal });
  return body.root;
}

export async function loadEntries(instanceId: string, pathValue: string, revision: string, cursor?: string, signal?: AbortSignal): Promise<DirectoryResult> {
  const params = new URLSearchParams({ path: pathValue || '.', rootRevision: revision, limit: '200' });
  if (cursor) params.set('cursor', cursor);
  return requestFilesystemJSON<DirectoryResult>(`/connection-instances/${encodeURIComponent(instanceId)}/filesystem/entries?${params.toString()}`, { signal });
}

export async function loadMetadata(instanceId: string, pathValue: string, revision: string, signal?: AbortSignal): Promise<FileMetadata> {
  const body = await requestFilesystemJSON<{ entry: FileMetadata; mimeType: string; encoding: string; capabilities: FileMetadata['capabilities']; consistencyToken: string }>(query(instanceId, 'stat', pathValue, revision), { signal });
  return { ...body.entry, mimeType: body.mimeType, encoding: body.encoding, capabilities: body.capabilities, consistencyToken: body.consistencyToken };
}

export async function readContent(instanceId: string, pathValue: string, revision: string, range?: { start: number; end?: number }, download = false, signal?: AbortSignal): Promise<{ response: Response; data: ArrayBuffer }> {
  const headers = new Headers();
  if (range) headers.set('Range', `bytes=${range.start}-${range.end ?? ''}`);
  const response = await requestFilesystemResponse(query(instanceId, 'content', pathValue, revision, download), { headers, signal });
  return { response, data: await response.arrayBuffer() };
}

export async function createUpload(
  instanceId: string,
  manifest: UploadManifest,
  files: LocalUploadFile[],
  signal?: AbortSignal,
): Promise<UploadStatus> {
  const form = new FormData();
  form.append('manifest', JSON.stringify(manifest));
  files.forEach((item, index) => form.append(`file-${index}`, item.file, item.relativePath));
  return requestFilesystemJSON<UploadStatus>(`/connection-instances/${encodeURIComponent(instanceId)}/filesystem/uploads`, { method: 'POST', body: form, signal });
}

export async function loadUpload(instanceId: string, uploadId: string): Promise<UploadStatus> {
  return requestFilesystemJSON<UploadStatus>(`/connection-instances/${encodeURIComponent(instanceId)}/filesystem/uploads/${encodeURIComponent(uploadId)}`);
}

export async function cancelUpload(instanceId: string, uploadId: string): Promise<UploadStatus> {
  return requestFilesystemJSON<UploadStatus>(`/connection-instances/${encodeURIComponent(instanceId)}/filesystem/uploads/${encodeURIComponent(uploadId)}`, { method: 'DELETE' });
}
