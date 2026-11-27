import { useCallback, useEffect, useRef, useState, type ReactNode } from 'react';
import { useLocation, useNavigate } from 'react-router-dom';
import { useAuth } from '../auth/AuthContext';
import { useLibraryStatusContext } from './Header';
import type { Folder } from '../api/types';
import type { ApiClient } from '../api/client';
import { ChevronDownIcon, ChevronLeftIcon, FolderIcon, HeartIcon, PhotoIcon, RefreshIcon, SpinnerIcon } from './Icons';

// Session caches so the tree survives navigation without refetching.
const childrenCache = new Map<string, Folder[]>();
const inflightChildren = new Map<string, Promise<Folder[]>>();

function fetchChildren(api: ApiClient, id: string): Promise<Folder[]> {
  const cached = childrenCache.get(id);
  if (cached) return Promise.resolve(cached);
  let p = inflightChildren.get(id);
  if (!p) {
    p = api
      .folder(id)
      .then((res) => {
        childrenCache.set(id, res.children);
        inflightChildren.delete(id);
        return res.children;
      })
      .catch((err) => {
        inflightChildren.delete(id);
        throw err;
      });
    inflightChildren.set(id, p);
  }
  return p;
}

/** Drop cached tree data (e.g. after a library rescan). */
export function invalidateTreeCache(): void {
  childrenCache.clear();
}

interface RowProps {
  folder: Folder;
  depth: number;
  active: boolean;
  expanded: boolean;
  onNavigate: (id: string) => void;
  onToggle: (id: string) => void;
}

function Row({ folder, depth, active, expanded, onNavigate, onToggle }: RowProps) {
  const expandable = folder.childCount > 0;
  return (
    <div className={`tree-row${active ? ' tree-row-active' : ''}`} style={{ paddingLeft: 8 + depth * 16 }}>
      {expandable ? (
        <button
          type="button"
          className="tree-expander"
          aria-label={expanded ? `Collapse ${folder.name}` : `Expand ${folder.name}`}
          aria-expanded={expanded}
          onClick={() => onToggle(folder.id)}
        >
          <ChevronDownIcon size={14} className={expanded ? 'tree-chevron-open' : 'tree-chevron'} />
        </button>
      ) : (
        <span className="tree-expander tree-expander-blank" aria-hidden="true" />
      )}
      <button
        type="button"
        className="tree-label"
        aria-current={active ? 'page' : undefined}
        onClick={() => onNavigate(folder.id)}
      >
        <FolderIcon size={15} />
        <span className="tree-name">{folder.name}</span>
      </button>
      {folder.fileCount > 0 && <span className="tree-count">{folder.fileCount}</span>}
    </div>
  );
}

export function FolderTree({ activeFolderId }: { activeFolderId: string | null }) {
  const { api } = useAuth();
  const navigate = useNavigate();
  const location = useLocation();
  const inFavorites = location.pathname === '/favorites';
  const { scanning, triggerScan } = useLibraryStatusContext();
  const [roots, setRoots] = useState<Folder[]>(() => childrenCache.get('root') ?? []);
  const [childrenByFolder, setChildrenByFolder] = useState<Record<string, Folder[]>>(() => {
    const initial: Record<string, Folder[]> = {};
    for (const [id, children] of childrenCache) initial[id] = children;
    return initial;
  });
  const [expanded, setExpanded] = useState<Set<string>>(new Set());
  const [error, setError] = useState<string | null>(null);
  const navRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    let cancelled = false;
    fetchChildren(api, 'root')
      .then((children) => {
        if (cancelled) return;
        setRoots(children);
        setError(null);
      })
      .catch((err) => {
        if (!cancelled) setError(err instanceof Error ? err.message : 'Failed to load folders');
      });
    return () => {
      cancelled = true;
    };
  }, [api]);

  const toggle = useCallback(
    (id: string) => {
      setExpanded((prev) => {
        const next = new Set(prev);
        if (next.has(id)) next.delete(id);
        else next.add(id);
        return next;
      });
      if (!childrenCache.has(id)) {
        fetchChildren(api, id)
          .then((children) => setChildrenByFolder((prev) => ({ ...prev, [id]: children })))
          .catch(() => {
            setExpanded((prev) => {
              const next = new Set(prev);
              next.delete(id);
              return next;
            });
          });
      }
    },
    [api],
  );

  const go = useCallback(
    (id: string) => {
      navigate(`/folders/${id}`);
    },
    [navigate],
  );

  // Keyboard: Up/Down move focus through visible rows.
  const onKeyDown = useCallback((e: React.KeyboardEvent) => {
    if (e.key !== 'ArrowDown' && e.key !== 'ArrowUp') return;
    const buttons = Array.from(navRef.current?.querySelectorAll<HTMLButtonElement>('.tree-label') ?? []);
    if (buttons.length === 0) return;
    const idx = buttons.indexOf(document.activeElement as HTMLButtonElement);
    if (idx === -1) return;
    e.preventDefault();
    const next = e.key === 'ArrowDown' ? Math.min(idx + 1, buttons.length - 1) : Math.max(idx - 1, 0);
    buttons[next].focus();
  }, []);

  const renderRows = (folders: Folder[], depth: number): ReactNode =>
    folders.map((folder) => {
      const isExpanded = expanded.has(folder.id);
      const children = childrenByFolder[folder.id] ?? childrenCache.get(folder.id) ?? null;
      return (
        <div key={folder.id} className="tree-node">
          <Row
            folder={folder}
            depth={depth}
            active={folder.id === activeFolderId}
            expanded={isExpanded}
            onNavigate={go}
            onToggle={toggle}
          />
          {isExpanded && children && renderRows(children, depth + 1)}
        </div>
      );
    });

  return (
    <div className="sidebar">
      <nav className="tree" ref={navRef} aria-label="Folders" onKeyDown={onKeyDown}>
        <div className={`tree-row${activeFolderId === 'root' ? ' tree-row-active' : ''}`}>
          <span className="tree-expander tree-expander-blank" aria-hidden="true" />
          <button type="button" className="tree-label" onClick={() => go('root')}>
            <PhotoIcon size={15} />
            <span className="tree-name">All media</span>
          </button>
        </div>
        <div className={`tree-row${inFavorites ? ' tree-row-active' : ''}`}>
          <span className="tree-expander tree-expander-blank" aria-hidden="true" />
          <button
            type="button"
            className="tree-label"
            aria-current={inFavorites ? 'page' : undefined}
            onClick={() => navigate('/favorites')}
          >
            <HeartIcon size={15} filled={inFavorites} />
            <span className="tree-name">Favorites</span>
          </button>
        </div>
        {renderRows(roots, 1)}
        {error && <p className="tree-error">{error}</p>}
      </nav>
      <div className="sidebar-foot">
        <button type="button" className="sidebar-scan" onClick={triggerScan} disabled={scanning}>
          {scanning ? <SpinnerIcon size={15} /> : <RefreshIcon size={15} />}
          {scanning ? 'Scanning…' : 'Rescan library'}
        </button>
      </div>
    </div>
  );
}

export function BackButton({ folder }: { folder: Folder | null }) {
  const navigate = useNavigate();
  if (!folder || folder.parentId === null) return null;
  return (
    <button type="button" className="crumb-back" onClick={() => navigate(`/folders/${folder.parentId}`)}>
      <ChevronLeftIcon size={16} />
      Back
    </button>
  );
}
