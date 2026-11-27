import { useCallback } from 'react';
import { useSearchParams } from 'react-router-dom';
import { useAuth } from '../auth/AuthContext';
import type { ItemListQuery } from '../api/query';
import { SearchIcon } from '../components/Icons';
import { ItemBrowser } from './ItemBrowser';

export function SearchView() {
  const { api } = useAuth();
  const [searchParams] = useSearchParams();
  const q = (searchParams.get('q') ?? '').trim();

  const fetchItems = useCallback(
    (query: ItemListQuery) => api.search(q, query),
    [api, q],
  );

  return (
    <ItemBrowser
      fetchItems={fetchItems}
      emptyTitle={q ? `No results for “${q}”` : 'Search your library'}
      emptyHint={
        q
          ? 'Try a different search term — filenames are matched, including extensions.'
          : 'Type in the search box above and press Enter to search filenames.'
      }
      emptyIcon={<SearchIcon size={30} />}
      banner={
        <div className="view-header">
          <div className="view-title-row">
            <h1 className="view-title">{q ? `Results for “${q}”` : 'Search'}</h1>
          </div>
        </div>
      }
    />
  );
}
