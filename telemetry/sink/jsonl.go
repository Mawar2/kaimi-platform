package sink

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sync"

	"github.com/Mawar2/kaimi-telemetry/event"
)

// JSONLSink writes events as append-only, newline-delimited JSON (one event per
// line) to a single file. Writes are buffered for throughput; Flush drains the
// buffer and fsyncs the file for durability. It is safe for concurrent use.
type JSONLSink struct {
	mu     sync.Mutex
	f      *os.File
	w      *bufio.Writer
	closed bool
}

// NewJSONLSink opens (creating if needed) the file at path for appending and
// returns a JSONLSink writing to it.
func NewJSONLSink(path string) (*JSONLSink, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open jsonl sink %q: %w", path, err)
	}
	return &JSONLSink{f: f, w: bufio.NewWriter(f)}, nil
}

// Emit marshals e to JSON and appends it as a single line. It returns ErrClosed
// if the sink has been closed.
//
//nolint:gocritic // hugeParam: the EventSink.Emit contract mandates a value param.
func (s *JSONLSink) Emit(_ context.Context, e event.Event) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return ErrClosed
	}

	line, err := json.Marshal(e)
	if err != nil {
		return fmt.Errorf("marshal event %s: %w", e.EventID, err)
	}
	if _, err := s.w.Write(line); err != nil {
		return fmt.Errorf("write event %s: %w", e.EventID, err)
	}
	if err := s.w.WriteByte('\n'); err != nil {
		return fmt.Errorf("write newline for event %s: %w", e.EventID, err)
	}
	return nil
}

// Flush drains the write buffer to the file and fsyncs it to stable storage. It
// returns ErrClosed if the sink has been closed.
func (s *JSONLSink) Flush(_ context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return ErrClosed
	}
	return s.flushLocked()
}

// flushLocked flushes the buffer and fsyncs the file. The caller must hold s.mu.
func (s *JSONLSink) flushLocked() error {
	if err := s.w.Flush(); err != nil {
		return fmt.Errorf("flush jsonl buffer: %w", err)
	}
	if err := s.f.Sync(); err != nil {
		return fmt.Errorf("fsync jsonl file: %w", err)
	}
	return nil
}

// Close flushes any buffered data and closes the underlying file. It is
// idempotent: calling Close more than once returns nil after the first success.
func (s *JSONLSink) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	s.closed = true

	flushErr := s.flushLocked()
	if err := s.f.Close(); err != nil {
		return errors.Join(flushErr, fmt.Errorf("close jsonl file: %w", err))
	}
	return flushErr
}
