import type { SortField, SortOrder, TypeFilter } from '../api/types';
import { ArrowDownIcon, ArrowUpIcon, RefreshIcon, SpinnerIcon } from './Icons';

interface ToolbarProps {
  total: number;
  sort: SortField;
  order: SortOrder;
  type: TypeFilter;
  onSortChange: (sort: SortField) => void;
  onOrderChange: (order: SortOrder) => void;
  onTypeChange: (type: TypeFilter) => void;
  onRefresh: () => void;
  refreshing: boolean;
}

const TYPE_OPTIONS: Array<{ value: TypeFilter; label: string }> = [
  { value: 'all', label: 'All' },
  { value: 'photo', label: 'Photos' },
  { value: 'video', label: 'Videos' },
];

export function Toolbar({
  total,
  sort,
  order,
  type,
  onSortChange,
  onOrderChange,
  onTypeChange,
  onRefresh,
  refreshing,
}: ToolbarProps) {
  return (
    <div className="toolbar" role="toolbar" aria-label="Item list controls">
      <span className="toolbar-count" aria-live="polite">
        {total} item{total === 1 ? '' : 's'}
      </span>
      <div className="toolbar-controls">
        <div className="seg" role="group" aria-label="Filter by type">
          {TYPE_OPTIONS.map((opt) => (
            <button
              key={opt.value}
              type="button"
              className="seg-btn"
              aria-pressed={type === opt.value}
              onClick={() => onTypeChange(opt.value)}
            >
              {opt.label}
            </button>
          ))}
        </div>
        <label className="sr-only" htmlFor="toolbar-sort">
          Sort by
        </label>
        <select
          id="toolbar-sort"
          className="input toolbar-select"
          value={sort}
          onChange={(e) => onSortChange(e.target.value as SortField)}
        >
          <option value="date">Date</option>
          <option value="name">Name</option>
          <option value="size">Size</option>
          <option value="type">Type</option>
        </select>
        <button
          type="button"
          className="btn-icon toolbar-order"
          aria-label={order === 'desc' ? 'Sort ascending' : 'Sort descending'}
          aria-pressed={order === 'asc'}
          onClick={() => onOrderChange(order === 'desc' ? 'asc' : 'desc')}
        >
          {order === 'desc' ? <ArrowDownIcon /> : <ArrowUpIcon />}
        </button>
        <button
          type="button"
          className="btn-icon"
          aria-label="Refresh items"
          onClick={onRefresh}
          disabled={refreshing}
        >
          {refreshing ? <SpinnerIcon /> : <RefreshIcon />}
        </button>
      </div>
    </div>
  );
}
