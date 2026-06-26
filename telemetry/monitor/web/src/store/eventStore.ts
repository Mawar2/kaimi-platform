/**
 * eventStore is the Monitor's vanilla external store, consumed from React via
 * useSyncExternalStore. It owns a bounded ring buffer of the most recent events
 * and a set of derived indices that are maintained *incrementally* as each event
 * arrives — never recomputed by scanning the whole buffer — so a high-rate
 * stream stays cheap.
 *
 * Derived indices:
 *   - timelinesByTrace: one Timeline per trace_id, assembling phase spans from
 *     `*.started` / `*.completed` (and `.failed` / `.needs_human`) pairs keyed by
 *     span_id, with llm.* spans nested under their phase via parent_span_id.
 *   - rolling stats: a sliding-window events/sec and error-rate, plus a live
 *     in-flight-proposal counter.
 *   - funnel: per-name counts of journey-category events, in first-seen order.
 *   - latestQuota: the most recent system event that reports a quota.
 *
 * React notifications are coalesced onto requestAnimationFrame: a burst of
 * pushes schedules at most one re-render per frame, so the stream cannot jank
 * the UI. Each flush publishes a fresh immutable snapshot; unchanged Timeline
 * objects keep their identity (copy-on-write per trace) so per-trace selectors
 * memoize correctly.
 */

import { useSyncExternalStore } from 'react';
import type { ConnectionStatus } from '../sse/EventStreamClient';
import {
  readLLMUsage,
  type Actor,
  type LLMEvent,
  type LLMUsage,
  type TelemetryEvent,
} from '../types/events';

/* ------------------------------------------------------------------------- *
 * Public derived types
 * ------------------------------------------------------------------------- */

/** SpanStatus is the lifecycle state of a phase span within a trace. */
export type SpanStatus = 'running' | 'completed' | 'failed' | 'needs_human';

/** LLMSpan is a single model call nested under a phase via parent_span_id. */
export interface LLMSpan {
  spanId: string;
  parentSpanId?: string;
  name: string;
  status: 'running' | 'completed';
  startedAt?: string;
  completedAt?: string;
  durationMs?: number;
  usage?: LLMUsage;
  /** True once an llm.fallback.triggered event was seen for this span's trace. */
  fallback: boolean;
}

/**
 * PhaseSpan is one work phase within a trace (e.g. the outline phase), assembled
 * from a `*.started` event and its matching terminal event sharing a span_id.
 */
export interface PhaseSpan {
  spanId: string;
  /** Base name with the lifecycle verb stripped (e.g. "proposal.outline"). */
  name: string;
  actor?: Actor;
  status: SpanStatus;
  startedAt?: string;
  completedAt?: string;
  durationMs?: number;
  /** Nested model calls, keyed by their own span_id. */
  llmSpans: ReadonlyMap<string, LLMSpan>;
}

/** Milestone is a point-in-time trace event that is not a span (e.g. selected). */
export interface Milestone {
  name: string;
  occurredAt: string;
  eventId: string;
  actor?: Actor;
}

/** Timeline is the assembled view of every event sharing one trace_id. */
export interface Timeline {
  traceId: string;
  phases: ReadonlyMap<string, PhaseSpan>;
  milestones: readonly Milestone[];
  firstSeen: string;
  lastSeen: string;
  /** 'submitted' once a proposal.submitted milestone is seen, else 'active'. */
  state: 'active' | 'submitted';
}

/** MonitorStats holds the rolling, whole-stream counters. */
export interface MonitorStats {
  /** Total events accepted since startup (not just those still in the buffer). */
  totalEvents: number;
  /** Events per second over the rolling window. */
  eventsPerSec: number;
  /** Fraction (0..1) of windowed events that were errors. */
  errorRate: number;
  /** Traces with a proposal lifecycle started but not yet submitted. */
  inFlightProposals: number;
}

/** FunnelStep is one journey step name with its observed count. */
export interface FunnelStep {
  name: string;
  count: number;
}

/** MonitorSnapshot is the immutable view published to React on each flush. */
export interface MonitorSnapshot {
  /** Monotonic version, bumped on every flush. */
  version: number;
  status: ConnectionStatus;
  /** Recent events, newest first, bounded by the ring-buffer capacity. */
  events: readonly TelemetryEvent[];
  stats: MonitorStats;
  timelines: ReadonlyMap<string, Timeline>;
  /** Trace ids ordered by most-recent activity first. */
  traceIds: readonly string[];
  /** Journey funnel steps in first-seen order. */
  funnel: readonly FunnelStep[];
  /** Most recent system event reporting a quota, if any. */
  latestQuota?: TelemetryEvent;
}

/* ------------------------------------------------------------------------- *
 * Tuning constants
 * ------------------------------------------------------------------------- */

/** Maximum events retained in the ring buffer. */
const RING_CAPACITY = 2000;
/** Sliding window (ms) over which events/sec and error-rate are computed. */
const ROLLING_WINDOW_MS = 5000;
/**
 * Soft cap on retained timelines. When exceeded, the least-recently-active
 * traces are evicted so a long-lived session cannot grow the index without
 * bound. Generous relative to realistic concurrency.
 */
const MAX_TIMELINES = 256;

/** Lifecycle verbs that open/close a phase span. */
const VERB_STARTED = 'started';
const TERMINAL_VERBS: Readonly<Record<string, SpanStatus>> = {
  completed: 'completed',
  failed: 'failed',
  needs_human: 'needs_human',
};

/* ------------------------------------------------------------------------- *
 * Internal mutable bookkeeping (not exposed in the snapshot)
 * ------------------------------------------------------------------------- */

/** arrival records one event's wall-clock arrival time and error flag. */
interface arrival {
  t: number;
  error: boolean;
}

/** splitName breaks an event name into its base and trailing lifecycle verb. */
function splitName(name: string): { base: string; verb: string } {
  const i = name.lastIndexOf('.');
  if (i < 0) {
    return { base: name, verb: '' };
  }
  return { base: name.slice(0, i), verb: name.slice(i + 1) };
}

/** actorOf returns the event's actor when populated, else undefined. */
function actorOf(ev: TelemetryEvent): Actor | undefined {
  const a = ev.actor;
  if (a && (a.kind || a.name || a.id)) {
    return a;
  }
  return undefined;
}

/** hasQuotaAttr reports whether a system event carries a quota-ish attribute. */
function isQuotaEvent(ev: TelemetryEvent): boolean {
  if (ev.category !== 'system') {
    return false;
  }
  if (/quota/i.test(ev.name)) {
    return true;
  }
  return (ev.attributes ?? []).some((a) => /quota/i.test(a.key));
}

/**
 * EventStore is the concrete external store. A single module-level instance is
 * exported below; the class is exported too so tests can build isolated stores.
 */
export class EventStore {
  // Ring buffer, ordered oldest -> newest.
  private readonly buffer: TelemetryEvent[] = [];

  // Derived indices.
  private readonly timelines = new Map<string, Timeline>();
  private readonly funnel = new Map<string, number>();
  private latestQuota: TelemetryEvent | undefined;

  // In-flight proposal accounting.
  private readonly proposalTraces = new Set<string>();
  private readonly submittedTraces = new Set<string>();

  // Rolling-metric window.
  private readonly arrivals: arrival[] = [];
  private totalEvents = 0;

  // Defensive dedupe (the client already dedupes; this keeps push idempotent).
  private readonly seenIds = new Set<string>();
  private readonly seenQueue: string[] = [];

  private status: ConnectionStatus = 'disconnected';

  // Snapshot + subscription machinery.
  private snapshot: MonitorSnapshot;
  private readonly listeners = new Set<() => void>();
  private rafHandle: number | null = null;

  // Bound so they can be passed directly to useSyncExternalStore.
  readonly subscribe = (listener: () => void): (() => void) => {
    this.listeners.add(listener);
    return () => {
      this.listeners.delete(listener);
    };
  };
  readonly getSnapshot = (): MonitorSnapshot => this.snapshot;

  constructor() {
    this.snapshot = this.buildSnapshot();
  }

  /** setStatus records the connection status and schedules a flush. */
  setStatus(status: ConnectionStatus): void {
    if (this.status === status) {
      return;
    }
    this.status = status;
    this.scheduleFlush();
  }

  /**
   * push ingests one event: it appends to the ring buffer, updates every derived
   * index incrementally, and schedules a coalesced flush. Duplicate event ids
   * are ignored so the call is idempotent.
   */
  push(ev: TelemetryEvent): void {
    if (this.seenIds.has(ev.event_id)) {
      return;
    }
    this.remember(ev.event_id);

    this.appendToBuffer(ev);
    this.totalEvents += 1;
    this.recordArrival(ev);

    if (ev.category === 'journey') {
      this.funnel.set(ev.name, (this.funnel.get(ev.name) ?? 0) + 1);
    }
    if (isQuotaEvent(ev)) {
      this.updateLatestQuota(ev);
    }
    if (ev.trace_id) {
      this.updateTimeline(ev.trace_id, ev);
    }

    this.scheduleFlush();
  }

  /* --------------------------- ring buffer ---------------------------- */

  private appendToBuffer(ev: TelemetryEvent): void {
    this.buffer.push(ev);
    if (this.buffer.length > RING_CAPACITY) {
      // Evict the oldest. Derived indices are intentionally not rewound on
      // eviction: rolling stats age out by time, and timelines are bounded
      // separately by MAX_TIMELINES.
      this.buffer.shift();
    }
  }

  private remember(eventId: string): void {
    this.seenIds.add(eventId);
    this.seenQueue.push(eventId);
    // Keep the dedupe set comfortably larger than the buffer so re-delivered
    // events are still recognized after they have scrolled out of the buffer.
    if (this.seenQueue.length > RING_CAPACITY * 2) {
      const evicted = this.seenQueue.shift();
      if (evicted !== undefined) {
        this.seenIds.delete(evicted);
      }
    }
  }

  /* ---------------------------- rolling stats ------------------------- */

  private recordArrival(ev: TelemetryEvent): void {
    const error = ev.level === 'error' || ev.name.endsWith('.failed');
    this.arrivals.push({ t: Date.now(), error });
    this.pruneArrivals(Date.now());
  }

  private pruneArrivals(now: number): void {
    const cutoff = now - ROLLING_WINDOW_MS;
    // Arrivals are appended in time order, so drop from the front until current.
    let drop = 0;
    while (drop < this.arrivals.length && this.arrivals[drop].t < cutoff) {
      drop += 1;
    }
    if (drop > 0) {
      this.arrivals.splice(0, drop);
    }
  }

  private computeStats(now: number): MonitorStats {
    this.pruneArrivals(now);
    const windowed = this.arrivals.length;
    const errors = this.arrivals.reduce((n, a) => (a.error ? n + 1 : n), 0);
    const eventsPerSec = windowed / (ROLLING_WINDOW_MS / 1000);
    const errorRate = windowed === 0 ? 0 : errors / windowed;
    let inFlight = 0;
    for (const trace of this.proposalTraces) {
      if (!this.submittedTraces.has(trace)) {
        inFlight += 1;
      }
    }
    return {
      totalEvents: this.totalEvents,
      eventsPerSec,
      errorRate,
      inFlightProposals: inFlight,
    };
  }

  private updateLatestQuota(ev: TelemetryEvent): void {
    // Events generally arrive in order; guard against an out-of-order replay by
    // keeping whichever has the later occurred_at timestamp.
    if (
      this.latestQuota === undefined ||
      ev.occurred_at >= this.latestQuota.occurred_at
    ) {
      this.latestQuota = ev;
    }
  }

  /* ----------------------------- timelines ---------------------------- */

  /**
   * updateTimeline applies one event to its trace's Timeline using copy-on-write
   * so the resulting Timeline (and any phase it touches) gets a fresh identity,
   * leaving sibling traces untouched.
   */
  private updateTimeline(traceId: string, ev: TelemetryEvent): void {
    const prev = this.timelines.get(traceId);
    const base: Timeline = prev ?? {
      traceId,
      phases: new Map<string, PhaseSpan>(),
      milestones: [],
      firstSeen: ev.occurred_at,
      lastSeen: ev.occurred_at,
      state: 'active',
    };

    // Track proposal lifecycle membership for the in-flight counter.
    if (ev.category === 'proposal') {
      this.proposalTraces.add(traceId);
    }

    let next: Timeline = {
      ...base,
      lastSeen: ev.occurred_at,
    };

    if (ev.category === 'llm' && ev.span_id) {
      next = this.applyLLMEvent(next, ev);
    } else if (ev.span_id && this.isPhaseEvent(ev.name)) {
      next = this.applyPhaseEvent(next, ev);
    } else {
      next = this.applyMilestone(next, ev);
    }

    if (ev.name === 'proposal.submitted') {
      this.submittedTraces.add(traceId);
      next = { ...next, state: 'submitted' };
    }

    this.timelines.set(traceId, next);
    this.evictTimelinesIfNeeded();
  }

  private isPhaseEvent(name: string): boolean {
    const { verb } = splitName(name);
    return verb === VERB_STARTED || verb in TERMINAL_VERBS;
  }

  /** applyPhaseEvent opens or closes a phase span keyed by span_id. */
  private applyPhaseEvent(t: Timeline, ev: TelemetryEvent): Timeline {
    const spanId = ev.span_id as string;
    const { base, verb } = splitName(ev.name);
    const phases = new Map(t.phases);
    const existing = phases.get(spanId);

    if (verb === VERB_STARTED) {
      const phase: PhaseSpan = {
        spanId,
        name: base,
        actor: actorOf(ev) ?? existing?.actor,
        status: 'running',
        startedAt: ev.occurred_at,
        // Preserve any terminal data that raced ahead of the start frame.
        completedAt: existing?.completedAt,
        durationMs: existing?.durationMs,
        // Attach any llm spans that arrived before their parent phase started.
        llmSpans: existing?.llmSpans ?? new Map<string, LLMSpan>(),
      };
      phases.set(spanId, phase);
      return { ...t, phases };
    }

    // Terminal verb: complete/fail/needs_human the phase, creating a stub if the
    // start frame has not been seen yet.
    const status = TERMINAL_VERBS[verb];
    const startedAt = existing?.startedAt;
    const durationMs =
      ev.duration_ms ??
      (startedAt ? Date.parse(ev.occurred_at) - Date.parse(startedAt) : undefined);
    const phase: PhaseSpan = {
      spanId,
      name: existing?.name ?? base,
      actor: actorOf(ev) ?? existing?.actor,
      status,
      startedAt,
      completedAt: ev.occurred_at,
      durationMs: Number.isFinite(durationMs) ? durationMs : undefined,
      llmSpans: existing?.llmSpans ?? new Map<string, LLMSpan>(),
    };
    phases.set(spanId, phase);
    return { ...t, phases };
  }

  /**
   * applyLLMEvent nests a model call under its parent phase (parent_span_id). If
   * the parent phase has not been seen yet, the llm span is parked on a synthetic
   * phase keyed by the parent span id so it is not lost; a later phase `.started`
   * for that span id adopts the parked spans.
   */
  private applyLLMEvent(t: Timeline, ev: LLMEvent): Timeline {
    const spanId = ev.span_id as string;
    const parentId = ev.parent_span_id ?? spanId;
    const { base, verb } = splitName(ev.name);

    if (ev.name === 'llm.fallback.triggered') {
      return this.applyFallback(t, parentId);
    }

    const phases = new Map(t.phases);
    const parent =
      phases.get(parentId) ??
      ({
        spanId: parentId,
        name: '(pending)',
        status: 'running',
        llmSpans: new Map<string, LLMSpan>(),
      } as PhaseSpan);

    const llmSpans = new Map(parent.llmSpans);
    const existing = llmSpans.get(spanId);

    if (verb === VERB_STARTED) {
      llmSpans.set(spanId, {
        spanId,
        parentSpanId: ev.parent_span_id,
        name: base,
        status: 'running',
        startedAt: ev.occurred_at,
        completedAt: existing?.completedAt,
        durationMs: existing?.durationMs,
        usage: existing?.usage,
        fallback: existing?.fallback ?? false,
      });
    } else {
      // Treat any non-started llm event as a completion carrying usage.
      const startedAt = existing?.startedAt;
      const durationMs =
        ev.duration_ms ??
        (startedAt
          ? Date.parse(ev.occurred_at) - Date.parse(startedAt)
          : undefined);
      llmSpans.set(spanId, {
        spanId,
        parentSpanId: ev.parent_span_id,
        name: existing?.name ?? base,
        status: 'completed',
        startedAt,
        completedAt: ev.occurred_at,
        durationMs: Number.isFinite(durationMs) ? durationMs : undefined,
        usage: readLLMUsage(ev),
        fallback: existing?.fallback ?? false,
      });
    }

    phases.set(parentId, { ...parent, llmSpans });
    return { ...t, phases };
  }

  /** applyFallback flags every llm span under a parent as having fallen back. */
  private applyFallback(t: Timeline, parentId: string): Timeline {
    const parent = t.phases.get(parentId);
    if (!parent) {
      return t;
    }
    const phases = new Map(t.phases);
    const llmSpans = new Map(parent.llmSpans);
    for (const [id, span] of llmSpans) {
      llmSpans.set(id, { ...span, fallback: true });
    }
    phases.set(parentId, { ...parent, llmSpans });
    return { ...t, phases };
  }

  /** applyMilestone appends a non-span trace event (selected, gate, etc.). */
  private applyMilestone(t: Timeline, ev: TelemetryEvent): Timeline {
    const milestone: Milestone = {
      name: ev.name,
      occurredAt: ev.occurred_at,
      eventId: ev.event_id,
      actor: actorOf(ev),
    };
    return { ...t, milestones: [...t.milestones, milestone] };
  }

  /** evictTimelinesIfNeeded drops least-recently-active traces over the cap. */
  private evictTimelinesIfNeeded(): void {
    if (this.timelines.size <= MAX_TIMELINES) {
      return;
    }
    // Find the oldest by lastSeen (RFC3339 UTC sorts lexicographically).
    let oldestKey: string | undefined;
    let oldestSeen: string | undefined;
    for (const [key, t] of this.timelines) {
      if (oldestSeen === undefined || t.lastSeen < oldestSeen) {
        oldestSeen = t.lastSeen;
        oldestKey = key;
      }
    }
    if (oldestKey !== undefined) {
      this.timelines.delete(oldestKey);
      this.proposalTraces.delete(oldestKey);
      this.submittedTraces.delete(oldestKey);
    }
  }

  /* ---------------------------- flush / snapshot ---------------------- */

  /**
   * scheduleFlush coalesces notifications onto the next animation frame so a
   * burst of pushes produces at most one snapshot+render per frame.
   */
  private scheduleFlush(): void {
    if (this.rafHandle !== null) {
      return;
    }
    if (typeof requestAnimationFrame === 'function') {
      this.rafHandle = requestAnimationFrame(() => {
        this.rafHandle = null;
        this.flush();
      });
    } else {
      // Non-browser fallback (e.g. SSR/test): approximate a frame with a timer.
      this.rafHandle = setTimeout(() => {
        this.rafHandle = null;
        this.flush();
      }, 16) as unknown as number;
    }
  }

  private flush(): void {
    this.snapshot = this.buildSnapshot();
    for (const listener of this.listeners) {
      listener();
    }
  }

  /** buildSnapshot assembles a fresh immutable snapshot from current state. */
  private buildSnapshot(): MonitorSnapshot {
    const now = Date.now();
    // Newest-first copy of the buffer for display convenience.
    const events = this.buffer.slice().reverse();

    // Fresh Map reference (so the snapshot's `timelines` identity changes), while
    // the per-trace Timeline objects keep identity via copy-on-write in push().
    const timelines = new Map(this.timelines);

    // Trace ids ordered by most-recent activity first.
    const traceIds = [...timelines.values()]
      .sort((a, b) => (a.lastSeen < b.lastSeen ? 1 : a.lastSeen > b.lastSeen ? -1 : 0))
      .map((t) => t.traceId);

    const funnel: FunnelStep[] = [...this.funnel.entries()].map(
      ([name, count]) => ({ name, count }),
    );

    return {
      version: (this.snapshot?.version ?? 0) + 1,
      status: this.status,
      events,
      stats: this.computeStats(now),
      timelines,
      traceIds,
      funnel,
      latestQuota: this.latestQuota,
    };
  }
}

/** store is the shared Monitor store instance the hooks read from. */
export const store = new EventStore();

/* ------------------------------------------------------------------------- *
 * Selector hooks
 *
 * Each hook returns a referentially stable value between flushes, because every
 * snapshot field is pre-assembled once per flush. That keeps useSyncExternalStore
 * from looping and lets components memoize on the returned reference.
 * ------------------------------------------------------------------------- */

function useSelector<T>(select: (s: MonitorSnapshot) => T): T {
  return useSyncExternalStore(
    store.subscribe,
    () => select(store.getSnapshot()),
    () => select(store.getSnapshot()),
  );
}

/** useConnectionStatus returns the current SSE connection status. */
export function useConnectionStatus(): ConnectionStatus {
  return useSelector((s) => s.status);
}

/** useMonitorStats returns the rolling whole-stream counters. */
export function useMonitorStats(): MonitorStats {
  return useSelector((s) => s.stats);
}

/** useEvents returns the recent events, newest first. */
export function useEvents(): readonly TelemetryEvent[] {
  return useSelector((s) => s.events);
}

/** useTimelines returns the per-trace timeline map. */
export function useTimelines(): ReadonlyMap<string, Timeline> {
  return useSelector((s) => s.timelines);
}

/** useTraceIds returns trace ids ordered by most-recent activity first. */
export function useTraceIds(): readonly string[] {
  return useSelector((s) => s.traceIds);
}

/**
 * useTimeline returns the timeline for one trace, or undefined. The result keeps
 * its identity across flushes when that trace did not change (copy-on-write), so
 * a subscribing component only re-renders when its own trace updates.
 */
export function useTimeline(traceId: string): Timeline | undefined {
  return useSelector((s) => s.timelines.get(traceId));
}

/** useFunnel returns the journey funnel steps in first-seen order. */
export function useFunnel(): readonly FunnelStep[] {
  return useSelector((s) => s.funnel);
}

/** useLatestQuota returns the most recent system quota event, if any. */
export function useLatestQuota(): TelemetryEvent | undefined {
  return useSelector((s) => s.latestQuota);
}
