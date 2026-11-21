package rules

import (
	"fmt"
	"testing"
)

func BenchmarkMatcherMatch(b *testing.B) {
	const entries = 100_000

	blocklist := make([]BlocklistDomain, 0, entries)
	for index := range entries {
		blocklist = append(blocklist, BlocklistDomain{
			Domain:        fmt.Sprintf("host-%d.zone-%d.example", index, index%1000),
			BlocklistID:   int64(index%10 + 1),
			BlocklistName: "benchmark",
		})
	}
	blocklist = append(blocklist, BlocklistDomain{
		Domain:        "tracked.example",
		BlocklistID:   1,
		BlocklistName: "benchmark",
	})

	matcher := NewMatcher([]Rule{
		{ID: 1, Domain: "manual.example", Kind: KindDeny},
		{ID: 2, Domain: "safe.manual.example", Kind: KindAllow},
	}, blocklist)

	queries := []string{
		"x.tracked.example.",
		"deep.manual.example.",
		"safe.manual.example.",
		"notlisted.example.",
	}

	b.ReportAllocs()
	b.ResetTimer()
	for index := range b.N {
		matcher.Match(queries[index%len(queries)])
	}
}
