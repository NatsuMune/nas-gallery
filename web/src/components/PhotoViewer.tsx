import { useCallback, useEffect, useRef, useState } from 'react';
import type { Item } from '../api/types';

interface PhotoViewerProps {
  item: Item;
  src: string;
  onNavigate: (delta: -1 | 1) => void;
}

interface Transform {
  scale: number;
  tx: number;
  ty: number;
}

const IDENTITY: Transform = { scale: 1, tx: 0, ty: 0 };
const MIN_SCALE = 1;
const MAX_SCALE = 5;
const ZOOM_STEP = 0.25;
const DOUBLE_CLICK_SCALE = 2.5;
const SWIPE_THRESHOLD = 60;

/** Zoomable / pannable photo surface with swipe navigation and pinch zoom. */
export function PhotoViewer({ item, src, onNavigate }: PhotoViewerProps) {
  const containerRef = useRef<HTMLDivElement>(null);
  const [transform, setTransform] = useState<Transform>(IDENTITY);
  const transformRef = useRef(transform);
  transformRef.current = transform;
  const [dragging, setDragging] = useState(false);
  const pointers = useRef(new Map<number, { x: number; y: number }>());
  const pinchBase = useRef<{ dist: number; scale: number; midX: number; midY: number; tx: number; ty: number } | null>(null);
  const swipeStart = useRef<{ x: number; y: number } | null>(null);
  const [loaded, setLoaded] = useState(false);

  // Reset zoom when switching items.
  useEffect(() => {
    setTransform(IDENTITY);
    setLoaded(false);
  }, [item.id]);

  const clampScale = (scale: number): number => Math.min(Math.max(scale, MIN_SCALE), MAX_SCALE);

  const zoomTo = useCallback((nextScale: number, clientX?: number, clientY?: number) => {
    const container = containerRef.current;
    const rect = container?.getBoundingClientRect();
    // Point relative to the container center; defaults to center.
    const px = rect && clientX !== undefined ? clientX - rect.left - rect.width / 2 : 0;
    const py = rect && clientY !== undefined ? clientY - rect.top - rect.height / 2 : 0;
    setTransform((prev) => {
      const scale = clampScale(nextScale);
      const k = scale / prev.scale;
      if (scale === MIN_SCALE) return IDENTITY;
      return { scale, tx: px - k * (px - prev.tx), ty: py - k * (py - prev.ty) };
    });
  }, []);

  // Keyboard zoom (+/-).
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      const target = e.target as HTMLElement | null;
      if (target && (target.tagName === 'INPUT' || target.tagName === 'SELECT' || target.isContentEditable)) return;
      if (e.key === '+' || e.key === '=') {
        e.preventDefault();
        zoomTo(transformRef.current.scale + ZOOM_STEP);
      } else if (e.key === '-' || e.key === '_') {
        e.preventDefault();
        zoomTo(transformRef.current.scale - ZOOM_STEP);
      } else if (e.key === '0') {
        setTransform(IDENTITY);
      }
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [zoomTo]);

  const onDoubleClick = (e: React.MouseEvent) => {
    const current = transformRef.current;
    zoomTo(current.scale > 1 ? MIN_SCALE : DOUBLE_CLICK_SCALE, e.clientX, e.clientY);
  };

  // --- Pointer: pan (1 pointer, zoomed), pinch (2 pointers), swipe (1 pointer, 1x) ---

  const onPointerDown = (e: React.PointerEvent) => {
    (e.target as HTMLElement).setPointerCapture?.(e.pointerId);
    pointers.current.set(e.pointerId, { x: e.clientX, y: e.clientY });
    if (pointers.current.size === 2) {
      const [p1, p2] = Array.from(pointers.current.values());
      const t = transformRef.current;
      pinchBase.current = {
        dist: Math.hypot(p1.x - p2.x, p1.y - p2.y),
        scale: t.scale,
        midX: (p1.x + p2.x) / 2,
        midY: (p1.y + p2.y) / 2,
        tx: t.tx,
        ty: t.ty,
      };
    } else if (pointers.current.size === 1) {
      swipeStart.current = { x: e.clientX, y: e.clientY };
      if (transformRef.current.scale > 1) setDragging(true);
    }
  };

  const onPointerMove = (e: React.PointerEvent) => {
    const prev = pointers.current.get(e.pointerId);
    if (!prev) return;
    pointers.current.set(e.pointerId, { x: e.clientX, y: e.clientY });

    if (pointers.current.size >= 2 && pinchBase.current) {
      const [p1, p2] = Array.from(pointers.current.values());
      const dist = Math.hypot(p1.x - p2.x, p1.y - p2.y);
      const container = containerRef.current;
      const rect = container?.getBoundingClientRect();
      const scale = clampScale((pinchBase.current.scale * dist) / pinchBase.current.dist);
      const k = scale / pinchBase.current.scale;
      if (rect) {
        const midX = (p1.x + p2.x) / 2 - rect.left - rect.width / 2;
        const midY = (p1.y + p2.y) / 2 - rect.top - rect.height / 2;
        const anchorX = pinchBase.current.midX - rect.left - rect.width / 2;
        const anchorY = pinchBase.current.midY - rect.top - rect.height / 2;
        if (scale <= MIN_SCALE) setTransform(IDENTITY);
        else setTransform({ scale, tx: midX - k * (anchorX - pinchBase.current.tx), ty: midY - k * (anchorY - pinchBase.current.ty) });
      }
      return;
    }

    if (pointers.current.size === 1 && transformRef.current.scale > 1) {
      setTransform((t) => ({ ...t, tx: t.tx + (e.clientX - prev.x), ty: t.ty + (e.clientY - prev.y) }));
    }
  };

  const onPointerUp = (e: React.PointerEvent) => {
    pointers.current.delete(e.pointerId);
    if (pointers.current.size < 2) pinchBase.current = null;
    if (pointers.current.size === 0) {
      setDragging(false);
      const start = swipeStart.current;
      swipeStart.current = null;
      if (start && transformRef.current.scale === MIN_SCALE) {
        const dx = e.clientX - start.x;
        const dy = e.clientY - start.y;
        if (Math.abs(dx) > SWIPE_THRESHOLD && Math.abs(dy) < SWIPE_THRESHOLD) {
          onNavigate(dx < 0 ? 1 : -1);
        }
      }
    }
  };

  // Trackpad pinch = ctrl+wheel.
  useEffect(() => {
    const container = containerRef.current;
    if (!container) return;
    const onWheel = (e: WheelEvent) => {
      if (!e.ctrlKey) return;
      e.preventDefault();
      const t = transformRef.current;
      zoomTo(t.scale - e.deltaY * 0.01, e.clientX, e.clientY);
    };
    container.addEventListener('wheel', onWheel, { passive: false });
    return () => container.removeEventListener('wheel', onWheel);
  }, [zoomTo]);

  const zoomed = transform.scale > 1;

  return (
    <div
      className={`photo-viewer${zoomed ? ' photo-viewer-zoomed' : ''}${dragging ? ' photo-viewer-dragging' : ''}`}
      ref={containerRef}
      onDoubleClick={onDoubleClick}
      onPointerDown={onPointerDown}
      onPointerMove={onPointerMove}
      onPointerUp={onPointerUp}
      onPointerCancel={onPointerUp}
    >
      <div
        className="photo-transform"
        style={{ transform: `translate(${transform.tx}px, ${transform.ty}px) scale(${transform.scale})` }}
      >
        {!loaded && <div className="photo-skeleton skeleton" aria-hidden="true" />}
        <img
          src={src}
          alt={item.name}
          draggable={false}
          onLoad={() => setLoaded(true)}
        />
      </div>
    </div>
  );
}
