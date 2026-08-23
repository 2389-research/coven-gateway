// ABOUTME: Unit tests for the shared formatTime helper.
// ABOUTME: Uses shape regexes so assertions hold in any local timezone.

import { describe, it, expect } from 'vitest';
import { formatTime } from './time.js';

describe('formatTime', () => {
  it('returns an em-dash for empty input', () => {
    expect(formatTime('')).toBe('—');
  });

  it('formats as "Mon DD HH:MM" without seconds by default', () => {
    expect(formatTime('2026-08-23T14:05:09Z')).toMatch(/^[A-Z][a-z]{2} \d{2} \d{2}:\d{2}$/);
  });

  it('includes seconds when requested', () => {
    expect(formatTime('2026-08-23T14:05:09Z', { seconds: true })).toMatch(
      /^[A-Z][a-z]{2} \d{2} \d{2}:\d{2}:\d{2}$/
    );
  });

  it('renders em-dash for null input', () => {
    expect(formatTime(null)).toBe('—');
  });
});
