// Package oplog keeps a bounded in-memory view of recent operational logs.
package oplog

import (
	"sync"
	"time"
)

// DefaultCapacity is the number of recent operational entries retained.
const DefaultCapacity = 500

// Entry is one structured operational log record.
type Entry struct {
	Time       time.Time      `json:"time"`
	Level      string         `json:"level"`
	Message    string         `json:"message"`
	Attributes map[string]any `json:"attributes"`
}

// Buffer is a concurrency-safe ring of recent log entries.
type Buffer struct {
	mu      sync.RWMutex
	entries []Entry
	next    int
	full    bool
}

// NewBuffer creates a ring retaining at most capacity entries.
func NewBuffer(capacity int) *Buffer {
	if capacity <= 0 {
		capacity = DefaultCapacity
	}
	return &Buffer{entries: make([]Entry, capacity)}
}

// Add appends one entry and evicts the oldest when full.
func (b *Buffer) Add(entry Entry) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.entries[b.next] = entry
	b.next = (b.next + 1) % len(b.entries)
	if b.next == 0 {
		b.full = true
	}
}

// Entries returns newest-first copies of the retained entries.
func (b *Buffer) Entries() []Entry {
	b.mu.RLock()
	defer b.mu.RUnlock()

	length := b.next
	if b.full {
		length = len(b.entries)
	}
	result := make([]Entry, 0, length)
	for offset := 1; offset <= length; offset++ {
		index := (b.next - offset + len(b.entries)) % len(b.entries)
		entry := b.entries[index]
		if entry.Attributes != nil {
			attributes := make(map[string]any, len(entry.Attributes))
			for key, value := range entry.Attributes {
				attributes[key] = value
			}
			entry.Attributes = attributes
		}
		result = append(result, entry)
	}
	return result
}

// Len returns the number of retained entries.
func (b *Buffer) Len() int {
	b.mu.RLock()
	defer b.mu.RUnlock()

	if b.full {
		return len(b.entries)
	}
	return b.next
}
