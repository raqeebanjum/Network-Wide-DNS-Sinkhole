package logging

import (
	"io"
	"log/slog"

	"github.com/raqeebanjum/network-wide-dns-sinkhole/internal/oplog"
)

// New creates the application logger. Config validation guarantees that
// format is either "text" or "json".
func New(w io.Writer, level slog.Level, format string) *slog.Logger {
	return slog.New(newHandler(w, level, format))
}

// NewWithBuffer also mirrors recent operational entries into a bounded buffer
// exposed by the dashboard.
func NewWithBuffer(w io.Writer, level slog.Level, format string, capacity int) (*slog.Logger, *oplog.Buffer) {
	buffer := oplog.NewBuffer(capacity)
	handler := oplog.NewHandler(newHandler(w, level, format), buffer)
	return slog.New(handler), buffer
}

func newHandler(w io.Writer, level slog.Level, format string) slog.Handler {
	options := &slog.HandlerOptions{Level: level}
	if format == "json" {
		return slog.NewJSONHandler(w, options)
	}
	return slog.NewTextHandler(w, options)
}
