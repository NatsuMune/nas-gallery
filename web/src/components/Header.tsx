import { createContext, useContext, useEffect, useRef, useState, type ReactNode } from 'react';
import { useLocation, useNavigate } from 'react-router-dom';
import { useAuth } from '../auth/AuthContext';
import { HeartIcon, LogoutIcon, PhotoIcon, SearchIcon, SpinnerIcon, UserIcon } from './Icons';
import { InstallButton } from './InstallButton';
import type { LibraryStatus } from '../api/types';

export interface LibraryStatusContextValue {
  scanning: boolean;
  status: LibraryStatus | null;
  triggerScan: () => void;
}

export const LibraryStatusContext = createContext<LibraryStatusContextValue | null>(null);

export function useLibraryStatusContext(): LibraryStatusContextValue {
  const ctx = useContext(LibraryStatusContext);
  if (!ctx) throw new Error('LibraryStatusContext missing');
  return ctx;
}

export function Header() {
  const { user, serverName, logout } = useAuth();
  const { scanning } = useLibraryStatusContext();
  const navigate = useNavigate();
  const location = useLocation();
  const [query, setQuery] = useState('');
  const inputRef = useRef<HTMLInputElement>(null);
  const inFavorites = location.pathname === '/favorites';

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === '/' && document.activeElement?.tagName !== 'INPUT') {
        e.preventDefault();
        inputRef.current?.focus();
      }
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, []);

  function submitSearch(e: React.FormEvent) {
    e.preventDefault();
    const q = query.trim();
    if (!q) return;
    navigate(`/search?q=${encodeURIComponent(q)}`);
  }

  return (
    <header className="header">
      <button className="header-brand" onClick={() => navigate('/')} aria-label="NAS Gallery home">
        <span className="header-logo" aria-hidden="true">
          <PhotoIcon size={22} />
        </span>
        <span className="header-title">NAS Gallery</span>
        {serverName && <span className="header-server">{serverName}</span>}
      </button>

      {scanning && (
        <span className="scan-pill" role="status">
          <SpinnerIcon size={14} />
          Scanning library…
        </span>
      )}

      <form className="header-search" role="search" onSubmit={submitSearch}>
        <SearchIcon size={16} />
        <input
          ref={inputRef}
          type="search"
          className="header-search-input"
          placeholder="Search files…"
          aria-label="Search files"
          value={query}
          onChange={(e) => setQuery(e.target.value)}
        />
      </form>

      <div className="header-actions">
        <InstallButton />
        <button
          type="button"
          className="btn-icon"
          aria-label="Favorites"
          aria-current={inFavorites ? 'page' : undefined}
          title="Favorites"
          onClick={() => navigate('/favorites')}
        >
          <HeartIcon filled={inFavorites} />
        </button>
        <span className="header-user" title={user ? `Signed in as ${user.username}` : undefined}>
          <UserIcon size={16} />
          <span>{user?.username}</span>
        </span>
        <button className="btn-icon" onClick={() => void logout()} aria-label="Sign out">
          <LogoutIcon />
        </button>
      </div>
    </header>
  );
}

export function HeaderSlot({ children }: { children: ReactNode }) {
  return <div className="header-slot">{children}</div>;
}
