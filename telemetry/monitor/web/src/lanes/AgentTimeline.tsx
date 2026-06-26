/**
 * AgentTimeline is the Monitor's centerpiece lane: one swimlane per trace_id
 * (newest / in-flight first), each showing the phase tracks assembled by the
 * store from *.started / *.completed pairs. Every phase shows its actor and
 * duration (live-elapsed while still running), and nests its llm.* child spans
 * (matched by parent_span_id) with token usage and a warning badge when a model
 * call was truncated.
 *
 * The lane is domain-agnostic: phase names, actor names, and trace ids are all
 * rendered straight from event data — nothing about any specific host is baked
 * in here.
 */

import { memo, useEffect, useState } from 'react';
import {
  useTimelines,
  useTraceIds,
  type LLMSpan,
  type PhaseSpan,
  type Timeline,
} from '../store/eventStore';
import { formatCount, formatDuration, lastSegment } from '../format';

/**
 * useNow returns a wall-clock timestamp that advances on a fixed interval,
 * forcing a re-render so live-elapsed durations on running phases keep ticking.
 * One second is plenty granular for human-readable elapsed times.
 */
function useNow(intervalMs = 1000): number {
  const [now, setNow] = useState(() => Date.now());
  useEffect(() => {
    const id = setInterval(() => setNow(Date.now()), intervalMs);
    return () => clearInterval(id);
  }, [intervalMs]);
  return now;
}

/** phaseDuration picks the recorded duration, or the live elapsed if running. */
function phaseDuration(phase: PhaseSpan, now: number): number | undefined {
  if (phase.status === 'running' && phase.startedAt) {
    return now - Date.parse(phase.startedAt);
  }
  return phase.durationMs;
}

/** startKey sorts spans by start time, leaving unknown-start spans at the end. */
function startKey(startedAt?: string): number {
  return startedAt ? Date.parse(startedAt) : Number.POSITIVE_INFINITY;
}

/** LLMSpanRow renders one nested model call under a phase. */
function LLMSpanRow({ span }: { span: LLMSpan }): JSX.Element {
  const usage = span.usage;
  return (
    <div className="llm-span">
      <span className="nm">{lastSegment(span.name)}</span>
      <span className="tokens">
        in {formatCount(usage?.input_tokens)} / out{' '}
        {formatCount(usage?.output_tokens)}
      </span>
      {usage?.truncated ? <span className="badge warn">truncated</span> : null}
      {span.fallback ? <span className="badge fallback">fallback</span> : null}
    </div>
  );
}

/** PhaseTrack renders one phase span and its nested llm child spans. */
function PhaseTrack({
  phase,
  now,
}: {
  phase: PhaseSpan;
  now: number;
}): JSX.Element {
  const llmSpans = [...phase.llmSpans.values()].sort(
    (a, b) => startKey(a.startedAt) - startKey(b.startedAt),
  );
  return (
    <div className={`phase ${phase.status}`}>
      <div className="phase-row">
        <span className="phase-name">{lastSegment(phase.name)}</span>
        {phase.actor?.name ? (
          <span className="phase-actor">{phase.actor.name}</span>
        ) : null}
        <span className="phase-status">{phase.status.replace('_', ' ')}</span>
        <span className="phase-dur">
          {formatDuration(phaseDuration(phase, now))}
        </span>
      </div>
      {llmSpans.length > 0 ? (
        <div className="llm-spans">
          {llmSpans.map((s) => (
            <LLMSpanRow key={s.spanId} span={s} />
          ))}
        </div>
      ) : null}
    </div>
  );
}

/**
 * Swimlane renders a single trace. It is memoized on the Timeline reference and
 * the ticking `now`, so it only re-renders when its own trace changes (the store
 * gives each touched trace a fresh identity via copy-on-write) or once per tick
 * to advance any running-phase elapsed timer.
 */
const Swimlane = memo(function Swimlane({
  timeline,
  now,
}: {
  timeline: Timeline;
  now: number;
}): JSX.Element {
  const phases = [...timeline.phases.values()].sort(
    (a, b) => startKey(a.startedAt) - startKey(b.startedAt),
  );
  return (
    <div className="swimlane">
      <div className="swimlane-head">
        <span className="trace" title={timeline.traceId}>
          {timeline.traceId}
        </span>
        <span className={`state-tag ${timeline.state}`}>{timeline.state}</span>
      </div>
      {phases.length > 0 ? (
        <div className="phases">
          {phases.map((p) => (
            <PhaseTrack key={p.spanId} phase={p} now={now} />
          ))}
        </div>
      ) : null}
      {timeline.milestones.length > 0 ? (
        <div className="milestones">
          {timeline.milestones.map((m) => (
            <span className="milestone" key={m.eventId} title={m.occurredAt}>
              {lastSegment(m.name)}
              {m.actor?.name ? ` · ${m.actor.name}` : ''}
            </span>
          ))}
        </div>
      ) : null}
    </div>
  );
});

/** TimelineSkeleton is the loading placeholder shown before the first event. */
function TimelineSkeleton(): JSX.Element {
  return (
    <>
      {[0, 1, 2].map((i) => (
        <div className="swimlane" key={i}>
          <div className="swimlane-head">
            <div className="skeleton skeleton-line" style={{ width: '40%' }} />
          </div>
          <div className="phases">
            <div className="skeleton skeleton-block" />
          </div>
        </div>
      ))}
    </>
  );
}

/** AgentTimeline is the large lane rendering every active proposal trace. */
export function AgentTimeline({ loading }: { loading: boolean }): JSX.Element {
  const traceIds = useTraceIds();
  const timelines = useTimelines();
  const now = useNow(1000);

  return (
    <section className="lane lane-timeline">
      <div className="lane-head">
        <span>Agent Timeline</span>
        <span className="sub">{traceIds.length} traces</span>
      </div>
      <div className="lane-body">
        {loading ? (
          <TimelineSkeleton />
        ) : traceIds.length === 0 ? (
          <div className="empty">No proposal traces yet.</div>
        ) : (
          traceIds.map((id) => {
            const timeline = timelines.get(id);
            return timeline ? (
              <Swimlane key={id} timeline={timeline} now={now} />
            ) : null;
          })
        )}
      </div>
    </section>
  );
}
