package blocklists

import (
	"fmt"
	"net/url"
	"strings"
)

const maxURLLength = 2048

func normalizeListURL(raw string) (string, error) {
	value := strings.TrimSpace(raw)
	if len(value) > maxURLLength {
		return "", fmt.Errorf("%w: URL is too long", ErrInvalid)
	}

	parsed, err := url.ParseRequestURI(value)
	if err != nil {
		return "", fmt.Errorf("%w: URL is malformed", ErrInvalid)
	}
	if parsed.Scheme != "https" {
		return "", fmt.Errorf("%w: blocklist URLs must use HTTPS", ErrInvalid)
	}
	if parsed.Hostname() == "" {
		return "", fmt.Errorf("%w: URL must include a host", ErrInvalid)
	}
	if parsed.User != nil {
		return "", fmt.Errorf("%w: URL must not include credentials", ErrInvalid)
	}
	parsed.Fragment = ""
	return parsed.String(), nil
}

func normalizeHomepage(raw string) (string, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "", nil
	}
	if len(value) > maxURLLength {
		return "", fmt.Errorf("%w: homepage URL is too long", ErrInvalid)
	}

	parsed, err := url.ParseRequestURI(value)
	if err != nil {
		return "", fmt.Errorf("%w: homepage URL is malformed", ErrInvalid)
	}
	if parsed.Scheme != "https" && parsed.Scheme != "http" {
		return "", fmt.Errorf("%w: homepage must use HTTP or HTTPS", ErrInvalid)
	}
	if parsed.Hostname() == "" || parsed.User != nil {
		return "", fmt.Errorf("%w: homepage URL is invalid", ErrInvalid)
	}
	parsed.Fragment = ""
	return parsed.String(), nil
}
