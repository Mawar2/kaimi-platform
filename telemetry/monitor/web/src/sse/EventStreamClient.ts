/**
 * EventStreamClient wraps the browser's native EventSource to consume the
 * telemetry Server-Sent Events stream (GET {apiBase}/events/stream).
 *
 * It adds three things the bare EventSource lacks:
 *   - Deterministic exponential backoff with jitter on reconnect (the native
 *     reconnector retries on a fixed server-advertised interval that we cannot
 *     tune, so we drive reconnection ourselves).
 *   - Resume across reconnects via Last-Event-ID. The native EventSource records
 *     the last `id:` it received on each message (`MessageEvent.lastEventId`);
 *     because the constructor cannot set request headers, we forward that value
 *     as the server-supported `last_event_id` query parameter when we recreate
 *     the connection, so the server replays exactly the events we missed.
 *   - Coarse status states (live | reconnecting | disconnected) and event-id
 *     deduplication, so an overlap in replayed history is dropped before it ever
 *     reaches the store.
 *
 * The server sets `event: <category>` on every frame, so frames are dispatched
 * as EventSource events named after their category ("journey"/"system"/
 * "proposal"/"llm") rather than the default "message". We therefore listen on
 * every category name (plus "message" as a defensive fallback).
 */

import { getConfig, streamUrl, type MonitorConfig } from '../config';
import type { Category, TelemetryEvent } from '../types/events';

/** ConnectionStatus is the coarse, observable state of the stream connection. */
export type ConnectionStatus = 'live' | 'reconnecting' | 'disconnected';

/** The fixed set of SSE event names the server emits (one per Category). */
const CATEGORY_EVENT_NAMES: readonly Category[] = [
  'journey',
  'system',
  'proposal',
  'llm',
];

/** EventStreamOptions configures an EventStreamClient. */
export interface EventStreamOptions {
  /** Called with each parsed, deduplicated event in arrival order. */
  onEvent: (event: TelemetryEvent) => void;
  /** Called whenever the connection status changes. */
  onStatus?: (status: ConnectionStatus) => void;
  /** Resolved config override; defaults to {@link getConfig}. */
  config?: MonitorConfig;
  /** Optional server-side category filter (`category` query param). */
  category?: Category;
  /** Optional server-side tenant filter (`tenant_id` query param). */
  tenantId?: string;
  /** First reconnect delay in ms (default 1000). */
  initialBackoffMs?: number;
  /** Maximum reconnect delay in ms (default 30000). */
  maxBackoffMs?: number;
  /** Multiplier applied to the delay after each failed attempt (default 2). */
  backoffFactor?: number;
  /**
   * How many recent event ids to remember for deduplication (default 5000).
   * Bounds memory while comfortably covering any replay overlap window.
   */
  dedupeCapacity?: number;
}

/**
 * EventStreamClient owns one logical connection to the SSE endpoint, including
 * its reconnection lifecycle. It is not safe for concurrent start/stop from
 * multiple call sites; drive it from a single owner (e.g. one React effect).
 */
export class EventStreamClient {
  private readonly onEvent: (event: TelemetryEvent) => void;
  private readonly onStatus?: (status: ConnectionStatus) => void;
  private readonly config: MonitorConfig;
  private readonly category?: Category;
  private readonly tenantId?: string;
  private readonly initialBackoffMs: number;
  private readonly maxBackoffMs: number;
  private readonly backoffFactor: number;
  private readonly dedupeCapacity: number;

  private source: EventSource | null = null;
  private status: ConnectionStatus = 'disconnected';
  private stopped = true;
  private reconnectTimer: ReturnType<typeof setTimeout> | null = null;
  private currentBackoffMs: number;

  /**
   * lastEventId is the most recent SSE `id:` (the server's monotonic Seq),
   * captured from the native EventSource and replayed as `last_event_id` so a
   * fresh connection resumes exactly where the previous one stopped.
   */
  private lastEventId: string | null = null;

  // Deduplication: a bounded FIFO of recently seen event ids. The Set gives O(1)
  // membership; the queue bounds memory by evicting the oldest ids.
  private readonly seenIds = new Set<string>();
  private readonly seenQueue: string[] = [];

  constructor(options: EventStreamOptions) {
    this.onEvent = options.onEvent;
    this.onStatus = options.onStatus;
    this.config = options.config ?? getConfig();
    this.category = options.category;
    this.tenantId = options.tenantId;
    this.initialBackoffMs = options.initialBackoffMs ?? 1000;
    this.maxBackoffMs = options.maxBackoffMs ?? 30000;
    this.backoffFactor = options.backoffFactor ?? 2;
    this.dedupeCapacity = options.dedupeCapacity ?? 5000;
    this.currentBackoffMs = this.initialBackoffMs;
  }

  /** start opens the connection. It is idempotent while already running. */
  start(): void {
    if (!this.stopped) {
      return;
    }
    this.stopped = false;
    // We are attempting to connect; surface "reconnecting" until the first open.
    this.setStatus('reconnecting');
    this.open();
  }

  /**
   * stop closes the connection, cancels any pending reconnect, and moves to the
   * disconnected state. The client may be restarted later with start().
   */
  stop(): void {
    this.stopped = true;
    this.clearReconnectTimer();
    this.closeSource();
    this.setStatus('disconnected');
  }

  /** open creates the EventSource and wires its handlers. */
  private open(): void {
    const url = streamUrl(this.config, {
      category: this.category,
      tenant_id: this.tenantId,
      // Forward the captured native Last-Event-ID so the server resumes from it.
      last_event_id: this.lastEventId ?? undefined,
    });

    const source = new EventSource(url);
    this.source = source;

    source.onopen = () => {
      // A successful open clears the backoff so the next failure starts small.
      this.currentBackoffMs = this.initialBackoffMs;
      this.setStatus('live');
    };

    const handler = (event: MessageEvent) => this.handleMessage(event);
    // Frames carry `event: <category>`, so listen per category name...
    for (const name of CATEGORY_EVENT_NAMES) {
      source.addEventListener(name, handler as EventListener);
    }
    // ...plus the default "message" type as a fallback for category-less frames.
    source.onmessage = handler;

    source.onerror = () => this.handleError();
  }

  /** handleMessage parses, deduplicates, and forwards one SSE frame. */
  private handleMessage(event: MessageEvent): void {
    // Record the resume cursor even for frames we end up dropping as duplicates.
    if (event.lastEventId) {
      this.lastEventId = event.lastEventId;
    }

    const parsed = this.parse(event.data);
    if (parsed === null) {
      return; // Skip malformed frames rather than tear down the stream.
    }
    if (this.isDuplicate(parsed.event_id)) {
      return;
    }
    this.remember(parsed.event_id);
    this.onEvent(parsed);
  }

  /**
   * parse turns a raw SSE data payload into a TelemetryEvent, returning null if
   * it is not valid JSON or is missing the required envelope fields.
   */
  private parse(data: unknown): TelemetryEvent | null {
    if (typeof data !== 'string' || data.length === 0) {
      return null;
    }
    let value: unknown;
    try {
      value = JSON.parse(data);
    } catch {
      return null;
    }
    if (!this.isTelemetryEvent(value)) {
      return null;
    }
    return value;
  }

  /**
   * isTelemetryEvent validates the minimal required envelope shape. It does not
   * deeply validate optional fields; the typed readers in types/events.ts narrow
   * those at the point of use.
   */
  private isTelemetryEvent(value: unknown): value is TelemetryEvent {
    if (typeof value !== 'object' || value === null) {
      return false;
    }
    const v = value as Record<string, unknown>;
    return (
      typeof v.event_id === 'string' &&
      typeof v.name === 'string' &&
      (v.category === 'journey' ||
        v.category === 'system' ||
        v.category === 'proposal' ||
        v.category === 'llm')
    );
  }

  /** handleError tears down the broken connection and schedules a reconnect. */
  private handleError(): void {
    if (this.stopped) {
      return; // A deliberate stop() closes the source; ignore the resulting error.
    }
    // Take over reconnection ourselves: close the native source (which would
    // otherwise retry on its own fixed schedule) and back off explicitly.
    this.closeSource();
    this.setStatus('reconnecting');
    this.scheduleReconnect();
  }

  /** scheduleReconnect arms a timer for the next attempt and grows the backoff. */
  private scheduleReconnect(): void {
    this.clearReconnectTimer();
    const delay = this.nextBackoffDelay();
    this.reconnectTimer = setTimeout(() => {
      this.reconnectTimer = null;
      if (this.stopped) {
        return;
      }
      this.open();
    }, delay);
  }

  /**
   * nextBackoffDelay returns the current delay with added jitter, then advances
   * the backoff geometrically up to the cap. Jitter spreads reconnect attempts
   * so many clients do not stampede a recovering server simultaneously.
   */
  private nextBackoffDelay(): number {
    const base = Math.min(this.currentBackoffMs, this.maxBackoffMs);
    this.currentBackoffMs = Math.min(
      this.currentBackoffMs * this.backoffFactor,
      this.maxBackoffMs,
    );
    // Full-ish jitter: add up to one base interval (capped at 1s of spread).
    const jitter = Math.random() * Math.min(base, 1000);
    return base + jitter;
  }

  private clearReconnectTimer(): void {
    if (this.reconnectTimer !== null) {
      clearTimeout(this.reconnectTimer);
      this.reconnectTimer = null;
    }
  }

  /** closeSource detaches handlers and closes the current EventSource, if any. */
  private closeSource(): void {
    if (this.source === null) {
      return;
    }
    this.source.onopen = null;
    this.source.onmessage = null;
    this.source.onerror = null;
    this.source.close();
    this.source = null;
  }

  private isDuplicate(eventId: string): boolean {
    return this.seenIds.has(eventId);
  }

  /** remember records an event id, evicting the oldest once over capacity. */
  private remember(eventId: string): void {
    this.seenIds.add(eventId);
    this.seenQueue.push(eventId);
    if (this.seenQueue.length > this.dedupeCapacity) {
      const evicted = this.seenQueue.shift();
      if (evicted !== undefined) {
        this.seenIds.delete(evicted);
      }
    }
  }

  private setStatus(status: ConnectionStatus): void {
    if (this.status === status) {
      return;
    }
    this.status = status;
    this.onStatus?.(status);
  }
}
