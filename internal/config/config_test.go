package config

import (
	"log/slog"
	"strings"
	"testing"
	"time"
)

func TestLoadDefaults(t *testing.T) {
	t.Parallel()

	cfg, err := load(mapLookup(nil))
	if err != nil {
		t.Fatalf("load defaults: %v", err)
	}

	if cfg.DNSListen != ":1053" {
		t.Errorf("DNSListen = %q, want :1053", cfg.DNSListen)
	}
	if cfg.HTTPListen != ":8080" {
		t.Errorf("HTTPListen = %q, want :8080", cfg.HTTPListen)
	}
	if cfg.DataDir != "./data" {
		t.Errorf("DataDir = %q, want ./data", cfg.DataDir)
	}
	if got, want := strings.Join(cfg.Upstreams, ","), "1.1.1.1:53,1.0.0.1:53"; got != want {
		t.Errorf("Upstreams = %q, want %q", got, want)
	}
	if cfg.DNSRequestTimeout != 5*time.Second {
		t.Errorf("DNSRequestTimeout = %s, want 5s", cfg.DNSRequestTimeout)
	}
	if cfg.DNSMaxInflight != 256 {
		t.Errorf("DNSMaxInflight = %d, want 256", cfg.DNSMaxInflight)
	}
	if cfg.QueryRetention != 30*24*time.Hour {
		t.Errorf("QueryRetention = %s, want 720h", cfg.QueryRetention)
	}
	if cfg.BlocklistRefresh != 24*time.Hour {
		t.Errorf("BlocklistRefresh = %s, want 24h", cfg.BlocklistRefresh)
	}
	if cfg.LogLevel != slog.LevelInfo {
		t.Errorf("LogLevel = %s, want INFO", cfg.LogLevel)
	}
	if cfg.LogFormat != "text" {
		t.Errorf("LogFormat = %q, want text", cfg.LogFormat)
	}
}

func TestLoadOverrides(t *testing.T) {
	t.Parallel()

	cfg, err := load(mapLookup(map[string]string{
		"SINKHOLE_DNS_LISTEN":        "127.0.0.1:5353",
		"SINKHOLE_HTTP_LISTEN":       "localhost:9090",
		"SINKHOLE_DATA_DIR":          "/tmp/sinkhole-test",
		"SINKHOLE_UPSTREAMS":         "9.9.9.9:53, [2620:fe::fe]:53",
		"SINKHOLE_DNS_TIMEOUT":       "750ms",
		"SINKHOLE_DNS_MAX_INFLIGHT":  "64",
		"SINKHOLE_QUERY_RETENTION":   "168h",
		"SINKHOLE_BLOCKLIST_REFRESH": "6h30m",
		"SINKHOLE_LOG_LEVEL":         "debug",
		"SINKHOLE_LOG_FORMAT":        "JSON",
	}))
	if err != nil {
		t.Fatalf("load overrides: %v", err)
	}

	if cfg.DNSListen != "127.0.0.1:5353" {
		t.Errorf("DNSListen = %q", cfg.DNSListen)
	}
	if cfg.HTTPListen != "localhost:9090" {
		t.Errorf("HTTPListen = %q", cfg.HTTPListen)
	}
	if cfg.DataDir != "/tmp/sinkhole-test" {
		t.Errorf("DataDir = %q", cfg.DataDir)
	}
	if got, want := strings.Join(cfg.Upstreams, ","), "9.9.9.9:53,[2620:fe::fe]:53"; got != want {
		t.Errorf("Upstreams = %q, want %q", got, want)
	}
	if cfg.DNSRequestTimeout != 750*time.Millisecond {
		t.Errorf("DNSRequestTimeout = %s, want 750ms", cfg.DNSRequestTimeout)
	}
	if cfg.DNSMaxInflight != 64 {
		t.Errorf("DNSMaxInflight = %d, want 64", cfg.DNSMaxInflight)
	}
	if cfg.QueryRetention != 7*24*time.Hour {
		t.Errorf("QueryRetention = %s, want 168h", cfg.QueryRetention)
	}
	if cfg.BlocklistRefresh != 6*time.Hour+30*time.Minute {
		t.Errorf("BlocklistRefresh = %s, want 6h30m", cfg.BlocklistRefresh)
	}
	if cfg.LogLevel != slog.LevelDebug {
		t.Errorf("LogLevel = %s, want DEBUG", cfg.LogLevel)
	}
	if cfg.LogFormat != "json" {
		t.Errorf("LogFormat = %q, want json", cfg.LogFormat)
	}
}

func TestLoadRejectsInvalidValues(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		env  map[string]string
		want string
	}{
		{
			name: "invalid DNS listen address",
			env:  map[string]string{"SINKHOLE_DNS_LISTEN": "localhost"},
			want: "SINKHOLE_DNS_LISTEN",
		},
		{
			name: "invalid DNS port",
			env:  map[string]string{"SINKHOLE_DNS_LISTEN": ":70000"},
			want: "port must be",
		},
		{
			name: "empty data directory",
			env:  map[string]string{"SINKHOLE_DATA_DIR": "  "},
			want: "SINKHOLE_DATA_DIR",
		},
		{
			name: "upstream without port",
			env:  map[string]string{"SINKHOLE_UPSTREAMS": "1.1.1.1"},
			want: "host:port",
		},
		{
			name: "empty upstream entry",
			env:  map[string]string{"SINKHOLE_UPSTREAMS": "1.1.1.1:53,"},
			want: "empty resolver",
		},
		{
			name: "duplicate upstream",
			env:  map[string]string{"SINKHOLE_UPSTREAMS": "1.1.1.1:53,1.1.1.1:53"},
			want: "more than once",
		},
		{
			name: "invalid DNS timeout",
			env:  map[string]string{"SINKHOLE_DNS_TIMEOUT": "five seconds"},
			want: "SINKHOLE_DNS_TIMEOUT",
		},
		{
			name: "non-positive DNS concurrency",
			env:  map[string]string{"SINKHOLE_DNS_MAX_INFLIGHT": "0"},
			want: "greater than zero",
		},
		{
			name: "invalid DNS concurrency",
			env:  map[string]string{"SINKHOLE_DNS_MAX_INFLIGHT": "many"},
			want: "integer",
		},
		{
			name: "invalid retention",
			env:  map[string]string{"SINKHOLE_QUERY_RETENTION": "30 days"},
			want: "SINKHOLE_QUERY_RETENTION",
		},
		{
			name: "negative retention",
			env:  map[string]string{"SINKHOLE_QUERY_RETENTION": "-1h"},
			want: "greater than zero",
		},
		{
			name: "invalid refresh",
			env:  map[string]string{"SINKHOLE_BLOCKLIST_REFRESH": "tomorrow"},
			want: "SINKHOLE_BLOCKLIST_REFRESH",
		},
		{
			name: "invalid log level",
			env:  map[string]string{"SINKHOLE_LOG_LEVEL": "trace"},
			want: "SINKHOLE_LOG_LEVEL",
		},
		{
			name: "invalid log format",
			env:  map[string]string{"SINKHOLE_LOG_FORMAT": "xml"},
			want: "SINKHOLE_LOG_FORMAT",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			_, err := load(mapLookup(test.env))
			if err == nil {
				t.Fatal("load returned nil error")
			}
			if !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %q, want it to contain %q", err, test.want)
			}
		})
	}
}

func mapLookup(values map[string]string) func(string) (string, bool) {
	return func(key string) (string, bool) {
		value, ok := values[key]
		return value, ok
	}
}
