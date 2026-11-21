package blocklists

import (
	"os"
	"reflect"
	"testing"
)

func TestParseSupportedFormats(t *testing.T) {
	t.Parallel()

	tests := []struct {
		fixture string
		format  string
		domains []string
	}{
		{
			fixture: "hosts.txt",
			format:  "hosts",
			domains: []string{"ads.example.com", "tracker.example.net"},
		},
		{
			fixture: "adblock.txt",
			format:  "adblock",
			domains: []string{"ads.example", "sub.tracker.example"},
		},
		{
			fixture: "domains.txt",
			format:  "domains",
			domains: []string{"ads.example.com", "plain.example"},
		},
		{
			fixture: "mixed.txt",
			format:  "mixed",
			domains: []string{"adblock.example", "hosts.example", "plain.example"},
		},
	}

	for _, test := range tests {
		t.Run(test.fixture, func(t *testing.T) {
			t.Parallel()

			data, err := os.ReadFile("testdata/" + test.fixture)
			if err != nil {
				t.Fatalf("read fixture: %v", err)
			}
			result, err := Parse(data)
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			if result.Format != test.format {
				t.Errorf("format = %q, want %q", result.Format, test.format)
			}
			if !reflect.DeepEqual(result.Domains, test.domains) {
				t.Errorf("domains = %#v, want %#v", result.Domains, test.domains)
			}
		})
	}
}

func TestParseRejectsListWithoutSupportedDomains(t *testing.T) {
	t.Parallel()

	_, err := Parse([]byte("# only comments\n127.0.0.1 localhost\n@@||allowed.example^\n"))
	if err == nil {
		t.Fatal("Parse returned nil error")
	}
}
