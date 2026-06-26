/**
 * Runtime configuration for the Monitor.
 *
 * The bundle is embedded and served from an arbitrary mount point, so it cannot
 * bake in an API origin at build time. Instead it reads an optional global
 * `window.__MONITOR_CONFIG__` (which a host may inject before the bundle loads,
 * e.g. via a small inline <script>) and otherwise defaults to a same-origin
 * `/v1` API base. The Monitor stays domain-agnostic: nothing here names a host.
 */

/** MonitorConfig is the resolved runtime configuration. */
export interface MonitorConfig {
  /**
   * apiBase is the root path (or absolute origin) of the telemetry HTTP API,
   * without a trailing slash. The SSE endpoint is `${apiBase}/events/stream`.
   */
  apiBase: string;
}

declare global {
  interface Window {
    /**
     * __MONITOR_CONFIG__ is an optional, host-injected partial override read at
     * startup. Absent in the default same-origin deployment.
     */
    __MONITOR_CONFIG__?: Partial<MonitorConfig>;
  }
}

/** DEFAULT_API_BASE is the same-origin API root used when no override exists. */
const DEFAULT_API_BASE = '/v1';

/**
 * getConfig resolves the runtime configuration, layering any
 * `window.__MONITOR_CONFIG__` override over the defaults and normalizing the
 * API base so it never carries a trailing slash.
 */
export function getConfig(): MonitorConfig {
  const override =
    (typeof window !== 'undefined' && window.__MONITOR_CONFIG__) || {};
  const raw =
    typeof override.apiBase === 'string' && override.apiBase.length > 0
      ? override.apiBase
      : DEFAULT_API_BASE;
  // Trim trailing slashes so `${apiBase}/events/stream` never doubles up.
  const apiBase = raw.replace(/\/+$/, '');
  return { apiBase };
}

/**
 * streamUrl builds the absolute (or root-relative) SSE endpoint URL from the
 * resolved config and optional query parameters. Undefined/empty params are
 * omitted so the server sees only the filters that were actually requested.
 */
export function streamUrl(
  config: MonitorConfig,
  params: Record<string, string | undefined> = {},
): string {
  const base = `${config.apiBase}/events/stream`;
  const query = new URLSearchParams();
  for (const [key, value] of Object.entries(params)) {
    if (value !== undefined && value !== '') {
      query.set(key, value);
    }
  }
  const qs = query.toString();
  return qs ? `${base}?${qs}` : base;
}
