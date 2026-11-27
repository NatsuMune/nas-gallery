// Time formatting + playback-position resume logic for the video player.

/** Formats seconds as mm:ss (04:07) or h:mm:ss (1:02:03) when >= 1 hour. */
export function formatClock(totalSeconds: number): string {
  if (!Number.isFinite(totalSeconds) || totalSeconds < 0) totalSeconds = 0;
  const total = Math.floor(totalSeconds);
  const hours = Math.floor(total / 3600);
  const minutes = Math.floor((total % 3600) / 60);
  const seconds = total % 60;
  const mm = String(minutes).padStart(2, '0');
  const ss = String(seconds).padStart(2, '0');
  return hours > 0 ? `${hours}:${mm}:${ss}` : `${mm}:${ss}`;
}

/** Formats an item's durationMs for video tile badges; '' when unknown. */
export function formatDurationMs(durationMs: number | null | undefined): string {
  if (typeof durationMs !== 'number' || !Number.isFinite(durationMs) || durationMs <= 0) return '';
  return formatClock(durationMs / 1000);
}

// --- Resume-position persistence (localStorage, per item id) ---

const RESUME_PREFIX = 'nasgallery.resume.';
/** Positions below this are not worth resuming. */
export const MIN_RESUME_SECONDS = 10;
/** Watched this fraction of the duration -> stop resuming / clear storage. */
export const WATCHED_FRACTION = 0.95;

function keyFor(itemId: string): string {
  return RESUME_PREFIX + itemId;
}

function safeGet(key: string): string | null {
  try {
    return globalThis.localStorage?.getItem(key) ?? null;
  } catch {
    return null;
  }
}

function safeSet(key: string, value: string): void {
  try {
    globalThis.localStorage?.setItem(key, value);
  } catch {
    // storage unavailable — resume is best-effort
  }
}

function safeRemove(key: string): void {
  try {
    globalThis.localStorage?.removeItem(key);
  } catch {
    // ignore
  }
}

/** True when >= 95% of the (known) duration has been watched. */
export function isMostlyWatched(seconds: number, durationSeconds: number | null | undefined): boolean {
  if (!Number.isFinite(seconds) || seconds <= 0) return false;
  if (typeof durationSeconds !== 'number' || !Number.isFinite(durationSeconds) || durationSeconds <= 0) return false;
  return seconds / durationSeconds >= WATCHED_FRACTION;
}

export function saveResumePosition(itemId: string, seconds: number, durationSeconds?: number | null): void {
  if (!Number.isFinite(seconds) || seconds <= 0) return;
  if (isMostlyWatched(seconds, durationSeconds ?? null)) {
    clearResumePosition(itemId);
    return;
  }
  safeSet(keyFor(itemId), String(Math.floor(seconds)));
}

/**
 * Returns the stored resume position, or null when there is none worth
 * resuming: missing, tiny (< 10s), or already >= 95% watched.
 */
export function loadResumePosition(itemId: string, durationSeconds: number | null | undefined): number | null {
  const raw = safeGet(keyFor(itemId));
  if (raw === null) return null;
  const seconds = Number(raw);
  if (!Number.isFinite(seconds) || seconds < MIN_RESUME_SECONDS) return null;
  if (isMostlyWatched(seconds, durationSeconds ?? null)) return null;
  return seconds;
}

export function clearResumePosition(itemId: string): void {
  safeRemove(keyFor(itemId));
}
