package oplog

import (
	"context"
	"log/slog"
	"strings"
	"time"
)

// Handler forwards structured logs to an underlying handler and mirrors them
// into a bounded Buffer for the dashboard.
type scopedAttr struct {
	groups []string
	attr   slog.Attr
}

type Handler struct {
	next   slog.Handler
	buffer *Buffer
	attrs  []scopedAttr
	groups []string
}

// NewHandler wraps next and mirrors enabled records into buffer.
func NewHandler(next slog.Handler, buffer *Buffer) *Handler {
	return &Handler{next: next, buffer: buffer}
}

// Enabled reports whether the underlying handler accepts the level.
func (h *Handler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.next.Enabled(ctx, level)
}

// Handle records the log entry in memory and forwards it to the original sink.
func (h *Handler) Handle(ctx context.Context, record slog.Record) error {
	if h.buffer != nil {
		attributes := make(map[string]any)
		for _, stored := range h.attrs {
			collectAttribute(attributes, stored.groups, stored.attr)
		}

		groups := h.groups
		record.Attrs(func(attr slog.Attr) bool {
			collectAttribute(attributes, groups, attr)
			return true
		})

		h.buffer.Add(Entry{
			Time:       record.Time.UTC(),
			Level:      record.Level.String(),
			Message:    record.Message,
			Attributes: attributes,
		})
	}

	return h.next.Handle(ctx, record)
}

// WithAttrs stores attrs for both the in-memory copy and the underlying handler.
func (h *Handler) WithAttrs(attrs []slog.Attr) slog.Handler {
	if len(attrs) == 0 {
		return h
	}

	stored := make([]scopedAttr, 0, len(h.attrs)+len(attrs))
	stored = append(stored, h.attrs...)
	for _, attr := range attrs {
		stored = append(stored, scopedAttr{
			groups: append([]string(nil), h.groups...),
			attr:   attr,
		})
	}

	return &Handler{
		next:   h.next.WithAttrs(attrs),
		buffer: h.buffer,
		attrs:  stored,
		groups: append([]string(nil), h.groups...),
	}
}

// WithGroup applies a group to both the in-memory copy and the underlying handler.
func (h *Handler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}

	groups := make([]string, 0, len(h.groups)+1)
	groups = append(groups, h.groups...)
	groups = append(groups, name)

	return &Handler{
		next:   h.next.WithGroup(name),
		buffer: h.buffer,
		attrs:  append([]scopedAttr(nil), h.attrs...),
		groups: groups,
	}
}

func collectAttribute(destination map[string]any, groups []string, attr slog.Attr) {
	attr.Value = attr.Value.Resolve()
	if attr.Equal(slog.Attr{}) {
		return
	}

	if attr.Value.Kind() == slog.KindGroup {
		nextGroups := groups
		if attr.Key != "" {
			nextGroups = appendCopy(groups, attr.Key)
		}
		for _, nested := range attr.Value.Group() {
			collectAttribute(destination, nextGroups, nested)
		}
		return
	}

	key := strings.Join(appendCopy(groups, attr.Key), ".")
	destination[key] = normalizeValue(attr.Value.Any())
}

func appendCopy(values []string, value string) []string {
	result := make([]string, 0, len(values)+1)
	result = append(result, values...)
	result = append(result, value)
	return result
}

func normalizeValue(value any) any {
	switch typed := value.(type) {
	case error:
		return typed.Error()
	case time.Duration:
		return typed.String()
	case time.Time:
		return typed.UTC().Format(time.RFC3339Nano)
	case []byte:
		return string(typed)
	default:
		return value
	}
}
