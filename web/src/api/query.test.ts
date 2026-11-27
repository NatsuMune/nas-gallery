import { describe, expect, it } from 'vitest';
import { buildItemListQuery, withParams } from './query';

describe('buildItemListQuery', () => {
  it('returns an empty string when nothing is set', () => {
    expect(buildItemListQuery()).toBe('');
    expect(buildItemListQuery({})).toBe('');
  });

  it('includes sort and order', () => {
    expect(buildItemListQuery({ sort: 'name', order: 'asc' })).toBe('?sort=name&order=asc');
  });

  it('includes type filter', () => {
    expect(buildItemListQuery({ type: 'video' })).toBe('?type=video');
    expect(buildItemListQuery({ sort: 'date', order: 'desc', type: 'photo' })).toBe(
      '?sort=date&order=desc&type=photo',
    );
  });

  it('includes pagination', () => {
    expect(buildItemListQuery({ page: 2, pageSize: 100 })).toBe('?page=2&pageSize=100');
  });

  it('builds the full set in a stable order', () => {
    expect(buildItemListQuery({ sort: 'size', order: 'desc', type: 'all', page: 1, pageSize: 500 })).toBe(
      '?sort=size&order=desc&type=all&page=1&pageSize=500',
    );
  });

  it('omits undefined values but keeps explicit zero-page-free values', () => {
    expect(buildItemListQuery({ sort: 'type', order: undefined })).toBe('?sort=type');
  });
});

describe('withParams', () => {
  it('appends extra params to an existing query string', () => {
    expect(withParams('?sort=name', { q: 'sunset' })).toBe('?sort=name&q=sunset');
  });

  it('creates a query string from nothing', () => {
    expect(withParams('', { q: 'x' })).toBe('?q=x');
  });

  it('overrides duplicates (URLSearchParams keeps the original position)', () => {
    expect(withParams('?q=old&sort=name', { q: 'new' })).toBe('?q=new&sort=name');
    expect(new URLSearchParams(withParams('?q=old&sort=name', { q: 'new' })).get('q')).toBe('new');
  });
});
