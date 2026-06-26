/**
 * TopStrip is the Monitor header: live throughput counters (events/sec, error
 * rate, in-flight proposals) plus the connection status, and the category /
 * tenant filter chips. Selecting a chip re-opens the SSE stream with new query
 * params — the actual stream lifecycle is owned by App, which passes the current
 * selection and change handlers down here.
 *
 * Domain-agnostic: the only fixed labels are the generic envelope categories;
 * tenant chips are discovered from the events that arrive.
 */

import {
  useConnectionStatus,
  useMonitorStats,
} from '../store/eventStore';
import type { Category } from '../types/events';
import { formatPercent, formatRate } from '../format';

/** The fixed envelope categories offered as filter chips. */
const CATEGORIES: readonly Category[] = ['journey', 'system', 'proposal', 'llm'];

/** TopStripProps wires the filter selection up to the stream owner (App). */
export interface TopStripProps {
  /** Currently selected category filter, or undefined for "all". */
  category?: Category;
  /** Currently selected tenant filter, or undefined for "all". */
  tenant?: string;
  /** Tenants discovered from the recent event stream, for the chip row. */
  tenants: readonly string[];
  /** Called when the category selection changes (undefined = all). */
  onCategory: (category: Category | undefined) => void;
  /** Called when the tenant selection changes (undefined = all). */
  onTenant: (tenant: string | undefined) => void;
}

/** Chip is a single selectable filter pill. */
function Chip({
  label,
  active,
  onClick,
}: {
  label: string;
  active: boolean;
  onClick: () => void;
}): JSX.Element {
  return (
    <button
      type="button"
      className={`chip ${active ? 'active' : ''}`}
      onClick={onClick}
    >
      {label}
    </button>
  );
}

/** TopStrip renders the header metrics and the filter chip rows. */
export function TopStrip({
  category,
  tenant,
  tenants,
  onCategory,
  onTenant,
}: TopStripProps): JSX.Element {
  const status = useConnectionStatus();
  const stats = useMonitorStats();

  return (
    <header className="topstrip">
      <span className="brand">
        <span className={`status-dot ${status}`} title={status} />
        Monitor
      </span>

      <div className="metric">
        <span className="value">{formatRate(stats.eventsPerSec)}</span>
        <span className="label">Events/sec</span>
      </div>
      <div className="metric">
        <span className={`value ${stats.errorRate > 0 ? 'err' : ''}`}>
          {formatPercent(stats.errorRate)}
        </span>
        <span className="label">Error rate</span>
      </div>
      <div className="metric">
        <span className="value">{stats.inFlightProposals}</span>
        <span className="label">In-flight</span>
      </div>

      <div className="filters">
        <div className="filter-group">
          <span className="grp-label">category</span>
          <Chip
            label="all"
            active={category === undefined}
            onClick={() => onCategory(undefined)}
          />
          {CATEGORIES.map((c) => (
            <Chip
              key={c}
              label={c}
              active={category === c}
              onClick={() => onCategory(c)}
            />
          ))}
        </div>

        {tenants.length > 0 ? (
          <div className="filter-group">
            <span className="grp-label">tenant</span>
            <Chip
              label="all"
              active={tenant === undefined}
              onClick={() => onTenant(undefined)}
            />
            {tenants.map((t) => (
              <Chip
                key={t}
                label={t}
                active={tenant === t}
                onClick={() => onTenant(t)}
              />
            ))}
          </div>
        ) : null}
      </div>
    </header>
  );
}
