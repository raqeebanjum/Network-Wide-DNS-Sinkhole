package config

import (
	"fmt"
	"log/slog"
	"net"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	defaultDNSListen        = ":1053"
	defaultHTTPListen       = ":8080"
	defaultDataDir          = "./data"
	defaultUpstreams        = "1.1.1.1:53,1.0.0.1:53"
	defaultQueryRetention   = 30 * 24 * time.Hour
	defaultBlocklistRefresh = 24 * time.Hour
	defaultLogLevel         = "info"
	defaultLogFormat        = "text"
)

// Config contains the runtime settings shared by the DNS and HTTP servers.
type Config struct {
	DNSListen        string
	HTTPListen       string
	DataDir          string
	Upstreams        []string
	QueryRetention   time.Duration
	BlocklistRefresh time.Duration
	LogLevel         slog.Level
	LogFormat        string
}

// Load reads configuration from SINKHOLE_* environment variables.
func Load() (Config, error) {
	return load(os.LookupEnv)
}

func load(lookupEnv func(string) (string, bool)) (Config, error) {
	dnsListen, err := listenAddress(lookupEnv, "SINKHOLE_DNS_LISTEN", defaultDNSListen)
	if err != nil {
		return Config{}, err
	}

	httpListen, err := listenAddress(lookupEnv, "SINKHOLE_HTTP_LISTEN", defaultHTTPListen)
	if err != nil {
		return Config{}, err
	}

	dataDir := value(lookupEnv, "SINKHOLE_DATA_DIR", defaultDataDir)
	if dataDir == "" {
		return Config{}, fmt.Errorf("SINKHOLE_DATA_DIR must not be empty")
	}

	upstreams, err := parseUpstreams(value(lookupEnv, "SINKHOLE_UPSTREAMS", defaultUpstreams))
	if err != nil {
		return Config{}, fmt.Errorf("SINKHOLE_UPSTREAMS: %w", err)
	}

	queryRetention, err := durationValue(lookupEnv, "SINKHOLE_QUERY_RETENTION", defaultQueryRetention)
	if err != nil {
		return Config{}, err
	}

	blocklistRefresh, err := durationValue(lookupEnv, "SINKHOLE_BLOCKLIST_REFRESH", defaultBlocklistRefresh)
	if err != nil {
		return Config{}, err
	}

	logLevel, err := parseLogLevel(value(lookupEnv, "SINKHOLE_LOG_LEVEL", defaultLogLevel))
	if err != nil {
		return Config{}, fmt.Errorf("SINKHOLE_LOG_LEVEL: %w", err)
	}

	logFormat := strings.ToLower(value(lookupEnv, "SINKHOLE_LOG_FORMAT", defaultLogFormat))
	if logFormat != "text" && logFormat != "json" {
		return Config{}, fmt.Errorf("SINKHOLE_LOG_FORMAT must be text or json, got %q", logFormat)
	}

	return Config{
		DNSListen:        dnsListen,
		HTTPListen:       httpListen,
		DataDir:          dataDir,
		Upstreams:        upstreams,
		QueryRetention:   queryRetention,
		BlocklistRefresh: blocklistRefresh,
		LogLevel:         logLevel,
		LogFormat:        logFormat,
	}, nil
}

func value(lookupEnv func(string) (string, bool), key, fallback string) string {
	if raw, ok := lookupEnv(key); ok {
		return strings.TrimSpace(raw)
	}
	return fallback
}

func listenAddress(lookupEnv func(string) (string, bool), key, fallback string) (string, error) {
	address := value(lookupEnv, key, fallback)
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return "", fmt.Errorf("%s must be a host:port address: %w", key, err)
	}
	if err := validatePort(port); err != nil {
		return "", fmt.Errorf("%s: %w", key, err)
	}
	if strings.Contains(host, "%") {
		return "", fmt.Errorf("%s must not include an IPv6 zone", key)
	}
	return address, nil
}

func parseUpstreams(raw string) ([]string, error) {
	parts := strings.Split(raw, ",")
	upstreams := make([]string, 0, len(parts))
	seen := make(map[string]struct{}, len(parts))

	for _, part := range parts {
		upstream := strings.TrimSpace(part)
		if upstream == "" {
			return nil, fmt.Errorf("must not contain an empty resolver address")
		}

		host, port, err := net.SplitHostPort(upstream)
		if err != nil {
			return nil, fmt.Errorf("%q must be a host:port address: %w", upstream, err)
		}
		if host == "" {
			return nil, fmt.Errorf("%q must include a host", upstream)
		}
		if err := validatePort(port); err != nil {
			return nil, fmt.Errorf("%q: %w", upstream, err)
		}
		if _, exists := seen[upstream]; exists {
			return nil, fmt.Errorf("%q is listed more than once", upstream)
		}
		seen[upstream] = struct{}{}
		upstreams = append(upstreams, upstream)
	}

	return upstreams, nil
}

func validatePort(raw string) error {
	port, err := strconv.Atoi(raw)
	if err != nil || port < 1 || port > 65535 {
		return fmt.Errorf("port must be an integer from 1 to 65535, got %q", raw)
	}
	return nil
}

func durationValue(lookupEnv func(string) (string, bool), key string, fallback time.Duration) (time.Duration, error) {
	raw := value(lookupEnv, key, fallback.String())
	duration, err := time.ParseDuration(raw)
	if err != nil {
		return 0, fmt.Errorf("%s must be a duration such as 24h: %w", key, err)
	}
	if duration <= 0 {
		return 0, fmt.Errorf("%s must be greater than zero", key)
	}
	return duration, nil
}

func parseLogLevel(raw string) (slog.Level, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "debug":
		return slog.LevelDebug, nil
	case "info":
		return slog.LevelInfo, nil
	case "warn", "warning":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return 0, fmt.Errorf("must be debug, info, warn, or error, got %q", raw)
	}
}
