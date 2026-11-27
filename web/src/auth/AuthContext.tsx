import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useRef,
  useState,
  type ReactNode,
} from 'react';
import { useNavigate } from 'react-router-dom';
import {
  createApiClient,
  type ApiClient,
  type ApiError,
} from '../api/client';
import {
  clearAuthStorage,
  getStoredServerUrl,
  getStoredToken,
  setRememberMe,
  setStoredServerUrl,
  setStoredToken,
} from '../api/storage';
import type { MeResponse } from '../api/types';

export type AuthStatus = 'booting' | 'signed-in' | 'signed-out';

export interface AuthContextValue {
  status: AuthStatus;
  user: MeResponse['user'] | null;
  serverName: string | null;
  serverVersion: string | null;
  serverUrl: string;
  api: ApiClient;
  login: (serverUrl: string, username: string, password: string, rememberMe: boolean) => Promise<void>;
  logout: () => Promise<void>;
}

const AuthContext = createContext<AuthContextValue | null>(null);

export function AuthProvider({ children }: { children: ReactNode }) {
  const navigate = useNavigate();
  const [status, setStatus] = useState<AuthStatus>('booting');
  const [user, setUser] = useState<MeResponse['user'] | null>(null);
  const [serverName, setServerName] = useState<string | null>(null);
  const [serverVersion, setServerVersion] = useState<string | null>(null);
  const bootedRef = useRef(false);

  const onUnauthorized = useCallback(() => {
    clearAuthStorage();
    setUser(null);
    setServerName(null);
    setServerVersion(null);
    setStatus('signed-out');
    navigate('/login', { replace: true });
  }, [navigate]);

  const api = useMemo(
    () =>
      createApiClient({
        getServerUrl: () => getStoredServerUrl(),
        getToken: () => getStoredToken(),
        onUnauthorized,
      }),
    [onUnauthorized],
  );

  // Boot: validate any stored token against /api/me.
  useEffect(() => {
    if (bootedRef.current) return;
    bootedRef.current = true;
    const token = getStoredToken();
    if (!token) {
      setStatus('signed-out');
      return;
    }
    let cancelled = false;
    api
      .me()
      .then((me) => {
        if (cancelled) return;
        setUser(me.user);
        setServerName(me.server.name);
        setServerVersion(me.server.version);
        setStatus('signed-in');
      })
      .catch((err: ApiError) => {
        if (cancelled) return;
        // 401 -> token invalid/expired: drop it. Network errors keep the
        // token (server may be temporarily unreachable); user re-logins.
        if (err.status === 401) clearAuthStorage();
        setStatus('signed-out');
      });
    return () => {
      cancelled = true;
    };
  }, [api]);

  const login = useCallback(
    async (serverUrl: string, username: string, password: string, rememberMe: boolean) => {
      // Persist the server URL first so the client can reach the login
      // endpoint on this server.
      setStoredServerUrl(serverUrl);
      const res = await api.login(username, password, rememberMe);
      setStoredToken(res.token);
      setRememberMe(rememberMe);
      try {
        const me = await api.me();
        setUser(me.user);
        setServerName(me.server.name);
        setServerVersion(me.server.version);
      } catch {
        // Token is valid; /api/me hiccup (e.g. flaky network). Fall back to
        // the provided username so the shell can still render.
        setUser({ id: 0, username });
      }
      setStatus('signed-in');
    },
    [api],
  );

  const logout = useCallback(async () => {
    try {
      await api.logout();
    } catch {
      // Best effort: revoke server-side; clear locally regardless.
    }
    clearAuthStorage();
    setUser(null);
    setServerName(null);
    setServerVersion(null);
    setStatus('signed-out');
    navigate('/login', { replace: true });
  }, [api, navigate]);

  const value = useMemo<AuthContextValue>(
    () => ({
      status,
      user,
      serverName,
      serverVersion,
      serverUrl: getStoredServerUrl(),
      api,
      login,
      logout,
    }),
    [status, user, serverName, serverVersion, api, login, logout],
  );

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
}

export function useAuth(): AuthContextValue {
  const ctx = useContext(AuthContext);
  if (!ctx) throw new Error('useAuth must be used inside AuthProvider');
  return ctx;
}
