import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { ApiError, createApiClient, joinUrl } from './client';

// The client under test, wired to stubbed storage + a mocked fetch.

let serverUrl = 'http://nas.example.com';
let token: string | null = null;
let unauthorizedCalls = 0;
let fetchMock: ReturnType<typeof vi.fn>;

function api() {
  return createApiClient({
    getServerUrl: () => serverUrl,
    getToken: () => token,
    onUnauthorized: () => {
      unauthorizedCalls += 1;
    },
  });
}

function jsonResponse(status: number, body: unknown, headers: Record<string, string> = {}) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'Content-Type': 'application/json', ...headers },
  });
}

beforeEach(() => {
  serverUrl = 'http://nas.example.com';
  token = 'token-abc';
  unauthorizedCalls = 0;
  fetchMock = vi.fn();
  vi.stubGlobal('fetch', fetchMock);
});

afterEach(() => {
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

describe('joinUrl', () => {
  it('prefixes the server origin to signed paths', () => {
    expect(joinUrl('http://nas.example.com', '/api/media/m_1/thumb?s=grid')).toBe(
      'http://nas.example.com/api/media/m_1/thumb?s=grid',
    );
  });

  it('tolerates trailing slashes on the base', () => {
    expect(joinUrl('http://nas.example.com/', '/api/health')).toBe('http://nas.example.com/api/health');
    expect(joinUrl('http://nas.example.com///', '/api/health')).toBe('http://nas.example.com/api/health');
  });

  it('keeps subpath bases', () => {
    expect(joinUrl('https://host.example.com/nas', '/api/health')).toBe(
      'https://host.example.com/nas/api/health',
    );
  });

  it('returns absolute URLs and non-http schemes untouched', () => {
    expect(joinUrl('http://nas.example.com', 'https://other.example.com/x')).toBe('https://other.example.com/x');
    expect(joinUrl('http://nas.example.com', 'blob:whatever')).toBe('blob:whatever');
  });
});

describe('auth header', () => {
  it('sends the Bearer token on authed calls', async () => {
    fetchMock.mockResolvedValueOnce(jsonResponse(200, { user: { id: 1, username: 'zach' }, server: { name: 'NAS', version: '1' } }));
    await api().me();
    const [, init] = fetchMock.mock.calls[0] as [string, RequestInit & { headers: Record<string, string> }];
    expect(init.headers['Authorization']).toBe('Bearer token-abc');
  });

  it('omits the Bearer token on public calls (health, login)', async () => {
    fetchMock.mockResolvedValueOnce(jsonResponse(200, { ok: true, version: '1.0.0' }));
    await api().health();
    fetchMock.mockResolvedValueOnce(jsonResponse(200, { token: 'x'.repeat(64), expiresAt: '2026-01-01T00:00:00Z' }));
    await api().login('u', 'p', false);
    for (const call of fetchMock.mock.calls as [string, RequestInit & { headers: Record<string, string> }][]) {
      expect(call[1].headers['Authorization']).toBeUndefined();
    }
  });
});

describe('error handling', () => {
  it('parses the JSON error body into ApiError', async () => {
    fetchMock.mockResolvedValueOnce(jsonResponse(404, { error: 'Folder not found' }));
    const err = await api().folder('f_nope').catch((e: unknown) => e);
    expect(err).toBeInstanceOf(ApiError);
    expect((err as ApiError).message).toBe('Folder not found');
    expect((err as ApiError).status).toBe(404);
  });

  it('exposes Retry-After on rate limit responses', async () => {
    fetchMock.mockResolvedValueOnce(
      jsonResponse(429, { error: 'Too many failed attempts. Try again later.' }, { 'Retry-After': '60' }),
    );
    const err = await api().login('u', 'p', false).catch((e: unknown) => e);
    expect((err as ApiError).status).toBe(429);
    expect((err as ApiError).retryAfterSeconds).toBe(60);
  });

  it('wraps network failures as status-0 ApiError mentioning the server', async () => {
    fetchMock.mockRejectedValueOnce(new TypeError('fetch failed'));
    const err = await api().me().catch((e: unknown) => e);
    expect(err).toBeInstanceOf(ApiError);
    expect((err as ApiError).status).toBe(0);
    expect((err as ApiError).message).toContain('http://nas.example.com');
  });
});

describe('401 handling', () => {
  it('fires onUnauthorized on a 401 from an authed endpoint and throws', async () => {
    fetchMock.mockResolvedValueOnce(jsonResponse(401, { error: 'Missing or invalid token' }));
    const err = await api().me().catch((e: unknown) => e);
    expect(err).toBeInstanceOf(ApiError);
    expect((err as ApiError).status).toBe(401);
    expect(unauthorizedCalls).toBe(1);
  });

  it('does not fire onUnauthorized for a failed login (401 invalid credentials)', async () => {
    fetchMock.mockResolvedValueOnce(jsonResponse(401, { error: 'Invalid username or password' }));
    await api().login('u', 'wrong', false).catch(() => undefined);
    expect(unauthorizedCalls).toBe(0);
  });

  it('handles 204 No Content', async () => {
    fetchMock.mockResolvedValueOnce(new Response(null, { status: 204 }));
    const result = await api().logout();
    expect(result).toBeUndefined();
  });
});

describe('request bodies and URLs', () => {
  it('login posts the credentials as JSON', async () => {
    fetchMock.mockResolvedValueOnce(jsonResponse(200, { token: 't'.repeat(64), expiresAt: 'x' }));
    await api().login('zach', 'secret', true);
    const [url, init] = fetchMock.mock.calls[0] as [string, RequestInit & { headers: Record<string, string> }];
    expect(url).toBe('http://nas.example.com/api/auth/login');
    expect(init.method).toBe('POST');
    expect(init.body).toBe(JSON.stringify({ username: 'zach', password: 'secret', rememberMe: true }));
    expect(init.headers['Content-Type']).toBe('application/json');
  });

  it('favorites PUT sends {favorite}', async () => {
    fetchMock.mockResolvedValueOnce(jsonResponse(200, { favorite: true }));
    await api().setFavorite('m_9', true);
    const [url, init] = fetchMock.mock.calls[0] as [string, RequestInit];
    expect(url).toBe('http://nas.example.com/api/items/m_9/favorite');
    expect(init.method).toBe('PUT');
    expect(init.body).toBe(JSON.stringify({ favorite: true }));
  });

  it('encodes folder ids in URLs', async () => {
    fetchMock.mockResolvedValueOnce(jsonResponse(200, { folder: null, children: [], breadcrumbs: [] }));
    await api().folder('root');
    expect((fetchMock.mock.calls[0] as [string, RequestInit])[0]).toBe('http://nas.example.com/api/folders/root');
  });
});
