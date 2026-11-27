import { useEffect, useState } from 'react';
import type { Item } from '../api/types';
import type { ApiClient } from '../api/client';

// Session-scoped item cache: folder cover tiles and video posters reuse item
// lookups (GET /api/items/{id}) without refetching.
const cache = new Map<string, Item>();
const inflight = new Map<string, Promise<Item>>();

export function getCachedItem(id: string): Item | undefined {
  return cache.get(id);
}

export function fetchItemIntoCache(api: ApiClient, id: string): Promise<Item> {
  const existing = cache.get(id);
  if (existing) return Promise.resolve(existing);
  let p = inflight.get(id);
  if (!p) {
    p = api
      .item(id)
      .then((item) => {
        cache.set(id, item);
        inflight.delete(id);
        return item;
      })
      .catch((err) => {
        inflight.delete(id);
        throw err;
      });
    inflight.set(id, p);
  }
  return p;
}

/** Returns the cached item for coverItemId, fetching once per session. */
export function useCoverItem(api: ApiClient, coverItemId: string | null): Item | null {
  const [item, setItem] = useState<Item | null>(() => (coverItemId ? cache.get(coverItemId) ?? null : null));

  useEffect(() => {
    if (!coverItemId) {
      setItem(null);
      return;
    }
    const cached = cache.get(coverItemId);
    if (cached) {
      setItem(cached);
      return;
    }
    let cancelled = false;
    fetchItemIntoCache(api, coverItemId)
      .then((it) => {
        if (!cancelled) setItem(it);
      })
      .catch(() => {
        // Cover stays blank; the tile shows a fallback icon.
      });
    return () => {
      cancelled = true;
    };
  }, [api, coverItemId]);

  return item;
}
