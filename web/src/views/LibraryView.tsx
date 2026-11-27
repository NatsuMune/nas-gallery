import { useCallback, useEffect, useState } from 'react';
import { useParams } from 'react-router-dom';
import { useAuth } from '../auth/AuthContext';
import type { Folder, FolderResponse } from '../api/types';
import type { ItemListQuery } from '../api/query';
import { Breadcrumbs } from '../components/Breadcrumbs';
import { BackButton } from '../components/FolderTree';
import { FolderStrip } from '../components/FolderStrip';
import { PhotoIcon } from '../components/Icons';
import { ErrorState } from '../components/States';
import { ItemBrowser } from './ItemBrowser';

export function LibraryView() {
  const { folderId = 'root' } = useParams();
  const { api } = useAuth();
  const [folderData, setFolderData] = useState<FolderResponse | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;
    setLoading(true);
    setError(null);
    setFolderData(null);
    api
      .folder(folderId)
      .then((res) => {
        if (cancelled) return;
        setFolderData(res);
        setLoading(false);
      })
      .catch((err: unknown) => {
        if (cancelled) return;
        setError(err instanceof Error ? err.message : 'Failed to load folder');
        setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [api, folderId]);

  const fetchItems = useCallback(
    (query: ItemListQuery) => api.folderItems(folderId, query),
    [api, folderId],
  );

  const crumbs = folderData?.breadcrumbs ?? [];
  const folder: Folder | null = folderData?.folder ?? null;
  const title = folder ? folder.name : 'All media';

  const banner = (
    <div className="view-header">
      <div className="view-title-row">
        <BackButton folder={folder} />
        <h1 className="view-title" title={title}>
          {title}
        </h1>
      </div>
      <Breadcrumbs crumbs={crumbs} />
      {folderData && folderData.children.length > 0 && <FolderStrip folders={folderData.children} />}
    </div>
  );

  if (error) {
    return (
      <section className="browser">
        {banner}
        <ErrorState title="Couldn’t load this folder" hint={error} onRetry={() => window.location.reload()} />
      </section>
    );
  }

  return (
    <ItemBrowser
      fetchItems={fetchItems}
      emptyTitle={loading ? 'Loading folder…' : 'This folder is empty'}
      emptyHint={
        loading
          ? undefined
          : 'Media files appear here once your library has been scanned. Try “Rescan library” in the sidebar.'
      }
      emptyIcon={<PhotoIcon size={30} />}
      banner={banner}
    />
  );
}
