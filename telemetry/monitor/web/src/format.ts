/**
 * Small, dependency-free formatting helpers shared by the Monitor lanes. They
 * turn raw numeric telemetry (durations in ms, token counts, rates) into compact
 * human-readable strings. Nothing here is host-specific; it only shapes numbers.
 */

/**
 * formatDuration renders a millisecond duration compactly: sub-second values as
 * "850ms", seconds as "1.2s", and minute-scale values as "1m 05s". Undefined or
 * non-finite input renders as an em dash so callers need no guard.
 */
export function formatDuration(ms: number | undefined): string {
  if (ms === undefined || !Number.isFinite(ms) || ms < 0) {
    return '—';
  }
  if (ms < 1000) {
    return `${Math.round(ms)}ms`;
  }
  const totalSeconds = ms / 1000;
  if (totalSeconds < 60) {
    return `${totalSeconds.toFixed(1)}s`;
  }
  const minutes = Math.floor(totalSeconds / 60);
  const seconds = Math.floor(totalSeconds % 60);
  return `${minutes}m ${seconds.toString().padStart(2, '0')}s`;
}

/**
 * formatCount renders an integer-ish count with thousands separators, or an em
 * dash when the value is absent.
 */
export function formatCount(n: number | undefined): string {
  if (n === undefined || !Number.isFinite(n)) {
    return '—';
  }
  return Math.round(n).toLocaleString();
}

/** formatRate renders a per-second rate to one decimal place (e.g. "12.3/s"). */
export function formatRate(perSec: number): string {
  return `${perSec.toFixed(1)}/s`;
}

/** formatPercent renders a 0..1 fraction as a whole-number percentage. */
export function formatPercent(fraction: number): string {
  return `${Math.round(fraction * 100)}%`;
}

/**
 * lastSegment returns the final dot-delimited segment of a name (e.g.
 * "proposal.outline" -> "outline"), used to label phases and steps concisely
 * without hard-coding any specific host name.
 */
export function lastSegment(name: string): string {
  const i = name.lastIndexOf('.');
  return i < 0 ? name : name.slice(i + 1);
}
