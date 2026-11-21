package rules_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/raqeebanjum/network-wide-dns-sinkhole/internal/rules"
)

func TestNormalizeDomain(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		raw  string
		want string
	}{
		{name: "trailing dot", raw: "Ads.Example.COM.", want: "ads.example.com"},
		{name: "whitespace", raw: "  example.com  ", want: "example.com"},
		{name: "single label", raw: "localhost", want: "localhost"},
		{name: "underscore", raw: "_dmarc.example.com", want: "_dmarc.example.com"},
		{name: "IDN", raw: "bücher.example", want: "xn--bcher-kva.example"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			got, err := rules.NormalizeDomain(test.raw)
			if err != nil {
				t.Fatalf("NormalizeDomain: %v", err)
			}
			if got != test.want {
				t.Errorf("domain = %q, want %q", got, test.want)
			}
		})
	}
}

func TestNormalizeDomainRejectsInvalidInput(t *testing.T) {
	t.Parallel()

	tests := []string{
		"",
		".",
		"*.example.com",
		"https://example.com",
		"example.com/path",
		"example..com",
		"-example.com",
		"example-.com",
		"example com",
		strings.Repeat("a", 64) + ".com",
	}

	for _, input := range tests {
		t.Run(input, func(t *testing.T) {
			t.Parallel()

			_, err := rules.NormalizeDomain(input)
			if !errors.Is(err, rules.ErrInvalid) {
				t.Fatalf("error = %v, want ErrInvalid", err)
			}
		})
	}
}

func TestParseKind(t *testing.T) {
	t.Parallel()

	kind, err := rules.ParseKind("ALLOW")
	if err != nil || kind != rules.KindAllow {
		t.Errorf("ParseKind(ALLOW) = %q, %v", kind, err)
	}

	if _, err := rules.ParseKind("block"); !errors.Is(err, rules.ErrInvalid) {
		t.Errorf("ParseKind(block) error = %v, want ErrInvalid", err)
	}
}
