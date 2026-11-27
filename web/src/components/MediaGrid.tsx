import { useCallback, useRef, useState, type ReactNode } from 'react';
import type { Item } from '../api/types';
import { formatDurationMs } from '../player/time';
import { HeartIcon, PlayIcon } from './Icons';

interface TileProps {
  item: Item;
  thumbSrc: string;
  onOpen: () => void;
  onToggleFavorite: (item: Item) => void;
}

export function Tile({ item, thumbSrc, onOpen, onToggleFavorite }: TileProps) {
  const [loaded, setLoaded] = useState(false);
  const [failed, setFailed] = useState(false);
  const duration = item.type === 'video' ? formatDurationMs(item.durationMs) : '';

  return (
    <div className="tile-wrap">
      <button
        className="tile"
        onClick={onOpen}
        aria-label={`${item.name}, ${item.type === 'video' ? `video, ${duration || 'unknown duration'}` : 'photo'}`}
      >
        <span className="tile-media">
          {!loaded && !failed && <span className="tile-skeleton skeleton" aria-hidden="true" />}
          {failed ? (
            <span className="tile-fallback" aria-hidden="true" />
          ) : (
            <img
              src={thumbSrc}
              alt=""
              loading="lazy"
              decoding="async"
              onLoad={() => setLoaded(true)}
              onError={() => setFailed(true)}
            />
          )}
          {item.type === 'video' && (
            <>
              <span className="tile-play" aria-hidden="true">
                <PlayIcon size={22} />
              </span>
              {duration && <span className="tile-duration">{duration}</span>}
            </>
          )}
        </span>
      </button>
      <button
        className="tile-fav"
        onClick={() => onToggleFavorite(item)}
        aria-pressed={item.favorite}
        aria-label={item.favorite ? `Remove ${item.name} from favorites` : `Add ${item.name} to favorites`}
      >
        <HeartIcon size={17} filled={item.favorite} />
      </button>
    </div>
  );
}

interface MediaGridProps {
  items: Item[];
  thumbSrc: (item: Item) => string;
  onOpen: (index: number) => void;
  onToggleFavorite: (item: Item) => void;
  /** Rendered when there are more pages to load (infinite scroll sentinel). */
  sentinel: ReactNode;
}

export function MediaGrid({ items, thumbSrc, onOpen, onToggleFavorite, sentinel }: MediaGridProps) {
  const containerRef = useRef<HTMLDivElement>(null);

  // Roving arrow-key navigation between tiles.
  const onKeyDown = useCallback((e: React.KeyboardEvent<HTMLDivElement>) => {
    const container = containerRef.current;
    if (!container) return;
    const tiles = Array.from(container.querySelectorAll<HTMLButtonElement>('.tile'));
    if (tiles.length === 0) return;
    const active = document.activeElement as HTMLButtonElement | null;
    const idx = active ? tiles.indexOf(active) : -1;
    if (idx === -1) return;

    // Column count = number of tiles on the first row.
    const firstTop = tiles[0].offsetTop;
    let cols = 0;
    while (cols < tiles.length && tiles[cols].offsetTop === firstTop) cols += 1;
    if (cols < 1) cols = 1;

    let next = idx;
    switch (e.key) {
      case 'ArrowRight':
        next = Math.min(idx + 1, tiles.length - 1);
        break;
      case 'ArrowLeft':
        next = Math.max(idx - 1, 0);
        break;
      case 'ArrowDown':
        next = Math.min(idx + cols, tiles.length - 1);
        break;
      case 'ArrowUp':
        next = Math.max(idx - cols, 0);
        break;
      case 'Home':
        next = 0;
        break;
      case 'End':
        next = tiles.length - 1;
        break;
      default:
        return;
    }
    if (next !== idx) {
      e.preventDefault();
      tiles[next].focus();
      tiles[next].scrollIntoView({ block: 'nearest' });
    }
  }, []);

  return (
    <div className="grid" ref={containerRef} onKeyDown={onKeyDown} role="listbox" aria-label="Media items">
      {items.map((item, index) => (
        <Tile
          key={item.id}
          item={item}
          thumbSrc={thumbSrc(item)}
          onOpen={() => onOpen(index)}
          onToggleFavorite={onToggleFavorite}
        />
      ))}
      {sentinel}
    </div>
  );
}

export function GridSkeleton({ count = 24 }: { count?: number }) {
  return (
    <div className="grid" aria-hidden="true">
      {Array.from({ length: count }, (_, i) => (
        <div key={i} className="tile-wrap">
          <div className="tile-skeleton skeleton" style={{ aspectRatio: '1 / 1' }} />
        </div>
      ))}
    </div>
  );
}
