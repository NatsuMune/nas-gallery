// localStorage-backed auth/server configuration. Keys are namespaced under
// "nasgallery." so multiple apps on the same origin never collide.

const TOKEN_KEY = 'nasgallery.token';
const SERVER_KEY = 'nasgallery.serverUrl';
const REMEMBER_KEY = 'nasgallery.rememberMe';

function safeGet(key: string): string | null {
  try {
    return globalThis.localStorage?.getItem(key) ?? null;
  } catch {
    return null;
  }
}

function safeSet(key: string, value: string): void {
  try {
    globalThis.localStorage?.setItem(key, value);
  } catch {
    // Storage unavailable (private mode etc.) — best effort only.
  }
}

function safeRemove(key: string): void {
  try {
    globalThis.localStorage?.removeItem(key);
  } catch {
    // ignore
  }
}

// --- server URL ---

export function normalizeServerUrl(url: string): string {
  return url.trim().replace(/\/+$/, '');
}

export function getStoredServerUrl(): string {
  return normalizeServerUrl(safeGet(SERVER_KEY) ?? '');
}

export function setStoredServerUrl(url: string): void {
  safeSet(SERVER_KEY, normalizeServerUrl(url));
}

// --- token ---

export function getStoredToken(): string | null {
  return safeGet(TOKEN_KEY);
}

export function setStoredToken(token: string): void {
  safeSet(TOKEN_KEY, token);
}

export function clearStoredToken(): void {
  safeRemove(TOKEN_KEY);
}

// --- remember me (server-side TTL hint; kept for UX continuity) ---

export function getRememberMe(): boolean {
  return safeGet(REMEMBER_KEY) === 'true';
}

export function setRememberMe(value: boolean): void {
  safeSet(REMEMBER_KEY, String(value));
}

export function clearAuthStorage(): void {
  safeRemove(TOKEN_KEY);
  safeRemove(REMEMBER_KEY);
}
