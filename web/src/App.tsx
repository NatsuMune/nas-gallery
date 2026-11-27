import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { Navigate, Route, Routes, useLocation, Outlet } from 'react-router-dom';
import { AuthProvider, useAuth } from './auth/AuthContext';
import { Header, LibraryStatusContext, type LibraryStatusContextValue } from './components/Header';
import { FolderTree, invalidateTreeCache } from './components/FolderTree';
import { UpdateBanner } from './components/UpdateBanner';
import { PhotoIcon, SpinnerIcon } from './components/Icons';
import { LoginView } from './views/LoginView';
import { LibraryView } from './views/LibraryView';
import { FavoritesView } from './views/FavoritesView';
import { SearchView } from './views/SearchView';
import type { LibraryStatus } from './api/types';

export default function App() {
  return (
    <AuthProvider>
      <AppRoutes />
    </AuthProvider>
  );
}

function AppRoutes() {
  const { status } = useAuth();
  if (status === 'booting') return <BootScreen />;
  return (
    <Routes>
      <Route path="/login" element={<LoginView />} />
      <Route element={<RequireAuth />}>
        <Route path="/" element={<Navigate to="/folders/root" replace />} />
        <Route path="/folders/:folderId" element={<LibraryView />} />
        <Route path="/favorites" element={<FavoritesView />} />
        <Route path="/search" element={<SearchView />} />
      </Route>
      <Route path="*" element={<Navigate to="/" replace />} />
    </Routes>
  );
}

function BootScreen() {
  return (
    <div className="boot-screen" role="status" aria-label="Loading">
      <span className="boot-logo">
        <PhotoIcon size={30} />
      </span>
      <SpinnerIcon size={26} />
    </div>
  );
}

function RequireAuth() {
  const { status } = useAuth();
  if (status !== 'signed-in') return <Navigate to="/login" replace />;
  return <AppShell />;
}

function AppShell() {
  const { api } = useAuth();
  const location = useLocation();
  const [status, setStatus] = useState<LibraryStatus | null>(null);
  const [scanning, setScanning] = useState(false);
  const wasScanningRef = useRef(false);

  const activeFolderId = useMemo(() => /^\/folders\/(.+)$/.exec(location.pathname)?.[1] ?? null, [location.pathname]);

  const refreshStatus = useCallback(() => {
    api
      .libraryStatus()
      .then((s) => {
        setStatus(s);
        setScanning(s.scanning);
      })
      .catch(() => undefined);
  }, [api]);

  useEffect(() => {
    refreshStatus();
    const id = window.setInterval(refreshStatus, 5000);
    return () => window.clearInterval(id);
  }, [refreshStatus]);

  const triggerScan = useCallback(() => {
    api
      .triggerScan()
      .then(() => {
        setScanning(true);
        refreshStatus();
      })
      .catch(() => undefined);
  }, [api, refreshStatus]);

  // When a scan finishes, drop cached tree data so new folders show up.
  useEffect(() => {
    if (wasScanningRef.current && !scanning) invalidateTreeCache();
    wasScanningRef.current = scanning;
  }, [scanning]);

  const statusValue = useMemo<LibraryStatusContextValue>(
    () => ({ scanning, status, triggerScan }),
    [scanning, status, triggerScan],
  );

  return (
    <LibraryStatusContext.Provider value={statusValue}>
      <div className="app-shell">
        <UpdateBanner />
        <Header />
        <div className="app-body">
          <aside className="app-sidebar">
            <FolderTree activeFolderId={activeFolderId} />
          </aside>
          <main className="app-main" id="main">
            <Outlet />
          </main>
        </div>
      </div>
    </LibraryStatusContext.Provider>
  );
}
