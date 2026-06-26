import { useEffect, useMemo, useState } from 'react';
import { EventStreamClient } from './sse/EventStreamClient';
import {
  store,
  useConnectionStatus,
  useEvents,
  useMonitorStats,
} from './store/eventStore';
import type { Category } from './types/events';
import { TopStrip } from './components/TopStrip';
import { AgentTimeline } from './lanes/AgentTimeline';
import { SystemLane } from './lanes/SystemLane';
import { JourneyLane } from './lanes/JourneyLane';

/** Cap on how many tenant chips to surface, newest-discovered kept readable. */
const MAX_TENANT_CHIPS = 12;

/**
 * App is the Monitor root. It owns the single SSE connection and the active
 * filter selection, and lays out the header (TopStrip) plus three lanes: the
 * large Agent Timeline, and the System and Journey lanes stacked beside it.
 *
 * Filter changes re-open the stream with new query params by tearing down and
 * recreating the EventStreamClient (the effect below is keyed on the selection).
 * The Monitor is domain-agnostic: every label it renders comes from event data
 * or the generic envelope categories.
 */
export function App(): JSX.Element {
  // Active server-side filters. undefined means "no filter" (all).
  const [category, setCategory] = useState<Category | undefined>(undefined);
  const [tenant, setTenant] = useState<string | undefined>(undefined);

  // Own the stream connection. Recreated whenever the filter selection changes
  // so the server re-opens the stream scoped to the new params.
  useEffect(() => {
    const client = new EventStreamClient({
      onEvent: (event) => store.push(event),
      onStatus: (status) => store.setStatus(status),
      category,
      tenantId: tenant,
    });
    client.start();
    return () => client.stop();
  }, [category, tenant]);

  const status = useConnectionStatus();
  const stats = useMonitorStats();
  const events = useEvents();

  // Discover tenant ids from the recent stream for the filter chips.
  const tenants = useMemo(() => {
    const seen = new Set<string>();
    for (const ev of events) {
      if (ev.tenant_id) {
        seen.add(ev.tenant_id);
      }
    }
    return [...seen].sort().slice(0, MAX_TENANT_CHIPS);
  }, [events]);

  // Before the first event arrives we show skeletons; once data exists but the
  // connection is not live, we keep the last data on screen, dimmed.
  const hasData = stats.totalEvents > 0;
  const loading = !hasData && status !== 'disconnected';
  const stale = hasData && status !== 'live';

  return (
    <div className="monitor">
      {status !== 'live' ? (
        <div className="banner">
          <span className="dot" />
          {status === 'reconnecting'
            ? 'Reconnecting to the event stream…'
            : 'Disconnected from the event stream.'}
        </div>
      ) : null}

      <TopStrip
        category={category}
        tenant={tenant}
        tenants={tenants}
        onCategory={setCategory}
        onTenant={setTenant}
      />

      <div className={`lanes ${stale ? 'is-stale' : ''}`}>
        <AgentTimeline loading={loading} />
        <SystemLane loading={loading} />
        <JourneyLane loading={loading} />
      </div>
    </div>
  );
}
