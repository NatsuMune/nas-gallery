import { useCallback, useEffect, useState, type ReactNode } from 'react';
import { useAuth } from '../auth/AuthContext';
import { useItemFeed } from '../hooks/useItemFeed';
import { joinUrl } from '../api/client';
import type { Item, ItemPage, SortField, SortOrder, TypeFilter } from '../api/types';
import type { ItemListQuery } from '../api/query';
import { Toolbar } from '../components/Toolbar';
import { GridSkeleton, MediaGrid } from '../components/MediaGrid';
import { EmptyState, ErrorState } from '../components/States';
import { Lightbox } from '../components/Lightbox';
import { SpinnerIcon } from '../components/Icons';

export interface ItemBrowserProps {
  /** Base fetcher; the browser layers sort/order/type/page on top. */
  fetchItems: (query: ItemListQuery) => Promise<ItemPage>;
  emptyTitle: string;
  emptyHint?: ReactNode;
  emptyIcon?: ReactNode;
  /** Favorites view: items disappear when un-favorited. */
  removeOnUnfavorite?: boolean;
  /** Rendered above the toolbar (breadcrumbs, titles, folder strips). */
  banner?: ReactNode;
}

export function ItemBrowser({
  fetchItems,
  emptyTitle,
  emptyHint,
  emptyIcon,
  removeOnUnfavorite = false,
  banner,
}: ItemBrowserProps) {
  const { api, serverUrl } = useAuth();
  const [sort, setSort] = useState<SortField>('date');
  const [order, setOrder] = useState<SortOrder>('desc');
  const [type, setType] = useState<TypeFilter>('all');
  const [lightboxIndex, setLightboxIndex] = useState<number | null>(null);

  const fetchPage = useCallback(
    (page: number) => fetchItems({ sort, order, type, page, pageSize: 100 }),
    [fetchItems, sort, order, type],
  );
  const feed = useItemFeed(fetchPage);

  const thumbSrc = useCallback((item: Item) => joinUrl(serverUrl, item.thumbUrl), [serverUrl]);

  const toggleFavorite = useCallback(
    async (item: Item) => {
      const next = !item.favorite;
      feed.patchItem(item.id, { favorite: next });
      try {
        await api.setFavorite(item.id, next);
        if (removeOnUnfavorite && !next) feed.removeItem(item.id);
      } catch {
        // Revert the optimistic update.
        feed.patchItem(item.id, { favorite: !next });
      }
    },
    [api, feed, removeOnUnfavorite],
  );

  const refreshItem = useCallback(
    async (id: string): Promise<Item | null> => {
      try {
        const fresh = await api.item(id);
        feed.patchItem(id, fresh);
        return fresh;
      } catch {
        return null;
      }
    },
    [api, feed],
  );

  // Infinite scroll sentinel.
  const [sentinelEl, setSentinelEl] = useState<HTMLDivElement | null>(null);
  const { loadMore, hasMore, loading, loadingMore, error } = feed;
  useEffect(() => {
    if (!sentinelEl || !hasMore || loading || loadingMore || error) return;
    const io = new IntersectionObserver(
      (entries) => {
        if (entries.some((entry) => entry.isIntersecting)) loadMore();
      },
      { rootMargin: '600px 0px' },
    );
    io.observe(sentinelEl);
    return () => io.disconnect();
  }, [sentinelEl, hasMore, loading, loadingMore, error, loadMore]);

  // Keep the lightbox index valid when items are removed.
  useEffect(() => {
    if (lightboxIndex === null) return;
    if (feed.items.length === 0) {
      setLightboxIndex(null);
    } else if (lightboxIndex >= feed.items.length) {
      setLightboxIndex(feed.items.length - 1);
    }
  }, [feed.items.length, lightboxIndex]);

  const sentinel =
    feed.hasMore && !feed.loading && !feed.error ? (
      <div ref={setSentinelEl} className="grid-sentinel" aria-hidden="true" />
    ) : null;

  return (
    <section className="browser">
      {banner}

      <Toolbar
        total={feed.total}
        sort={sort}
        order={order}
        type={type}
        onSortChange={setSort}
        onOrderChange={setOrder}
        onTypeChange={setType}
        onRefresh={feed.reload}
        refreshing={feed.loading}
      />

      {feed.error ? (
        <ErrorState
          title="Couldn’t load items"
          hint={feed.error}
          onRetry={feed.reload}
        />
      ) : feed.loading ? (
        <GridSkeleton />
      ) : feed.items.length === 0 ? (
        <EmptyState icon={emptyIcon} title={emptyTitle} hint={emptyHint} />
      ) : (
        <>
          <MediaGrid
            items={feed.items}
            thumbSrc={thumbSrc}
            onOpen={setLightboxIndex}
            onToggleFavorite={(item) => void toggleFavorite(item)}
            sentinel={sentinel}
          />
          {feed.loadingMore && (
            <div className="grid-loading-more" role="status" aria-label="Loading more items">
              <SpinnerIcon size={22} />
            </div>
          )}
        </>
      )}

      {lightboxIndex !== null && feed.items.length > 0 && (
        <Lightbox
          items={feed.items}
          index={Math.min(lightboxIndex, feed.items.length - 1)}
          serverUrl={serverUrl}
          onIndexChange={setLightboxIndex}
          onClose={() => setLightboxIndex(null)}
          onToggleFavorite={(item) => void toggleFavorite(item)}
          onRefreshItem={refreshItem}
        />
      )}
    </section>
  );
}
