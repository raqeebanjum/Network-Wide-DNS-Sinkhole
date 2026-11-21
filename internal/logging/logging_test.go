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
