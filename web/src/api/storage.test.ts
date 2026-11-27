import { beforeEach, describe, expect, it } from 'vitest';
import {
  clearAuthStorage,
  clearStoredToken,
  getRememberMe,
  getStoredServerUrl,
  getStoredToken,
  normalizeServerUrl,
  setRememberMe,
  setStoredServerUrl,
  setStoredToken,
} from './storage';
import { loadVolumePrefs, saveVolumePrefs } from '../player/prefs';

beforeEach(() => {
  localStorage.clear();
});

describe('auth storage', () => {
  it('stores and retrieves the token', () => {
    expect(getStoredToken()).toBeNull();
    setStoredToken('abc123');
    expect(getStoredToken()).toBe('abc123');
  });

  it('clears the token', () => {
    setStoredToken('abc123');
    clearStoredToken();
    expect(getStoredToken()).toBeNull();
  });

  it('persists the server URL trimmed of trailing slashes', () => {
    setStoredServerUrl('  https://nas.example.com/  ');
    expect(getStoredServerUrl()).toBe('https://nas.example.com');
    setStoredServerUrl('https://nas.example.com///');
    expect(getStoredServerUrl()).toBe('https://nas.example.com');
  });

  it('remembers the remember-me flag', () => {
    expect(getRememberMe()).toBe(false);
    setRememberMe(true);
    expect(getRememberMe()).toBe(true);
    setRememberMe(false);
    expect(getRememberMe()).toBe(false);
  });

  it('clearAuthStorage removes token and remember-me but keeps the server URL', () => {
    setStoredToken('t');
    setRememberMe(true);
    setStoredServerUrl('https://nas.example.com');
    clearAuthStorage();
    expect(getStoredToken()).toBeNull();
    expect(getRememberMe()).toBe(false);
    // The server URL survives so the next login is prefilled.
    expect(getStoredServerUrl()).toBe('https://nas.example.com');
  });
});

describe('normalizeServerUrl', () => {
  it('trims whitespace and trailing slashes', () => {
    expect(normalizeServerUrl(' https://x.dev/ ')).toBe('https://x.dev');
    expect(normalizeServerUrl('http://localhost:8787')).toBe('http://localhost:8787');
    expect(normalizeServerUrl('')).toBe('');
  });
});

describe('volume prefs', () => {
  it('defaults to full volume, unmuted', () => {
    const prefs = loadVolumePrefs();
    expect(prefs).toEqual({ volume: 1, muted: false });
  });

  it('round-trips and clamps', () => {
    saveVolumePrefs({ volume: 0.4, muted: true });
    expect(loadVolumePrefs()).toEqual({ volume: 0.4, muted: true });
    saveVolumePrefs({ volume: 2, muted: false });
    expect(loadVolumePrefs().volume).toBe(1);
  });
});
