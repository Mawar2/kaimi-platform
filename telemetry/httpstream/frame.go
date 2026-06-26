package httpstream

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/Mawar2/kaimi-telemetry/sink"
)

// writeFrame writes one Server-Sent Events frame to b: an "id:" line carrying
// seq, an "event:" line carrying category (omitted when empty), one "data:"
// line per line of payload, and the trailing blank line that terminates the
// frame. Splitting payload on '\n' keeps multi-line JSON valid on the wire,
// since SSE treats a bare newline inside a data value as a frame boundary.
func writeFrame(b *strings.Builder, seq uint64, category, payload string) {
	b.WriteString("id: ")
	b.WriteString(strconv.FormatUint(seq, 10))
	b.WriteByte('\n')

	if category != "" {
		b.WriteString("event: ")
		b.WriteString(category)
		b.WriteByte('\n')
	}

	for _, line := range strings.Split(payload, "\n") {
		b.WriteString("data: ")
		b.WriteString(line)
		b.WriteByte('\n')
	}

	b.WriteByte('\n')
}

// appendStamped marshals s.Event to JSON and appends its SSE frame to b. It
// returns an error only when the event cannot be marshaled, leaving b unchanged
// in that case so a single bad event cannot corrupt the stream. s is taken by
// pointer because sink.Stamped embeds the full event envelope.
func appendStamped(b *strings.Builder, s *sink.Stamped) error {
	payload, err := json.Marshal(s.Event)
	if err != nil {
		return fmt.Errorf("marshal event %s: %w", s.Event.EventID, err)
	}
	writeFrame(b, s.Seq, string(s.Event.Category), string(payload))
	return nil
}
