import { useCallback } from 'react';
import { useAuth } from '../auth/AuthContext';
import type { ItemListQuery } from '../api/query';
import { HeartIcon } from '../components/Icons';
import { ItemBrowser } from './ItemBrowser';

export function FavoritesView() {
  const { api } = useAuth();

  const fetchItems = useCallback(
    (query: ItemListQuery) => api.favorites(query),
    [api],
  );

  return (
    <ItemBrowser
      fetchItems={fetchItems}
      removeOnUnfavorite
      emptyTitle="No favorites yet"
      emptyHint="Tap the heart on any photo or video to keep it here."
      emptyIcon={<HeartIcon size={30} />}
      banner={
        <div className="view-header">
          <div className="view-title-row">
            <h1 className="view-title">Favorites</h1>
          </div>
        </div>
      }
    />
  );
}
