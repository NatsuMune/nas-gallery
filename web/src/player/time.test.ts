import { beforeEach, describe, expect, it } from 'vitest';
import {
  clearResumePosition,
  formatClock,
  formatDurationMs,
  isMostlyWatched,
  loadResumePosition,
  saveResumePosition,
} from './time';

beforeEach(() => {
  localStorage.clear();
});

describe('formatClock', () => {
  it('formats seconds as mm:ss', () => {
    expect(formatClock(0)).toBe('00:00');
    expect(formatClock(4)).toBe('00:04');
    expect(formatClock(42)).toBe('00:42');
    expect(formatClock(247)).toBe('04:07');
    expect(formatClock(599)).toBe('09:59');
    expect(formatClock(600)).toBe('10:00');
  });

  it('switches to h:mm:ss at one hour', () => {
    expect(formatClock(3600)).toBe('1:00:00');
    expect(formatClock(3723)).toBe('1:02:03');
    expect(formatClock(40000)).toBe('11:06:40');
  });

  it('degrades gracefully on bad input', () => {
    expect(formatClock(Number.NaN)).toBe('00:00');
    expect(formatClock(-5)).toBe('00:00');
    expect(formatClock(Number.POSITIVE_INFINITY)).toBe('00:00');
  });
});

describe('formatDurationMs', () => {
  it('converts milliseconds', () => {
    expect(formatDurationMs(30400)).toBe('00:30');
    expect(formatDurationMs(66000)).toBe('01:06');
    expect(formatDurationMs(3723000)).toBe('1:02:03');
  });

  it('returns empty string for unknown durations', () => {
    expect(formatDurationMs(null)).toBe('');
    expect(formatDurationMs(undefined)).toBe('');
    expect(formatDurationMs(0)).toBe('');
    expect(formatDurationMs(Number.NaN)).toBe('');
  });
});

describe('resume position', () => {
  it('round-trips a saved position', () => {
    saveResumePosition('m_1', 95.8, 400);
    expect(loadResumePosition('m_1', 400)).toBe(95);
  });

  it('does not resume tiny positions', () => {
    saveResumePosition('m_2', 8, 400);
    expect(loadResumePosition('m_2', 400)).toBeNull();
  });

  it('returns null when nothing was saved', () => {
    expect(loadResumePosition('m_3', 400)).toBeNull();
  });

  it('clears storage when saving a mostly-watched position', () => {
    saveResumePosition('m_4', 395, 400);
    expect(localStorage.getItem('nasgallery.resume.m_4')).toBeNull();
    expect(loadResumePosition('m_4', 400)).toBeNull();
  });

  it('does not resume positions already >= 95% watched', () => {
    saveResumePosition('m_5', 300, 400);
    expect(loadResumePosition('m_5', 400)).toBe(300);
    // Duration now known to be smaller — the stored position is 95%+ through.
    expect(loadResumePosition('m_5', 310)).toBeNull();
  });

  it('isMostlyWatched matches the spec', () => {
    expect(isMostlyWatched(380, 400)).toBe(true);
    expect(isMostlyWatched(379, 400)).toBe(false);
    expect(isMostlyWatched(380, null)).toBe(false);
    expect(isMostlyWatched(0, 400)).toBe(false);
  });

  it('clearResumePosition removes the entry', () => {
    saveResumePosition('m_6', 100, 400);
    clearResumePosition('m_6');
    expect(loadResumePosition('m_6', 400)).toBeNull();
  });

  it('ignores invalid saves', () => {
    saveResumePosition('m_7', Number.NaN, 400);
    saveResumePosition('m_8', -3, 400);
    expect(localStorage.getItem('nasgallery.resume.m_7')).toBeNull();
    expect(localStorage.getItem('nasgallery.resume.m_8')).toBeNull();
  });
});
