export const API_PREFIX = '/api/v2';
export const WS_PREFIX = '/ws/v2';
export const WS_PROTOCOL = 'roaminal.v2';

export function apiPath(path: string): string {
	if (path.startsWith(`${API_PREFIX}/`) || path === API_PREFIX) return path;
	if (!path.startsWith('/')) return `${API_PREFIX}/${path}`;
	return `${API_PREFIX}${path}`;
}

export function websocketPath(endpoint: string, id: string): string {
  return `${WS_PREFIX}/${endpoint}/${encodeURIComponent(id)}`;
}
