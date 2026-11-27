// Query-string building for item list endpoints (sort/order/type/page/pageSize).
import type { SortField, SortOrder, TypeFilter } from './types';

export interface ItemListQuery {
  sort?: SortField;
  order?: SortOrder;
  type?: TypeFilter;
  page?: number;
  pageSize?: number;
}

/**
 * Builds the query string (with leading "?" or empty string) for item list
 * endpoints. Only defined values are included; the server applies defaults
 * (date/desc/all/page 1/pageSize 100) for anything omitted.
 */
export function buildItemListQuery(query: ItemListQuery = {}): string {
  const params = new URLSearchParams();
  if (query.sort !== undefined) params.set('sort', query.sort);
  if (query.order !== undefined) params.set('order', query.order);
  if (query.type !== undefined) params.set('type', query.type);
  if (query.page !== undefined) params.set('page', String(query.page));
  if (query.pageSize !== undefined) params.set('pageSize', String(query.pageSize));
  const s = params.toString();
  return s ? `?${s}` : '';
}

/** Appends extra params (e.g. search q) to a list query string. */
export function withParams(queryString: string, extra: Record<string, string>): string {
  const params = new URLSearchParams(queryString.startsWith('?') ? queryString.slice(1) : queryString);
  for (const [key, value] of Object.entries(extra)) params.set(key, value);
  const s = params.toString();
  return s ? `?${s}` : '';
}
