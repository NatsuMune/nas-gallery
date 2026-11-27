import { useCallback, useEffect, useRef } from 'react';
import { joinUrl } from '../api/client';
import type { Item } from '../api/types';
import { downloadUrlFor, VideoPlayer } from './VideoPlayer';
import { PhotoViewer } from './PhotoViewer';
import { ChevronLeftIcon, ChevronRightIcon, CloseIcon, DownloadIcon, HeartIcon } from './Icons';

interface LightboxProps {
  items: Item[];
  index: number;
  serverUrl: string;
  onIndexChange: (index: number) => void;
  onClose: () => void;
  onToggleFavorite: (item: Item) => void;
  onRefreshItem: (id: string) => Promise<Item | null>;
}

function formatBytes(bytes: number): string {
  if (!Number.isFinite(bytes) || bytes < 0) return '—';
  if (bytes < 1024) return `${bytes} B`;
  const units = ['KB', 'MB', 'GB', 'TB'];
  let value = bytes;
  let unit = -1;
  do {
    value /= 1024;
    unit += 1;
  } while (value >= 1024 && unit < units.length - 1);
  return `${value.toFixed(value >= 100 ? 0 : 1)} ${units[unit]}`;
}

function formatDate(iso: string): string {
  const date = new Date(iso);
  if (Number.isNaN(date.getTime())) return iso;
  return date.toLocaleDateString(undefined, { year: 'numeric', month: 'short', day: 'numeric' });
}

export function Lightbox({
  items,
  index,
  serverUrl,
  onIndexChange,
  onClose,
  onToggleFavorite,
  onRefreshItem,
}: LightboxProps) {
  const dialogRef = useRef<HTMLDivElement>(null);
  const closeRef = useRef<HTMLButtonElement>(null);
  const item = items[index];

  const goTo = useCallback(
    (delta: -1 | 1) => {
      if (items.length < 2) return;
      onIndexChange((index + delta + items.length) % items.length);
    },
    [items.length, index, onIndexChange],
  );

  // Focus management + body scroll lock.
  useEffect(() => {
    closeRef.current?.focus();
    const prevOverflow = document.body.style.overflow;
    document.body.style.overflow = 'hidden';
    return () => {
      document.body.style.overflow = prevOverflow;
    };
  }, []);

  // Keyboard: Esc closes; arrows navigate (photo mode only — the video
  // player owns arrow keys for seeking while a video is open).
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        e.preventDefault();
        onClose();
        return;
      }
      if (item?.type === 'photo') {
        if (e.key === 'ArrowLeft') {
          e.preventDefault();
          goTo(-1);
        } else if (e.key === 'ArrowRight') {
          e.preventDefault();
          goTo(1);
        }
      }
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [item?.type, goTo, onClose]);

  if (!item) return null;

  const meta: string[] = [];
  if (item.width && item.height) meta.push(`${item.width} × ${item.height}`);
  meta.push(formatBytes(item.size));
  meta.push(formatDate(item.modTime));

  return (
    <div className="lightbox-backdrop" onPointerDown={(e) => e.stopPropagation()}>
      <div
        className="lightbox"
        ref={dialogRef}
        role="dialog"
        aria-modal="true"
        aria-label={`${item.name}, item ${index + 1} of ${items.length}`}
      >
        <header className="lightbox-header">
          <div className="lightbox-title">
            <h2 className="lightbox-name">{item.name}</h2>
            <span className="lightbox-meta">{meta.join(' · ')}</span>
          </div>
          <div className="lightbox-actions">
            <span className="lightbox-counter" aria-live="polite">
              {index + 1} / {items.length}
            </span>
            <button
              type="button"
              className="btn-icon lightbox-action"
              aria-pressed={item.favorite}
              aria-label={item.favorite ? `Remove ${item.name} from favorites` : `Add ${item.name} to favorites`}
              onClick={() => onToggleFavorite(item)}
            >
              <HeartIcon filled={item.favorite} />
            </button>
            <a
              className="btn-icon lightbox-action lightbox-download"
              href={downloadUrlFor(serverUrl, item)}
              download={item.name}
              aria-label={`Download ${item.name}`}
              title={`Download ${item.name}`}
            >
              <DownloadIcon />
            </a>
            <button type="button" className="btn-icon lightbox-action" aria-label="Close viewer" onClick={onClose} ref={closeRef}>
              <CloseIcon />
            </button>
          </div>
        </header>

        <div className="lightbox-stage">
          {item.type === 'video' ? (
            <VideoPlayer item={item} serverUrl={serverUrl} onRefreshItem={onRefreshItem} onNavigate={goTo} />
          ) : (
            <PhotoViewer item={item} src={joinUrl(serverUrl, item.streamUrl)} onNavigate={goTo} />
          )}

          {item.type === 'photo' && (
            <>
              <button type="button" className="lightbox-nav lightbox-nav-left" aria-label="Previous item" onClick={() => goTo(-1)}>
                <ChevronLeftIcon size={26} />
              </button>
              <button type="button" className="lightbox-nav lightbox-nav-right" aria-label="Next item" onClick={() => goTo(1)}>
                <ChevronRightIcon size={26} />
              </button>
            </>
          )}
        </div>
      </div>
    </div>
  );
}
