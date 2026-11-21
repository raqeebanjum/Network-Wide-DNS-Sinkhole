package oplog

import (
	"fmt"
	"testing"
)

func TestBufferKeepsNewestEntries(t *testing.T) {
	t.Parallel()

	buffer := NewBuffer(2)
	for index := range 3 {
		buffer.Add(Entry{Message: fmt.Sprintf("entry-%d", index)})
	}

	if buffer.Len() != 2 {
		t.Fatalf("Len = %d, want 2", buffer.Len())
	}
	entries := buffer.Entries()
	if len(entries) != 2 {
		t.Fatalf("entries = %d, want 2", len(entries))
	}
	if entries[0].Message != "entry-2" || entries[1].Message != "entry-1" {
		t.Errorf("entries = %#v", entries)
	}
}

func TestBufferReturnsAttributeCopies(t *testing.T) {
	t.Parallel()

	buffer := NewBuffer(1)
	buffer.Add(Entry{
		Message:    "test",
		Attributes: map[string]any{"key": "original"},
	})

	entries := buffer.Entries()
	entries[0].Attributes["key"] = "changed"

	again := buffer.Entries()
	if again[0].Attributes["key"] != "original" {
		t.Errorf("stored attributes mutated: %#v", again[0].Attributes)
	}
}
