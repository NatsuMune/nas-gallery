// Typed API client for docs/API_CONTRACT.md.
//
// - Server base URL comes from storage (see ./storage.ts).
// - Authenticated calls carry `Authorization: Bearer <token>`.
// - A 401 on any authed endpoint fires the onUnauthorized hook (the app
//   clears the token and returns to the login screen) and throws ApiError.
// - Errors are always ApiError with a human-readable message parsed from the
//   JSON `{"error": ...}` body, plus status and Retry-After when present.

import {
  buildItemListQuery,
  withParams,
  type ItemListQuery,
} from './query';
import type {
  FavoriteResponse,
  FolderResponse,
  HealthResponse,
  Item,
  ItemPage,
  LibraryStatus,
  LoginResponse,
  MeResponse,
  ScanResponse,
} from './types';

export class ApiError extends Error {
  /** HTTP status; 0 means the request never got a response (network). */
  readonly status: number;
  readonly retryAfterSeconds: number | undefined;

  constructor(message: string, status: number, retryAfterSeconds?: number) {
    super(message);
    this.name = 'ApiError';
    this.status = status;
    this.retryAfterSeconds = retryAfterSeconds;
  }
}

export interface ApiClientHooks {
  getServerUrl(): string;
  getToken(): string | null;
  /** Called once when an authed call receives 401 (token expired/revoked). */
  onUnauthorized(): void;
}

/** Joins a server origin with a signed API path ("/api/media/..."). */
export function joinUrl(base: string, path: string): string {
  if (/^[a-z][a-z0-9+.-]*:/i.test(path) || path.startsWith('//')) return path;
  const trimmed = base.trim().replace(/\/+$/, '');
  if (!trimmed) return path;
  return trimmed + (path.startsWith('/') ? path : `/${path}`);
}

interface RequestOptions {
  method?: 'GET' | 'POST' | 'PUT' | 'DELETE';
  body?: unknown;
  /** false for public endpoints (health, login) — no Bearer header. */
  auth?: boolean;
  signal?: AbortSignal;
}

export function createApiClient(hooks: ApiClientHooks) {
  async function request<T>(path: string, opts: RequestOptions = {}): Promise<T> {
    const serverUrl = hooks.getServerUrl();
    const url = joinUrl(serverUrl, path);
    const headers: Record<string, string> = {};
    if (opts.body !== undefined) headers['Content-Type'] = 'application/json';
    const token = hooks.getToken();
    const auth = opts.auth !== false;
    if (auth && token) headers['Authorization'] = `Bearer ${token}`;

    let res: Response;
    try {
      res = await fetch(url, {
        method: opts.method ?? 'GET',
        headers,
        body: opts.body !== undefined ? JSON.stringify(opts.body) : undefined,
        signal: opts.signal,
      });
    } catch (err) {
      if (err instanceof DOMException && err.name === 'AbortError') throw err;
      throw new ApiError(`Could not reach the server at ${serverUrl || '(no server URL set)'}`, 0);
    }

    if (!res.ok) {
      let message = `Request failed (${res.status})`;
      try {
        const body: unknown = await res.json();
        if (body && typeof body === 'object' && typeof (body as { error?: unknown }).error === 'string') {
          message = (body as { error: string }).error;
        }
      } catch {
        // non-JSON error body — keep the generic message
      }
      const raHeader = res.headers.get('Retry-After');
      const retryAfterSeconds = raHeader !== null ? Number(raHeader) : undefined;
      if (res.status === 401 && auth && !path.startsWith('/api/auth/login')) {
        hooks.onUnauthorized();
      }
      throw new ApiError(message, res.status, Number.isFinite(retryAfterSeconds) ? retryAfterSeconds : undefined);
    }

    if (res.status === 204) return undefined as T;
    try {
      return (await res.json()) as T;
    } catch {
      throw new ApiError('Server returned an invalid response', 0);
    }
  }

  return {
    health: (signal?: AbortSignal) => request<HealthResponse>('/api/health', { auth: false, signal }),
    login: (username: string, password: string, rememberMe: boolean, signal?: AbortSignal) =>
      request<LoginResponse>('/api/auth/login', { method: 'POST', body: { username, password, rememberMe }, auth: false, signal }),
    logout: () => request<void>('/api/auth/logout', { method: 'POST' }),
    me: () => request<MeResponse>('/api/me'),
    folder: (id: string) => request<FolderResponse>(`/api/folders/${encodeURIComponent(id)}`),
    folderItems: (id: string, query: ItemListQuery = {}) =>
      request<ItemPage>(`/api/folders/${encodeURIComponent(id)}/items${buildItemListQuery(query)}`),
    item: (id: string) => request<Item>(`/api/items/${encodeURIComponent(id)}`),
    favorites: (query: ItemListQuery = {}) => request<ItemPage>(`/api/favorites${buildItemListQuery(query)}`),
    search: (q: string, query: ItemListQuery = {}) =>
      request<ItemPage>(`/api/search${withParams(buildItemListQuery(query), { q })}`),
    setFavorite: (id: string, favorite: boolean) =>
      request<FavoriteResponse>(`/api/items/${encodeURIComponent(id)}/favorite`, { method: 'PUT', body: { favorite } }),
    libraryStatus: () => request<LibraryStatus>('/api/library/status'),
    triggerScan: () => request<ScanResponse>('/api/library/scan', { method: 'POST' }),
  };
}

export type ApiClient = ReturnType<typeof createApiClient>;
