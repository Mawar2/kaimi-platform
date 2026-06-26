/**
 * SystemLane surfaces the health of the running system: the overall request
 * (event) rate, a count of errors seen in the recent window, and a QuotaGauge
 * bound to the most recent system.* event that reports a remaining quota.
 *
 * Domain-agnostic: it reads generic rolling stats and looks for quota attributes
 * by name on whatever system events arrive — it never names a specific host.
 */

import {
  useEvents,
  useLatestQuota,
  useMonitorStats,
} from '../store/eventStore';
import { attrNumber } from '../types/events';
import type { TelemetryEvent } from '../types/events';
import { formatCount, formatRate } from '../format';

/** Attribute keys the gauge probes for, in priority order. */
const QUOTA_REMAINING_KEYS = ['quota_remaining', 'remaining'];
const QUOTA_LIMIT_KEYS = ['quota_limit', 'quota_total', 'limit'];

/** firstAttrNumber returns the first present numeric attr among candidate keys. */
function firstAttrNumber(
  ev: TelemetryEvent,
  keys: readonly string[],
): number | undefined {
  for (const key of keys) {
    const v = attrNumber(ev, key);
    if (v !== undefined) {
      return v;
    }
  }
  return undefined;
}

/** countErrors tallies error-level / *.failed events in the current buffer. */
function countErrors(events: readonly TelemetryEvent[]): number {
  let n = 0;
  for (const ev of events) {
    if (ev.level === 'error' || ev.name.endsWith('.failed')) {
      n += 1;
    }
  }
  return n;
}

/**
 * QuotaGauge renders the latest remaining quota. When a matching limit attribute
 * is present it draws a proportional bar with low/critical coloring; otherwise it
 * shows the raw remaining value without implying a proportion.
 */
function QuotaGauge({ event }: { event?: TelemetryEvent }): JSX.Element {
  if (!event) {
    return <div className="empty">No quota reported.</div>;
  }
  const remaining = firstAttrNumber(event, QUOTA_REMAINING_KEYS);
  if (remaining === undefined) {
    return <div className="empty">No quota reported.</div>;
  }
  const limit = firstAttrNumber(event, QUOTA_LIMIT_KEYS);
  const ratio =
    limit && limit > 0 ? Math.max(0, Math.min(1, remaining / limit)) : undefined;
  const pct = ratio === undefined ? 100 : ratio * 100;
  const level = ratio === undefined ? '' : ratio < 0.2 ? 'crit' : ratio < 0.5 ? 'low' : '';

  return (
    <div className="gauge">
      <div className="gauge-head">
        <span>Quota remaining</span>
        <span>
          {formatCount(remaining)}
          {limit ? ` / ${formatCount(limit)}` : ''}
        </span>
      </div>
      <div className="gauge-track">
        <div className={`gauge-fill ${level}`} style={{ width: `${pct}%` }} />
      </div>
    </div>
  );
}

/** SystemSkeleton is the loading placeholder shown before the first event. */
function SystemSkeleton(): JSX.Element {
  return (
    <>
      <div className="stat-grid">
        <div className="skeleton skeleton-block" />
        <div className="skeleton skeleton-block" />
      </div>
      <div className="skeleton skeleton-line" style={{ width: '60%' }} />
      <div className="skeleton skeleton-line" style={{ width: '100%' }} />
    </>
  );
}

/** SystemLane is the upper-right lane summarizing system health. */
export function SystemLane({ loading }: { loading: boolean }): JSX.Element {
  const stats = useMonitorStats();
  const events = useEvents();
  const quota = useLatestQuota();
  const errors = countErrors(events);

  return (
    <section className="lane lane-system">
      <div className="lane-head">
        <span>System</span>
      </div>
      <div className="lane-body">
        {loading ? (
          <SystemSkeleton />
        ) : (
          <>
            <div className="stat-grid">
              <div className="stat-box">
                <div className="value">{formatRate(stats.eventsPerSec)}</div>
                <div className="label">Request rate</div>
              </div>
              <div className="stat-box">
                <div className={`value ${errors > 0 ? 'err' : ''}`}>
                  {formatCount(errors)}
                </div>
                <div className="label">Errors</div>
              </div>
            </div>
            <QuotaGauge event={quota} />
          </>
        )}
      </div>
    </section>
  );
}
