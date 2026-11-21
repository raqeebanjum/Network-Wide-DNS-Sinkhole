package logging

import (
	"io"
	"log/slog"
)

// New creates the application logger. Config validation guarantees that
// format is either "text" or "json".
func New(w io.Writer, level slog.Level, format string) *slog.Logger {
	options := &slog.HandlerOptions{Level: level}

	var handler slog.Handler
	if format == "json" {
		handler = slog.NewJSONHandler(w, options)
	} else {
		handler = slog.NewTextHandler(w, options)
	}

	return slog.New(handler)
}
