import { loadAuth, refresh } from '../auth/auth-client';
import type {
  DirectoryResult,
  FileMetadata,
  FileSystemError,
  LocalUploadFile,
  RootContext,
  UploadManifest,
  UploadStatus,
} from './filesystem-types';

async function responseError(response: Response): Promise<FileSystemError> {
  const body = await response.json().catch(() => ({})) as { error?: string; code?: string; root?: RootContext };
  const error = new Error(body.error || response.statusText) as FileSystemError;
  error.code = body.code;
  error.status = response.status;
  error.root = body.root;
  return error;
}

async function requestResponse(path: string, init: RequestInit = {}, retried = false): Promise<Response> {
  const headers = new Headers(init.headers);
  const auth = loadAuth();
  if (auth?.accessToken) headers.set('Authorization', `Bearer ${auth.accessToken}`);
  if (!(init.body instanceof FormData) && init.body !== undefined) headers.set('Content-Type', 'application/json');
  const response = await fetch(path, { ...init, headers, cache: init.cache || 'no-store' });
  if (response.status === 401 && !retried && await refresh()) return requestResponse(path, init, true);
  if (!response.ok) throw await responseError(response);
  return response;
}

function query(instanceId: string, endpoint: string, pathValue: string, revision?: string, download = false): string {
  const params = new URLSearchParams();
  params.set('path', pathValue || '.');
  if (revision) params.set('rootRevision', revision);
  if (download) params.set('download', '1');
  return `/api/connection-instances/${encodeURIComponent(instanceId)}/filesystem/${endpoint}?${params.toString()}`;
}

export async function loadRoot(instanceId: string, signal?: AbortSignal): Promise<RootContext> {
  const response = await requestResponse(`/api/connection-instances/${encodeURIComponent(instanceId)}/filesystem/root`, { signal });
  const body = await response.json() as { root: RootContext };
  return body.root;
}

export async function loadEntries(instanceId: string, pathValue: string, revision: string, cursor?: string, signal?: AbortSignal): Promise<DirectoryResult> {
  const params = new URLSearchParams({ path: pathValue || '.', rootRevision: revision, limit: '200' });
  if (cursor) params.set('cursor', cursor);
  const response = await requestResponse(`/api/connection-instances/${encodeURIComponent(instanceId)}/filesystem/entries?${params.toString()}`, { signal });
  return await response.json() as DirectoryResult;
}

export async function loadMetadata(instanceId: string, pathValue: string, revision: string, signal?: AbortSignal): Promise<FileMetadata> {
  const response = await requestResponse(query(instanceId, 'stat', pathValue, revision), { signal });
  const body = await response.json() as { entry: FileMetadata; mimeType: string; encoding: string; capabilities: FileMetadata['capabilities']; consistencyToken: string };
  return { ...body.entry, mimeType: body.mimeType, encoding: body.encoding, capabilities: body.capabilities, consistencyToken: body.consistencyToken };
}

export async function readContent(instanceId: string, pathValue: string, revision: string, range?: { start: number; end?: number }, download = false, signal?: AbortSignal): Promise<{ response: Response; data: ArrayBuffer }> {
  const headers = new Headers();
  if (range) headers.set('Range', `bytes=${range.start}-${range.end ?? ''}`);
  const response = await requestResponse(query(instanceId, 'content', pathValue, revision, download), { headers, signal });
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
  const response = await requestResponse(`/api/connection-instances/${encodeURIComponent(instanceId)}/filesystem/uploads`, { method: 'POST', body: form, signal });
  return await response.json() as UploadStatus;
}

export async function loadUpload(instanceId: string, uploadId: string): Promise<UploadStatus> {
  const response = await requestResponse(`/api/connection-instances/${encodeURIComponent(instanceId)}/filesystem/uploads/${encodeURIComponent(uploadId)}`);
  return await response.json() as UploadStatus;
}

export async function cancelUpload(instanceId: string, uploadId: string): Promise<UploadStatus> {
  const response = await requestResponse(`/api/connection-instances/${encodeURIComponent(instanceId)}/filesystem/uploads/${encodeURIComponent(uploadId)}`, { method: 'DELETE' });
  return await response.json() as UploadStatus;
}
