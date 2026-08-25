import { apiPath } from './routes';

export type ApiErrorBody = {
  error?: string;
  code?: string;
  retryable?: boolean;
  field?: string;
  requestId?: string;
  details?: unknown;
};

export class RoaminalApiError extends Error {
  readonly code: string | undefined;
  readonly status: number;
  readonly retryable: boolean;
  readonly field: string | undefined;
  readonly requestId: string | undefined;
  readonly details: unknown;

  constructor(status: number, fallback: string, body: ApiErrorBody = {}) {
    super(body.error || fallback);
    this.name = 'RoaminalApiError';
    this.code = body.code;
    this.status = status;
    this.retryable = body.retryable === true;
    this.field = body.field;
    this.requestId = body.requestId;
    this.details = body.details;
  }
}

async function readError(response: Response): Promise<RoaminalApiError> {
  const body = await response.json().catch(() => ({})) as ApiErrorBody;
  return new RoaminalApiError(response.status, response.statusText, body);
}

export async function requestResponse(path: string, init: RequestInit = {}, accessToken?: string | null): Promise<Response> {
  const headers = new Headers(init.headers);
  if (accessToken) headers.set('Authorization', `Bearer ${accessToken}`);
  if (init.body !== undefined && !(init.body instanceof FormData) && !headers.has('Content-Type')) {
    headers.set('Content-Type', 'application/json');
  }
  const response = await fetch(apiPath(path), {
    ...init,
    headers,
    cache: init.cache || 'no-store',
  });
  if (!response.ok) throw await readError(response);
  return response;
}

export async function requestJSON<T>(path: string, init: RequestInit = {}, accessToken?: string | null): Promise<T> {
  const response = await requestResponse(path, init, accessToken);
  return response.status === 204 ? undefined as T : await response.json() as T;
}

export async function requestWithMeta<T>(path: string, init: RequestInit = {}, accessToken?: string | null): Promise<{ data: T; etag: string | null }> {
  const response = await requestResponse(path, init, accessToken);
  return {
    data: response.status === 204 ? undefined as T : await response.json() as T,
    etag: response.headers.get('ETag'),
  };
}
