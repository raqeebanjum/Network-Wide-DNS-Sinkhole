package rules

import (
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"golang.org/x/net/idna"
)

// ErrInvalid identifies input that cannot be stored as a rule.
var ErrInvalid = errors.New("invalid rule")

// Kind determines whether a matching domain is allowed or denied.
type Kind string

const (
	KindAllow Kind = "allow"
	KindDeny  Kind = "deny"
)

// Rule is a manually managed allow or deny rule.
type Rule struct {
	ID        int64
	Domain    string
	Kind      Kind
	Note      string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Input contains unnormalized values supplied for a rule mutation.
type Input struct {
	Domain string
	Kind   Kind
	Note   string
}

// ParseKind validates a rule kind supplied by an API client.
func ParseKind(raw string) (Kind, error) {
	switch Kind(strings.ToLower(strings.TrimSpace(raw))) {
	case KindAllow:
		return KindAllow, nil
	case KindDeny:
		return KindDeny, nil
	default:
		return "", fmt.Errorf("%w: type must be allow or deny", ErrInvalid)
	}
}

// NormalizeDomain converts a user-supplied domain into the canonical form used
// for storage and matching. The returned name has no trailing root dot.
func NormalizeDomain(raw string) (string, error) {
	domain := strings.ToLower(strings.TrimSpace(raw))
	domain = strings.TrimSuffix(domain, ".")

	if domain == "" {
		return "", fmt.Errorf("%w: domain must not be empty", ErrInvalid)
	}
	if strings.ContainsAny(domain, " \t\r\n/*:@\\") {
		return "", fmt.Errorf("%w: domain contains unsupported characters", ErrInvalid)
	}

	if !isASCII(domain) {
		ascii, err := idna.Lookup.ToASCII(domain)
		if err != nil {
			return "", fmt.Errorf("%w: domain is not valid IDNA: %v", ErrInvalid, err)
		}
		domain = ascii
	}

	if len(domain) > 253 {
		return "", fmt.Errorf("%w: domain is longer than 253 characters", ErrInvalid)
	}

	labels := strings.Split(domain, ".")
	for _, label := range labels {
		if label == "" || len(label) > 63 {
			return "", fmt.Errorf("%w: domain contains an invalid label", ErrInvalid)
		}
		if label[0] == '-' || label[len(label)-1] == '-' {
			return "", fmt.Errorf("%w: domain labels cannot start or end with a hyphen", ErrInvalid)
		}
		for _, char := range label {
			if !isLabelCharacter(char) {
				return "", fmt.Errorf("%w: domain contains unsupported characters", ErrInvalid)
			}
		}
	}

	return domain, nil
}

// CanonicalName normalizes a DNS wire name for matcher lookup.
func CanonicalName(name string) string {
	return strings.TrimSuffix(strings.ToLower(name), ".")
}

func normalizeInput(input Input) (Input, error) {
	domain, err := NormalizeDomain(input.Domain)
	if err != nil {
		return Input{}, err
	}

	kind, err := ParseKind(string(input.Kind))
	if err != nil {
		return Input{}, err
	}

	note := strings.TrimSpace(input.Note)
	if utf8.RuneCountInString(note) > 500 {
		return Input{}, fmt.Errorf("%w: note is longer than 500 characters", ErrInvalid)
	}

	return Input{Domain: domain, Kind: kind, Note: note}, nil
}

func isASCII(value string) bool {
	for _, char := range value {
		if char > unicode.MaxASCII {
			return false
		}
	}
	return true
}

func isLabelCharacter(char rune) bool {
	return char >= 'a' && char <= 'z' ||
		char >= '0' && char <= '9' ||
		char == '-' ||
		char == '_'
}
