// Persisted player preferences (volume / mute), shared across sessions.

const VOLUME_KEY = 'nasgallery.volume';

export interface VolumePrefs {
  volume: number;
  muted: boolean;
}

export function loadVolumePrefs(): VolumePrefs {
  try {
    const raw = globalThis.localStorage?.getItem(VOLUME_KEY);
    if (raw) {
      const parsed: unknown = JSON.parse(raw);
      if (parsed && typeof parsed === 'object') {
        const volume = (parsed as { volume?: unknown }).volume;
        const muted = (parsed as { muted?: unknown }).muted;
        return {
          volume: typeof volume === 'number' && Number.isFinite(volume) ? Math.min(Math.max(volume, 0), 1) : 1,
          muted: muted === true,
        };
      }
    }
  } catch {
    // fall through to defaults
  }
  return { volume: 1, muted: false };
}

export function saveVolumePrefs(prefs: VolumePrefs): void {
  try {
    globalThis.localStorage?.setItem(VOLUME_KEY, JSON.stringify(prefs));
  } catch {
    // best effort
  }
}
