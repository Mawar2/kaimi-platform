package event

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"
)

// SchemaVersion is the current version of the Event envelope shape. It is
// stamped onto every Event created via NewEvent so that stored events remain
// interpretable as the schema evolves.
const SchemaVersion = 1

// Category is a coarse, domain-agnostic grouping for an Event. The core ships a
// small fixed set; hosts classify their opaque event names into one of these.
type Category string

// The categories recognized by the core.
const (
	// CategoryJourney covers product/user-journey events (e.g. a user reaching
	// a milestone in the host application).
	CategoryJourney Category = "journey"
	// CategorySystem covers infrastructure and lifecycle events (e.g. boot,
	// shutdown, scheduled-run start).
	CategorySystem Category = "system"
	// CategoryProposal covers host work-unit events. The name is generic on
	// purpose; the core attaches no proposal-specific meaning.
	CategoryProposal Category = "proposal"
	// CategoryLLM covers model-interaction events (e.g. a generation call).
	CategoryLLM Category = "llm"
)

// Level is the severity or importance of an Event (e.g. "info", "warn",
// "error"). Values are opaque strings chosen by the host.
type Level string

// Common Level values. Hosts may use others; the core does not enumerate them.
const (
	// LevelDebug marks low-importance diagnostic events.
	LevelDebug Level = "debug"
	// LevelInfo marks normal, expected events.
	LevelInfo Level = "info"
	// LevelWarn marks recoverable anomalies worth attention.
	LevelWarn Level = "warn"
	// LevelError marks failures.
	LevelError Level = "error"
)

// Actor identifies who or what produced an Event — a human, an agent, or a
// system component. All fields are optional.
type Actor struct {
	// Kind is the broad type of actor (e.g. "user", "agent", "system").
	Kind string `json:"kind,omitempty"`
	// ID is a stable identifier for the actor within its Kind.
	ID string `json:"id,omitempty"`
	// Name is a human-readable label for the actor.
	Name string `json:"name,omitempty"`
}

// Class marks whether an attribute's value is safe to forward (usage) or must
// stay inside the deployment (content). It is the seam the redaction gate acts
// on.
type Class uint8

// The attribute classes. Their numeric values are stable: usage is the safe
// default (0) and content is the protected class (1).
const (
	// ClassUsage marks metadata safe to forward to a central sink — counts,
	// durations, model names, token totals, and the like.
	ClassUsage Class = 0
	// ClassContent marks sensitive payloads — prompts, responses, user text —
	// that must never leave the deployment.
	ClassContent Class = 1
)

// Attr is a single key/value pair on an Event, tagged with the Class that
// determines whether it may be forwarded.
type Attr struct {
	// Key is the attribute name.
	Key string `json:"key"`
	// Value is the attribute value. It must be JSON-serializable.
	Value any `json:"value"`
	// Class is the redaction class for this attribute.
	Class Class `json:"class"`
}

// Usage returns an Attr classified as ClassUsage (safe to forward).
func Usage(key string, v any) Attr {
	return Attr{Key: key, Value: v, Class: ClassUsage}
}

// Content returns an Attr classified as ClassContent (must not leave the
// deployment).
func Content(key string, v any) Attr {
	return Attr{Key: key, Value: v, Class: ClassContent}
}

// Attrs is an ordered collection of Attr.
type Attrs []Attr

// Get returns the value of the first attribute with the given key and true, or
// nil and false if no such attribute exists.
func (a Attrs) Get(key string) (any, bool) {
	for _, attr := range a {
		if attr.Key == key {
			return attr.Value, true
		}
	}
	return nil, false
}

// Event is the envelope every part of the telemetry core operates on. Required
// fields (EventID, SchemaVersion, OccurredAt, Category, Name) are always
// present; the rest are optional and omitted from JSON when empty.
type Event struct {
	// EventID is a unique identifier for this event (32 hex chars).
	EventID string `json:"event_id"`
	// SchemaVersion is the envelope version this event was produced under.
	SchemaVersion int `json:"schema_version"`
	// OccurredAt is when the event happened, in UTC.
	OccurredAt time.Time `json:"occurred_at"`
	// TenantID scopes the event to a tenant in a multi-tenant deployment.
	TenantID string `json:"tenant_id,omitempty"`
	// Category is the coarse grouping for the event.
	Category Category `json:"category"`
	// Name is the host-supplied, opaque event name.
	Name string `json:"name"`
	// Level is the severity/importance of the event.
	Level Level `json:"level,omitempty"`
	// Actor identifies who or what produced the event. omitzero (Go 1.24+)
	// drops the field entirely when the Actor is its zero value, since a value
	// struct is never omitted by omitempty.
	Actor Actor `json:"actor,omitzero"`
	// TraceID groups events belonging to one distributed trace.
	TraceID string `json:"trace_id,omitempty"`
	// SpanID identifies this event's span within the trace.
	SpanID string `json:"span_id,omitempty"`
	// ParentSpanID identifies the parent span, if any.
	ParentSpanID string `json:"parent_span_id,omitempty"`
	// DurationMS is the elapsed time in milliseconds for a spanning event.
	DurationMS int64 `json:"duration_ms,omitempty"`
	// Attributes are the typed, class-tagged key/value pairs for the event.
	Attributes Attrs `json:"attributes,omitempty"`
}

// NewEvent builds an Event for the given category and name with the supplied
// attributes, stamping a fresh EventID, the current SchemaVersion, and a UTC
// OccurredAt timestamp. It panics only if the system's secure random source
// is unavailable, which indicates a broken platform rather than a recoverable
// condition.
func NewEvent(category Category, name string, attrs ...Attr) Event {
	return Event{
		EventID:       newEventID(),
		SchemaVersion: SchemaVersion,
		OccurredAt:    time.Now().UTC(),
		Category:      category,
		Name:          name,
		Attributes:    Attrs(attrs),
	}
}

// newEventID returns a 32-character hex string from 16 cryptographically
// random bytes.
func newEventID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		// crypto/rand failing means the OS CSPRNG is unavailable; there is no
		// safe fallback for a unique identifier, so fail loudly.
		panic(fmt.Errorf("event: read random bytes for EventID: %w", err))
	}
	return hex.EncodeToString(b[:])
}
