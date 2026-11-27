import { useCallback, useEffect, useRef, useState } from 'react';
import { joinUrl } from '../api/client';
import type { Item } from '../api/types';
import {
  clearResumePosition,
  formatClock,
  isMostlyWatched,
  loadResumePosition,
  saveResumePosition,
} from '../player/time';
import { loadVolumePrefs, saveVolumePrefs } from '../player/prefs';
import {
  Back30Icon,
  Forward30Icon,
  FullscreenExitIcon,
  FullscreenIcon,
  PauseIcon,
  PlayIcon,
  RefreshIcon,
  DownloadIcon,
  SpinnerIcon,
  VolumeIcon,
  VolumeMuteIcon,
  ChevronLeftIcon,
  ChevronRightIcon,
} from './Icons';

const SPEEDS = [0.5, 0.75, 1, 1.25, 1.5, 2] as const;
const HIDE_DELAY_MS = 3000;

export function downloadUrlFor(serverUrl: string, item: Item): string {
  const base = joinUrl(serverUrl, item.streamUrl);
  return base + (base.includes('?') ? '&' : '?') + 'download=1';
}

interface VideoPlayerProps {
  item: Item;
  serverUrl: string;
  /** Refetch the item for fresh signed URLs; resolves null on failure. */
  onRefreshItem: (id: string) => Promise<Item | null>;
  onNavigate?: (delta: -1 | 1) => void;
}

type ErrorState = 'none' | 'retrying' | 'failed';

export function VideoPlayer({ item, serverUrl, onRefreshItem, onNavigate }: VideoPlayerProps) {
  const videoRef = useRef<HTMLVideoElement>(null);
  const containerRef = useRef<HTMLDivElement>(null);
  const seekRef = useRef<HTMLDivElement>(null);
  const speedMenuRef = useRef<HTMLDivElement>(null);

  const hideTimerRef = useRef<number | null>(null);
  const clickTimerRef = useRef<number | null>(null);
  const retryCountRef = useRef(0);
  const lastSaveRef = useRef(0);
  const controlsHoverRef = useRef(false);
  const speedOpenRef = useRef(false);
  const seekDraggingRef = useRef(false);

  const [playing, setPlaying] = useState(false);
  const [currentTime, setCurrentTime] = useState(0);
  const [duration, setDuration] = useState(() => (item.durationMs ? item.durationMs / 1000 : 0));
  const [buffered, setBuffered] = useState<Array<[number, number]>>([]);
  const [volumePrefs] = useState(loadVolumePrefs);
  const [volume, setVolume] = useState(volumePrefs.volume);
  const [muted, setMuted] = useState(volumePrefs.muted);
  const [rate, setRate] = useState<number>(1);
  const [speedMenuOpen, setSpeedMenuOpen] = useState(false);
  const [isFullscreen, setFullscreen] = useState(false);
  const [controlsVisible, setControlsVisible] = useState(true);
  const [waiting, setWaiting] = useState(false);
  const [errorState, setErrorState] = useState<ErrorState>('none');
  const [hover, setHover] = useState<{ time: number; x: number } | null>(null);

  const src = joinUrl(serverUrl, item.streamUrl);

  // Reset player state when the media source changes (item switch or a
  // refetch with fresh signed URLs after an error retry).
  useEffect(() => {
    retryCountRef.current = 0;
    setErrorState('none');
    setCurrentTime(0);
    setBuffered([]);
    setPlaying(false);
    setDuration(item.durationMs ? item.durationMs / 1000 : 0);
    lastSaveRef.current = 0;
  }, [src, item.id]);

  // Apply persisted volume/mute + playback rate to the element.
  useEffect(() => {
    const v = videoRef.current;
    if (v) {
      v.volume = volume;
      v.muted = muted;
    }
    saveVolumePrefs({ volume, muted });
  }, [volume, muted]);

  useEffect(() => {
    const v = videoRef.current;
    if (v) v.playbackRate = rate;
  }, [rate, src]);

  // Save the resume position on unmount / page hide.
  useEffect(() => {
    const save = () => {
      const v = videoRef.current;
      if (v && v.currentTime > 0 && Number.isFinite(v.duration)) {
        saveResumePosition(item.id, v.currentTime, v.duration);
      }
    };
    window.addEventListener('pagehide', save);
    return () => {
      window.removeEventListener('pagehide', save);
      save();
    };
  }, [item.id]);

  // Auto-hiding control bar.
  const showControls = useCallback(() => {
    setControlsVisible(true);
    if (hideTimerRef.current !== null) window.clearTimeout(hideTimerRef.current);
    hideTimerRef.current = window.setTimeout(() => {
      const v = videoRef.current;
      if (v && !v.paused && !controlsHoverRef.current && !speedOpenRef.current && !seekDraggingRef.current) {
        setControlsVisible(false);
      }
    }, HIDE_DELAY_MS);
  }, []);

  useEffect(() => {
    if (speedMenuOpen) showControls();
    speedOpenRef.current = speedMenuOpen;
  }, [speedMenuOpen, showControls]);

  useEffect(() => {
    return () => {
      if (hideTimerRef.current !== null) window.clearTimeout(hideTimerRef.current);
      if (clickTimerRef.current !== null) window.clearTimeout(clickTimerRef.current);
    };
  }, []);

  // --- Playback actions ---

  const togglePlay = useCallback(() => {
    const v = videoRef.current;
    if (!v || errorState !== 'none') return;
    if (v.paused) {
      void v.play().catch(() => {
        // Autoplay restrictions or decode failure; the error event covers it.
      });
    } else {
      v.pause();
    }
  }, [errorState]);

  const seekBy = useCallback((delta: number) => {
    const v = videoRef.current;
    if (!v || errorState !== 'none' || !Number.isFinite(v.duration) || v.duration <= 0) return;
    v.currentTime = Math.min(Math.max(v.currentTime + delta, 0), v.duration);
    setCurrentTime(v.currentTime);
  }, [errorState]);

  const seekTo = useCallback((seconds: number) => {
    const v = videoRef.current;
    if (!v || !Number.isFinite(v.duration) || v.duration <= 0) return;
    v.currentTime = Math.min(Math.max(seconds, 0), v.duration);
    setCurrentTime(v.currentTime);
  }, []);

  const changeVolume = useCallback((delta: number) => {
    setVolume((prev) => {
      const next = Math.min(Math.max(prev + delta, 0), 1);
      return next;
    });
    if (delta > 0) setMuted(false);
  }, []);

  const toggleMute = useCallback(() => setMuted((m) => !m), []);

  const toggleFullscreen = useCallback(() => {
    if (document.fullscreenElement) {
      void document.exitFullscreen().catch(() => undefined);
    } else {
      void containerRef.current?.requestFullscreen().catch(() => undefined);
    }
  }, []);

  // --- Keyboard shortcuts (window level while the player is mounted) ---

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      const target = e.target as HTMLElement | null;
      const interactive =
        target &&
        (target.tagName === 'INPUT' ||
          target.tagName === 'SELECT' ||
          target.tagName === 'TEXTAREA' ||
          target.tagName === 'BUTTON' ||
          target.tagName === 'A' ||
          target.isContentEditable);
      // Let the browser activate focused controls with Space; everything
      // else is a player shortcut.
      if (interactive && (e.key === ' ' || e.key === 'Enter')) return;
      switch (e.key) {
        case ' ':
        case 'k':
        case 'K':
          e.preventDefault();
          togglePlay();
          break;
        case 'j':
        case 'J':
          seekBy(-30);
          break;
        case 'l':
        case 'L':
          seekBy(30);
          break;
        case 'ArrowLeft':
          e.preventDefault();
          seekBy(-10);
          break;
        case 'ArrowRight':
          e.preventDefault();
          seekBy(10);
          break;
        case 'ArrowUp':
          e.preventDefault();
          changeVolume(0.1);
          break;
        case 'ArrowDown':
          e.preventDefault();
          changeVolume(-0.1);
          break;
        case 'm':
        case 'M':
          toggleMute();
          break;
        case 'f':
        case 'F':
          toggleFullscreen();
          break;
        default:
          return;
      }
      showControls();
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [togglePlay, seekBy, changeVolume, toggleMute, toggleFullscreen, showControls]);

  // --- Media element events ---

  const updateBuffered = useCallback(() => {
    const v = videoRef.current;
    if (!v) return;
    const b = v.buffered;
    const ranges: Array<[number, number]> = [];
    for (let i = 0; i < b.length; i++) ranges.push([b.start(i), b.end(i)]);
    setBuffered(ranges);
  }, []);

  const handleLoadedMetadata = useCallback(() => {
    const v = videoRef.current;
    if (!v) return;
    if (Number.isFinite(v.duration) && v.duration > 0) setDuration(v.duration);
    // Resume playback position across sessions.
    const pos = loadResumePosition(item.id, v.duration);
    if (pos !== null) v.currentTime = pos;
  }, [item.id]);

  const handleTimeUpdate = useCallback(() => {
    const v = videoRef.current;
    if (!v) return;
    setCurrentTime(v.currentTime);
    updateBuffered();
    if (v.currentTime - lastSaveRef.current >= 5) {
      lastSaveRef.current = v.currentTime;
      saveResumePosition(item.id, v.currentTime, v.duration);
    }
    if (isMostlyWatched(v.currentTime, v.duration)) clearResumePosition(item.id);
  }, [item.id, updateBuffered]);

  // --- Error handling: refetch fresh signed URLs, then fall back ---

  const handleError = useCallback(() => {
    if (retryCountRef.current >= 2) {
      setErrorState('failed');
      return;
    }
    retryCountRef.current += 1;
    setErrorState('retrying');
    onRefreshItem(item.id)
      .then((fresh) => {
        if (!fresh || fresh.streamUrl === item.streamUrl) setErrorState('failed');
        // else: new signed URL resets state via the src effect and reloads.
      })
      .catch(() => setErrorState('failed'));
  }, [item.id, item.streamUrl, onRefreshItem]);

  const manualRetry = useCallback(() => {
    retryCountRef.current = 0;
    setErrorState('retrying');
    onRefreshItem(item.id)
      .then((fresh) => {
        if (!fresh || fresh.streamUrl === item.streamUrl) setErrorState('failed');
      })
      .catch(() => setErrorState('failed'));
  }, [item.id, item.streamUrl, onRefreshItem]);

  // --- Seek bar pointer logic (mouse + touch) ---

  const ratioFromPointer = (e: React.PointerEvent): number => {
    const el = seekRef.current;
    if (!el) return 0;
    const rect = el.getBoundingClientRect();
    return Math.min(Math.max((e.clientX - rect.left) / rect.width, 0), 1);
  };

  const onSeekPointerDown = (e: React.PointerEvent) => {
    e.preventDefault();
    seekRef.current?.setPointerCapture(e.pointerId);
    seekDraggingRef.current = true;
    const v = videoRef.current;
    const dur = v && Number.isFinite(v.duration) ? v.duration : duration;
    seekTo(ratioFromPointer(e) * dur);
    showControls();
  };

  const onSeekPointerMove = (e: React.PointerEvent) => {
    const el = seekRef.current;
    if (!el) return;
    const rect = el.getBoundingClientRect();
    const ratio = ratioFromPointer(e);
    const v = videoRef.current;
    const dur = v && Number.isFinite(v.duration) ? v.duration : duration;
    setHover({ time: ratio * dur, x: Math.min(Math.max(e.clientX - rect.left, 0), rect.width) });
    if (seekDraggingRef.current) seekTo(ratio * dur);
  };

  const onSeekPointerUp = (e: React.PointerEvent) => {
    seekDraggingRef.current = false;
    try {
      seekRef.current?.releasePointerCapture(e.pointerId);
    } catch {
      // pointer already released
    }
    showControls();
  };

  // --- Volume slider pointer logic ---

  const volRef = useRef<HTMLDivElement>(null);
  const volDraggingRef = useRef(false);

  const volumeFromPointer = (e: React.PointerEvent): number => {
    const el = volRef.current;
    if (!el) return 0;
    const rect = el.getBoundingClientRect();
    return Math.min(Math.max((e.clientX - rect.left) / rect.width, 0), 1);
  };

  const onVolPointerDown = (e: React.PointerEvent) => {
    e.preventDefault();
    volRef.current?.setPointerCapture(e.pointerId);
    volDraggingRef.current = true;
    setVolume(volumeFromPointer(e));
    setMuted(false);
  };

  const onVolPointerMove = (e: React.PointerEvent) => {
    if (volDraggingRef.current) setVolume(volumeFromPointer(e));
  };

  const onVolPointerUp = (e: React.PointerEvent) => {
    volDraggingRef.current = false;
    try {
      volRef.current?.releasePointerCapture(e.pointerId);
    } catch {
      // ignore
    }
  };

  // --- Click vs double-click on the video surface ---

  const handleVideoClick = () => {
    if (clickTimerRef.current !== null) {
      window.clearTimeout(clickTimerRef.current);
      clickTimerRef.current = null;
      toggleFullscreen();
      return;
    }
    clickTimerRef.current = window.setTimeout(() => {
      clickTimerRef.current = null;
      togglePlay();
    }, 220);
  };

  // Speed menu click-away.
  useEffect(() => {
    if (!speedMenuOpen) return;
    const close = (e: MouseEvent) => {
      if (!speedMenuRef.current?.contains(e.target as Node)) setSpeedMenuOpen(false);
    };
    document.addEventListener('click', close);
    return () => document.removeEventListener('click', close);
  }, [speedMenuOpen]);

  // Fullscreen state sync.
  useEffect(() => {
    const onFsChange = () => setFullscreen(Boolean(document.fullscreenElement));
    document.addEventListener('fullscreenchange', onFsChange);
    return () => document.removeEventListener('fullscreenchange', onFsChange);
  }, []);

  const pct = (value: number): string => (duration > 0 ? `${Math.min((value / duration) * 100, 100)}%` : '0%');

  const showPrevNext = onNavigate !== undefined;
  const canPrev = showPrevNext;
  const canNext = showPrevNext;

  return (
    <div
      className={`video-player${isFullscreen ? ' video-player-fullscreen' : ''}`}
      ref={containerRef}
      onPointerMove={showControls}
      onPointerLeave={() => {
        if (playing && !controlsHoverRef.current) setControlsVisible(false);
      }}
    >
      <video
        ref={videoRef}
        src={src}
        playsInline
        preload="metadata"
        poster={joinUrl(serverUrl, item.thumbUrl)}
        aria-label={`Video: ${item.name}`}
        onClick={handleVideoClick}
        onDoubleClick={toggleFullscreen}
        onLoadedMetadata={handleLoadedMetadata}
        onTimeUpdate={handleTimeUpdate}
        onProgress={updateBuffered}
        onDurationChange={() => {
          const v = videoRef.current;
          if (v && Number.isFinite(v.duration) && v.duration > 0) setDuration(v.duration);
        }}
        onPlay={() => {
          setPlaying(true);
          showControls();
        }}
        onPause={() => {
          setPlaying(false);
          setControlsVisible(true);
          const v = videoRef.current;
          if (v && v.currentTime > 0) saveResumePosition(item.id, v.currentTime, v.duration);
        }}
        onEnded={() => {
          clearResumePosition(item.id);
          setPlaying(false);
          setControlsVisible(true);
        }}
        onWaiting={() => setWaiting(true)}
        onPlaying={() => setWaiting(false)}
        onCanPlay={() => setWaiting(false)}
        onError={handleError}
      />

      {waiting && errorState === 'none' && (
        <div className="video-spinner" aria-label="Buffering">
          <SpinnerIcon size={36} />
        </div>
      )}

      {!playing && errorState === 'none' && !waiting && (
        <button type="button" className="video-big-play" aria-label="Play video" onClick={togglePlay}>
          <PlayIcon size={34} />
        </button>
      )}

      {errorState !== 'none' && (
        <div className="video-error-overlay" role="alert">
          {errorState === 'retrying' ? (
            <div className="video-error-retry">
              <SpinnerIcon size={30} />
              <p>Refreshing streaming link…</p>
            </div>
          ) : (
            <div className="video-error-failed">
              <p className="video-error-title">Playback failed</p>
              <p className="video-error-hint">
                This video could not be played. The format may not be supported by your browser, or the streaming
                link expired.
              </p>
              <div className="video-error-actions">
                <button type="button" className="btn" onClick={manualRetry}>
                  <RefreshIcon size={16} />
                  Retry
                </button>
                <a className="btn btn-primary" href={downloadUrlFor(serverUrl, item)} download={item.name}>
                  <DownloadIcon size={16} />
                  Download to watch
                </a>
              </div>
            </div>
          )}
        </div>
      )}

      {showPrevNext && (
        <>
          <button
            type="button"
            className={`lightbox-nav lightbox-nav-left${controlsVisible ? '' : ' lightbox-nav-hidden'}`}
            aria-label="Previous item"
            disabled={!canPrev}
            onClick={() => onNavigate?.(-1)}
          >
            <ChevronLeftIcon size={26} />
          </button>
          <button
            type="button"
            className={`lightbox-nav lightbox-nav-right${controlsVisible ? '' : ' lightbox-nav-hidden'}`}
            aria-label="Next item"
            disabled={!canNext}
            onClick={() => onNavigate?.(1)}
          >
            <ChevronRightIcon size={26} />
          </button>
        </>
      )}

      <div
        className={`player-controls${controlsVisible ? '' : ' player-controls-hidden'}`}
        onPointerEnter={() => {
          controlsHoverRef.current = true;
        }}
        onPointerLeave={() => {
          controlsHoverRef.current = false;
        }}
      >
        <div
          className="player-seek"
          ref={seekRef}
          role="slider"
          tabIndex={0}
          aria-label="Seek"
          aria-valuemin={0}
          aria-valuemax={Math.max(duration, 0.1)}
          aria-valuenow={Math.floor(currentTime)}
          aria-valuetext={`${formatClock(currentTime)} of ${formatClock(duration)}`}
          onPointerDown={onSeekPointerDown}
          onPointerMove={onSeekPointerMove}
          onPointerUp={onSeekPointerUp}
          onPointerCancel={onSeekPointerUp}
          onPointerLeave={() => {
            if (!seekDraggingRef.current) setHover(null);
          }}
        >
          <div className="player-seek-track">
            {buffered.map(([start, end], i) => (
              <span key={i} className="player-seek-buffered" style={{ left: pct(start), width: pct(end - start) }} />
            ))}
            <span className="player-seek-played" style={{ width: pct(currentTime) }} />
            <span className="player-seek-thumb" style={{ left: pct(currentTime) }} />
          </div>
          {hover && (
            <span className="player-seek-tooltip" style={{ left: hover.x }} role="tooltip">
              {formatClock(hover.time)}
            </span>
          )}
        </div>

        <div className="player-buttons">
          <button
            type="button"
            className="btn-icon player-btn"
            aria-label={playing ? 'Pause' : 'Play'}
            onClick={togglePlay}
          >
            {playing ? <PauseIcon /> : <PlayIcon />}
          </button>
          <button type="button" className="btn-icon player-btn" aria-label="Back 30 seconds" onClick={() => seekBy(-30)}>
            <Back30Icon />
          </button>
          <button type="button" className="btn-icon player-btn" aria-label="Forward 30 seconds" onClick={() => seekBy(30)}>
            <Forward30Icon />
          </button>

          <span className="player-time" aria-label={`Playback time ${formatClock(currentTime)} of ${formatClock(duration)}`}>
            <span className="player-time-current">{formatClock(currentTime)}</span>
            <span className="player-time-sep">/</span>
            <span className="player-time-total">{formatClock(duration)}</span>
          </span>

          <span className="player-volume">
            <button
              type="button"
              className="btn-icon player-btn"
              aria-label={muted || volume === 0 ? 'Unmute' : 'Mute'}
              onClick={toggleMute}
            >
              {muted || volume === 0 ? <VolumeMuteIcon /> : <VolumeIcon />}
            </button>
            <div
              className="player-vol-slider"
              ref={volRef}
              role="slider"
              tabIndex={0}
              aria-label="Volume"
              aria-valuemin={0}
              aria-valuemax={1}
              aria-valuenow={Math.round(volume * 100) / 100}
              aria-valuetext={`${Math.round((muted ? 0 : volume) * 100)}% volume`}
              onPointerDown={onVolPointerDown}
              onPointerMove={onVolPointerMove}
              onPointerUp={onVolPointerUp}
              onPointerCancel={onVolPointerUp}
            >
              <div className="player-vol-track">
                <span className="player-vol-fill" style={{ width: `${(muted ? 0 : volume) * 100}%` }} />
                <span className="player-vol-thumb" style={{ left: `${(muted ? 0 : volume) * 100}%` }} />
              </div>
            </div>
          </span>

          <div className="player-speed" ref={speedMenuRef}>
            <button
              type="button"
              className="player-speed-btn"
              aria-haspopup="menu"
              aria-expanded={speedMenuOpen}
              aria-label={`Playback speed ${rate} times`}
              onClick={() => setSpeedMenuOpen((o) => !o)}
            >
              {rate}×
            </button>
            {speedMenuOpen && (
              <div className="player-speed-menu" role="menu" aria-label="Playback speed">
                {SPEEDS.map((speed) => (
                  <button
                    key={speed}
                    type="button"
                    role="menuitemradio"
                    aria-checked={rate === speed}
                    className={`player-speed-option${rate === speed ? ' player-speed-option-active' : ''}`}
                    onClick={() => {
                      setRate(speed);
                      setSpeedMenuOpen(false);
                    }}
                  >
                    {speed}×
                  </button>
                ))}
              </div>
            )}
          </div>

          <button
            type="button"
            className="btn-icon player-btn"
            aria-label={isFullscreen ? 'Exit fullscreen' : 'Enter fullscreen'}
            onClick={toggleFullscreen}
          >
            {isFullscreen ? <FullscreenExitIcon /> : <FullscreenIcon />}
          </button>
        </div>
      </div>
    </div>
  );
}
