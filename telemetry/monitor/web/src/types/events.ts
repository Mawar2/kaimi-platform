/**
 * Telemetry event envelope and per-category helpers for the Monitor.
 *
 * These shapes mirror the wire format emitted by the kaimi-telemetry core
 * (telemetry/event/event.go) exactly. The Monitor is domain-agnostic: every
 * field below is generic, and the only host-specific knowledge encoded here is
 * the set of *opaque* event names the core's own hosts happen to emit (kept as
 * open string-literal unions so unknown names still type-check).
 *
 * Naming note: fields are snake_case because that is the JSON the server sends;
 * we bind to the wire shape rather than re-casing it.
 */

/**
 * AttrClass tags whether an attribute value is safe to forward (usage) or must
 * stay inside the deployment (content). Numeric values are stable: 0=usage,
 * 1=content — matching event.Class in the Go core.
 */
export type AttrClass = 0 | 1;

/** Convenience aliases for the two attribute classes. */
export const AttrClassUsage: AttrClass = 0;
export const AttrClassContent: AttrClass = 1;

/**
 * Attr is one class-tagged key/value pair on an event. `value` is the single
 * place an arbitrary JSON value crosses the type boundary, so it is typed
 * `any` deliberately; callers narrow it through the typed readers below.
 */
export interface Attr {
  key: string;
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  value: any;
  class: AttrClass;
}

/**
 * Actor identifies who or what produced an event. The whole object is optional
 * on an event; when present, `kind` and `name` are populated by the core.
 */
export interface Actor {
  kind: string;
  id?: string;
  name: string;
}

/** Category is the coarse, domain-agnostic grouping the envelope discriminates on. */
export type Category = 'journey' | 'system' | 'proposal' | 'llm';

/**
 * BaseEvent holds the fields common to every category. The required fields
 * (event_id, schema_version, occurred_at, category, name) are always present;
 * the rest are omitted from the JSON when empty.
 */
interface BaseEvent {
  event_id: string;
  schema_version: number;
  /** RFC3339 timestamp (UTC). */
  occurred_at: string;
  tenant_id: string;
  name: string;
  level?: string;
  actor?: Actor;
  trace_id?: string;
  span_id?: string;
  parent_span_id?: string;
  duration_ms?: number;
  attributes?: Attr[];
}

/**
 * OpenLiteral keeps a string-literal union open: known members provide
 * autocomplete while any other string still assigns. Used for opaque,
 * host-supplied event names the Monitor must not reject.
 */
// eslint-disable-next-line @typescript-eslint/ban-types
type OpenLiteral<T extends string> = T | (string & {});

/** Known journey-category event. Names are fully host-defined. */
export interface JourneyEvent extends BaseEvent {
  category: 'journey';
}

/** Known system-category event (boot, shutdown, scheduled-run lifecycle). */
export interface SystemEvent extends BaseEvent {
  category: 'system';
}

/**
 * Proposal event names emitted by the core's reference hosts. The list is the
 * known set; the type stays open because names are opaque to the core.
 */
export type ProposalEventName = OpenLiteral<
  | 'proposal.selected'
  | 'proposal.outline.started'
  | 'proposal.outline.completed'
  | 'proposal.outline.failed'
  | 'proposal.writer.started'
  | 'proposal.writer.completed'
  | 'proposal.writer.failed'
  | 'proposal.section.updated'
  | 'proposal.gate.reached'
  | 'proposal.approved'
  | 'proposal.finalreview.started'
  | 'proposal.finalreview.completed'
  | 'proposal.finalreview.needs_human'
  | 'proposal.submitted'
>;

/** Proposal-category event. A proposal *trace* = all events sharing trace_id. */
export interface ProposalEvent extends BaseEvent {
  category: 'proposal';
  name: ProposalEventName;
}

/** LLM event names emitted by the core's reference hosts. Open by design. */
export type LLMEventName = OpenLiteral<
  'llm.request.started' | 'llm.request.completed' | 'llm.fallback.triggered'
>;

/** LLM-category event. Nests under a phase span via parent_span_id/span_id. */
export interface LLMEvent extends BaseEvent {
  category: 'llm';
  name: LLMEventName;
}

/**
 * TelemetryEvent is the discriminated union the Monitor renders. Narrow on
 * `category` to access category-specific name unions and attribute helpers.
 */
export type TelemetryEvent = JourneyEvent | SystemEvent | ProposalEvent | LLMEvent;

/* ------------------------------------------------------------------------- *
 * Attribute readers
 *
 * Attributes arrive as an unordered list of class-tagged key/value pairs.
 * These helpers locate a key and narrow its value to a concrete type without
 * leaking `any` past the boundary.
 * ------------------------------------------------------------------------- */

/** findAttr returns the first attribute with the given key, or undefined. */
export function findAttr(ev: TelemetryEvent, key: string): Attr | undefined {
  return ev.attributes?.find((a) => a.key === key);
}

/** attrNumber reads a numeric attribute, or undefined if absent/non-numeric. */
export function attrNumber(ev: TelemetryEvent, key: string): number | undefined {
  const v = findAttr(ev, key)?.value;
  return typeof v === 'number' ? v : undefined;
}

/** attrString reads a string attribute, or undefined if absent/non-string. */
export function attrString(ev: TelemetryEvent, key: string): string | undefined {
  const v = findAttr(ev, key)?.value;
  return typeof v === 'string' ? v : undefined;
}

/** attrBoolean reads a boolean attribute, or undefined if absent/non-boolean. */
export function attrBoolean(ev: TelemetryEvent, key: string): boolean | undefined {
  const v = findAttr(ev, key)?.value;
  return typeof v === 'boolean' ? v : undefined;
}

/* ------------------------------------------------------------------------- *
 * Per-category attribute shapes
 * ------------------------------------------------------------------------- */

/**
 * LLMUsage is the usage-class attribute bundle carried by llm.request.completed
 * events. Every field is optional because attributes may be partial or redacted.
 */
export interface LLMUsage {
  input_tokens?: number;
  output_tokens?: number;
  thinking_tokens?: number;
  total_tokens?: number;
  finish_reason?: string;
  truncated?: boolean;
  latency_ms?: number;
  cost_usd?: number;
}

/** readLLMUsage extracts the LLMUsage bundle from an LLM event's attributes. */
export function readLLMUsage(ev: LLMEvent): LLMUsage {
  return {
    input_tokens: attrNumber(ev, 'input_tokens'),
    output_tokens: attrNumber(ev, 'output_tokens'),
    thinking_tokens: attrNumber(ev, 'thinking_tokens'),
    total_tokens: attrNumber(ev, 'total_tokens'),
    finish_reason: attrString(ev, 'finish_reason'),
    truncated: attrBoolean(ev, 'truncated'),
    latency_ms: attrNumber(ev, 'latency_ms'),
    cost_usd: attrNumber(ev, 'cost_usd'),
  };
}
