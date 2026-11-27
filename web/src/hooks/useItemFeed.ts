import { useCallback, useEffect, useRef, useState } from 'react';
import type { Item, ItemPage } from '../api/types';

export interface ItemFeed {
  items: Item[];
  total: number;
  /** Initial page load (show skeletons). */
  loading: boolean;
  /** Loading an additional page (show spinner row). */
  loadingMore: boolean;
  error: string | null;
  hasMore: boolean;
  loadMore: () => void;
  reload: () => void;
  patchItem: (id: string, patch: Partial<Item>) => void;
  removeItem: (id: string) => void;
}

/**
 * Infinite-scroll feed over a paginated item endpoint. `fetchPage` must be a
 * stable callback (useCallback keyed by sort/filter/identity params) — a new
 * identity resets the feed to page 1.
 */
export function useItemFeed(fetchPage: (page: number) => Promise<ItemPage>): ItemFeed {
  const [items, setItems] = useState<Item[]>([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(0);
  const [loading, setLoading] = useState(true);
  const [loadingMore, setLoadingMore] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [reloadNonce, setReloadNonce] = useState(0);
  const seq = useRef(0);
  const pageRef = useRef(0);
  const itemsRef = useRef<Item[]>([]);
  pageRef.current = page;
  itemsRef.current = items;

  const run = useCallback(
    async (targetPage: number, mode: 'replace' | 'append') => {
      const mySeq = ++seq.current;
      if (mode === 'replace') setLoading(true);
      else setLoadingMore(true);
      setError(null);
      try {
        const res = await fetchPage(targetPage);
        if (mySeq !== seq.current) return; // superseded by a newer load
        setItems((prev) => (mode === 'append' ? [...prev, ...res.items] : res.items));
        setTotal(res.total);
        setPage(res.page);
      } catch (err) {
        if (mySeq !== seq.current) return;
        setError(err instanceof Error ? err.message : 'Something went wrong');
      } finally {
        if (mySeq === seq.current) {
          setLoading(false);
          setLoadingMore(false);
        }
      }
    },
    [fetchPage],
  );

  // Reset + load page 1 whenever the fetcher (or reload) changes.
  useEffect(() => {
    const mySeq = ++seq.current;
    setItems([]);
    setTotal(0);
    setPage(0);
    setLoading(true);
    setError(null);
    fetchPage(1)
      .then((res) => {
        if (mySeq !== seq.current) return;
        setItems(res.items);
        setTotal(res.total);
        setPage(res.page);
      })
      .catch((err: unknown) => {
        if (mySeq !== seq.current) return;
        setError(err instanceof Error ? err.message : 'Something went wrong');
      })
      .finally(() => {
        if (mySeq === seq.current) setLoading(false);
      });
  }, [fetchPage, reloadNonce]);

  const loadMore = useCallback(() => {
    if (loading || loadingMore) return;
    if (page > 0 && itemsRef.current.length >= total) return;
    void run(pageRef.current + 1, 'append');
  }, [loading, loadingMore, page, total, run]);

  const reload = useCallback(() => {
    setReloadNonce((n) => n + 1);
  }, []);

  const patchItem = useCallback((id: string, patch: Partial<Item>) => {
    setItems((prev) => prev.map((it) => (it.id === id ? { ...it, ...patch } : it)));
  }, []);

  const removeItem = useCallback((id: string) => {
    setItems((prev) => {
      const next = prev.filter((it) => it.id !== id);
      setTotal((t) => Math.max(0, t - (next.length === prev.length ? 0 : 1)));
      return next;
    });
  }, []);

  return {
    items,
    total,
    loading,
    loadingMore,
    error,
    hasMore: page > 0 && items.length < total,
    loadMore,
    reload,
    patchItem,
    removeItem,
  };
}
