package blocklists

import (
	"os"
	"testing"

	"github.com/raqeebanjum/network-wide-dns-sinkhole/internal/rules"
)

func FuzzParse(f *testing.F) {
	for _, fixture := range []string{"hosts.txt", "adblock.txt", "domains.txt", "mixed.txt"} {
		data, err := os.ReadFile("testdata/" + fixture)
		if err != nil {
			f.Fatalf("read fixture: %v", err)
		}
		f.Add(data)
	}
	f.Add([]byte("0.0.0.0 one.example two.example\n||three.example^\n"))
	f.Add([]byte("\xef\xbb\xbf#\n@@||exception.example^\ninvalid/path\n"))

	f.Fuzz(func(t *testing.T, data []byte) {
		result, err := Parse(data)
		if err != nil {
			return
		}
		if len(result.Domains) == 0 {
			t.Fatal("successful parse returned zero domains")
		}
		switch result.Format {
		case "hosts", "adblock", "domains", "mixed":
		default:
			t.Fatalf("unexpected format %q", result.Format)
		}
		if result.Ignored < 0 {
			t.Fatalf("ignored count = %d", result.Ignored)
		}
		for _, domain := range result.Domains {
			normalized, err := rules.NormalizeDomain(domain)
			if err != nil {
				t.Fatalf("parsed domain %q is invalid: %v", domain, err)
			}
			if normalized != domain {
				t.Fatalf("parsed domain %q is not canonical (%q)", domain, normalized)
			}
		}
	})
}
