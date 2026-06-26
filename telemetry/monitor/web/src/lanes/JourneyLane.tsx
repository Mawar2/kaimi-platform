/**
 * JourneyLane renders the onboarding funnel: the per-step counts of
 * journey.onboarding.step.* events with the drop-off between consecutive steps,
 * plus the latest reported active-session count.
 *
 * Domain-agnostic: steps are discovered by name prefix from whatever journey
 * events arrive, in the order the store first saw them (which mirrors emission
 * order, i.e. funnel order).
 */

import { useEvents, useFunnel } from '../store/eventStore';
import type { FunnelStep } from '../store/eventStore';
import { attrNumber } from '../types/events';
import type { TelemetryEvent } from '../types/events';
import { formatCount, lastSegment } from '../format';

/** Prefix that identifies an onboarding funnel step event. */
const ONBOARDING_STEP_PREFIX = 'journey.onboarding.step.';

/** Attribute keys probed (newest event first) for a live active-session count. */
const ACTIVE_SESSION_KEYS = ['active_sessions', 'sessions_active'];

/** onboardingSteps keeps only funnel steps under the onboarding prefix. */
function onboardingSteps(funnel: readonly FunnelStep[]): FunnelStep[] {
  return funnel.filter((s) => s.name.startsWith(ONBOARDING_STEP_PREFIX));
}

/**
 * latestActiveSessions scans recent events (newest first) for an active-session
 * attribute, returning the first value found or undefined.
 */
function latestActiveSessions(
  events: readonly TelemetryEvent[],
): number | undefined {
  for (const ev of events) {
    for (const key of ACTIVE_SESSION_KEYS) {
      const v = attrNumber(ev, key);
      if (v !== undefined) {
        return v;
      }
    }
  }
  return undefined;
}

/** JourneySkeleton is the loading placeholder shown before the first event. */
function JourneySkeleton(): JSX.Element {
  return (
    <>
      {[0, 1, 2, 3].map((i) => (
        <div className="funnel-step" key={i}>
          <div className="skeleton skeleton-line" style={{ width: '70%' }} />
          <div className="skeleton funnel-bar" />
        </div>
      ))}
    </>
  );
}

/** JourneyLane is the lower-right lane rendering the onboarding funnel. */
export function JourneyLane({ loading }: { loading: boolean }): JSX.Element {
  const funnel = useFunnel();
  const events = useEvents();
  const steps = onboardingSteps(funnel);
  const sessions = latestActiveSessions(events);

  // The first step is the funnel's widest bar; later bars scale against it.
  const maxCount = steps.length > 0 ? steps[0].count : 0;

  return (
    <section className="lane lane-journey">
      <div className="lane-head">
        <span>Journey</span>
        <span className="sub">
          {sessions === undefined ? '—' : formatCount(sessions)} active
        </span>
      </div>
      <div className="lane-body">
        {loading ? (
          <JourneySkeleton />
        ) : steps.length === 0 ? (
          <div className="empty">No onboarding steps yet.</div>
        ) : (
          steps.map((step, i) => {
            const prev = i > 0 ? steps[i - 1].count : undefined;
            const dropoff =
              prev && prev > 0 ? (prev - step.count) / prev : undefined;
            const width = maxCount > 0 ? (step.count / maxCount) * 100 : 0;
            return (
              <div className="funnel-step" key={step.name}>
                <div className="funnel-row">
                  <span className="nm">{lastSegment(step.name)}</span>
                  <span className="ct">
                    {formatCount(step.count)}
                    {dropoff !== undefined && dropoff > 0 ? (
                      <span className="dropoff">
                        -{Math.round(dropoff * 100)}%
                      </span>
                    ) : null}
                  </span>
                </div>
                <div className="funnel-bar">
                  <div
                    className="funnel-bar-fill"
                    style={{ width: `${width}%` }}
                  />
                </div>
              </div>
            );
          })
        )}
      </div>
    </section>
  );
}
