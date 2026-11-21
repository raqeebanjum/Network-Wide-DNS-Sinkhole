package oplog

import (
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"
)

func TestHandlerCapturesScopedAttributes(t *testing.T) {
	t.Parallel()

	buffer := NewBuffer(10)
	base := slog.NewTextHandler(io.Discard, nil)
	logger := slog.New(NewHandler(base, buffer))

	logger.
		With("component", "dns").
		WithGroup("upstream").
		Warn("resolution failed",
			"address", "1.1.1.1:53",
			"error", errors.New("timeout"),
			"duration", 2*time.Millisecond,
		)

	entries := buffer.Entries()
	if len(entries) != 1 {
		t.Fatalf("entries = %d, want 1", len(entries))
	}
	entry := entries[0]
	if entry.Level != "WARN" || entry.Message != "resolution failed" {
		t.Errorf("entry = %#v", entry)
	}
	if got := entry.Attributes["component"]; got != "dns" {
		t.Errorf("component = %#v", got)
	}
	if got := entry.Attributes["upstream.address"]; got != "1.1.1.1:53" {
		t.Errorf("upstream.address = %#v", got)
	}
	if got := entry.Attributes["upstream.error"]; got != "timeout" {
		t.Errorf("upstream.error = %#v", got)
	}
	if got := entry.Attributes["upstream.duration"]; got != "2ms" {
		t.Errorf("upstream.duration = %#v", got)
	}
}
