package logging

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
)

func TestNewJSONLogger(t *testing.T) {
	t.Parallel()

	var output bytes.Buffer
	logger := New(&output, slog.LevelInfo, "json")

	logger.Debug("hidden")
	logger.Info("server started", "address", ":8080")

	var entry map[string]any
	if err := json.Unmarshal(output.Bytes(), &entry); err != nil {
		t.Fatalf("decode log output: %v\noutput: %s", err, output.String())
	}
	if entry["msg"] != "server started" {
		t.Errorf("msg = %v, want server started", entry["msg"])
	}
	if entry["address"] != ":8080" {
		t.Errorf("address = %v, want :8080", entry["address"])
	}
}

func TestNewWithBufferMirrorsRecords(t *testing.T) {
	t.Parallel()

	var output bytes.Buffer
	logger, buffer := NewWithBuffer(&output, slog.LevelInfo, "json", 10)
	logger.Info("listener started", "port", 1053)

	if !strings.Contains(output.String(), "listener started") {
		t.Errorf("stdout output is missing record: %s", output.String())
	}
	entries := buffer.Entries()
	if len(entries) != 1 {
		t.Fatalf("buffer entries = %d, want 1", len(entries))
	}
	if entries[0].Message != "listener started" || entries[0].Attributes["port"] != int64(1053) {
		t.Errorf("buffer entry = %#v", entries[0])
	}
}

func TestNewTextLoggerHonorsLevel(t *testing.T) {
	t.Parallel()

	var output bytes.Buffer
	logger := New(&output, slog.LevelWarn, "text")

	logger.Info("hidden")
	logger.Warn("upstream unavailable")

	if strings.Contains(output.String(), "hidden") {
		t.Errorf("output contains filtered INFO log: %s", output.String())
	}
	if !strings.Contains(output.String(), "upstream unavailable") {
		t.Errorf("output is missing WARN log: %s", output.String())
	}
}
