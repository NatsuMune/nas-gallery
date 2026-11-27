import { useEffect, useState, type FormEvent } from 'react';
import { Navigate } from 'react-router-dom';
import { useAuth } from '../auth/AuthContext';
import { ApiError } from '../api/client';
import { getRememberMe, getStoredServerUrl, setStoredServerUrl } from '../api/storage';
import { PhotoIcon, SpinnerIcon } from '../components/Icons';

export function LoginView() {
  const { status, login, api } = useAuth();
  const [serverUrl, setServerUrl] = useState(() => getStoredServerUrl());
  const [username, setUsername] = useState('');
  const [password, setPassword] = useState('');
  const [rememberMe, setRememberMe] = useState(() => getRememberMe());
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<{ message: string; retryAfter?: number } | null>(null);

  useEffect(() => {
    if (status === 'signed-in') return; // <Navigate> handles redirect below
  }, [status]);

  async function onSubmit(e: FormEvent) {
    e.preventDefault();
    if (busy) return;
    setError(null);

    if (!serverUrl.trim()) {
      setError({ message: 'Enter the address of your NAS Gallery server.' });
      return;
    }
    if (!username.trim() || !password) {
      setError({ message: 'Enter your username and password.' });
      return;
    }

    setBusy(true);
    try {
      // Persist the URL first (the API client reads it from storage), then
      // validate reachability via GET /api/health before attempting login.
      const url = serverUrl.trim();
      setStoredServerUrl(url);
      try {
        await api.health();
      } catch {
        setError({
          message: `Could not reach the server at ${url}. Check the address and that the server is running.`,
        });
        setBusy(false);
        return;
      }
      await login(url, username.trim(), password, rememberMe);
    } catch (err) {
      if (err instanceof ApiError) {
        if (err.status === 429) {
          const seconds = err.retryAfterSeconds;
          setError({
            message:
              seconds && seconds > 0
                ? `Too many failed attempts. Please wait ${seconds}s before trying again.`
                : 'Too many failed attempts. Please wait a moment before trying again.',
            retryAfter: seconds,
          });
        } else if (err.status === 0) {
          setError({
            message: `Could not reach the server at ${serverUrl.trim()}. Check the address and that the server is running.`,
          });
        } else {
          setError({ message: err.message });
        }
      } else {
        setError({ message: 'Something went wrong. Please try again.' });
      }
      setBusy(false);
    }
  }

  if (status === 'signed-in') return <Navigate to="/" replace />;

  return (
    <div className="login-page">
      <div className="login-card">
        <div className="login-brand">
          <span className="login-logo" aria-hidden="true">
            <PhotoIcon size={26} />
          </span>
          <h1>NAS Gallery</h1>
          <p className="login-sub">Your photos and videos, straight from your NAS.</p>
        </div>

        <form onSubmit={onSubmit} noValidate>
          <div className="field">
            <label className="field-label" htmlFor="login-server">
              Server URL
            </label>
            <input
              id="login-server"
              className="input"
              type="url"
              inputMode="url"
              autoComplete="url"
              spellCheck={false}
              placeholder="https://nas.example.com"
              value={serverUrl}
              onChange={(e) => setServerUrl(e.target.value)}
              aria-invalid={error !== null && error.message.includes('reach')}
            />
            <span className="field-hint">Validated against the server’s /api/health endpoint.</span>
          </div>

          <div className="field">
            <label className="field-label" htmlFor="login-username">
              Username
            </label>
            <input
              id="login-username"
              className="input"
              type="text"
              autoComplete="username"
              spellCheck={false}
              value={username}
              onChange={(e) => setUsername(e.target.value)}
            />
          </div>

          <div className="field">
            <label className="field-label" htmlFor="login-password">
              Password
            </label>
            <input
              id="login-password"
              className="input"
              type="password"
              autoComplete="current-password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
            />
          </div>

          <label className="checkbox login-remember">
            <input
              type="checkbox"
              checked={rememberMe}
              onChange={(e) => setRememberMe(e.target.checked)}
            />
            <span>Remember me for 180 days</span>
          </label>

          {error && (
            <p className="login-error" role="alert">
              {error.message}
            </p>
          )}

          <button type="submit" className="btn btn-primary login-submit" disabled={busy}>
            {busy ? <SpinnerIcon size={18} /> : null}
            {busy ? 'Signing in…' : 'Sign in'}
          </button>
        </form>
      </div>
    </div>
  );
}
