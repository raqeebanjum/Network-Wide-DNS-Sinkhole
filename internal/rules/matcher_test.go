package rules_test

import (
	"testing"

	"github.com/raqeebanjum/network-wide-dns-sinkhole/internal/rules"
)

func TestMatcherUsesLabelBoundaries(t *testing.T) {
	t.Parallel()

	matcher := rules.NewMatcher([]rules.Rule{
		{ID: 1, Domain: "doubleclick.net", Kind: rules.KindDeny},
	})

	tests := []struct {
		name      string
		domain    string
		wantMatch bool
	}{
		{name: "exact", domain: "doubleclick.net", wantMatch: true},
		{name: "subdomain", domain: "ads.doubleclick.net", wantMatch: true},
		{name: "deep subdomain", domain: "x.ads.doubleclick.net", wantMatch: true},
		{name: "wire name", domain: "ADS.DoubleClick.NET.", wantMatch: true},
		{name: "lookalike suffix", domain: "notdoubleclick.net", wantMatch: false},
		{name: "different domain", domain: "doubleclick.net.example.com", wantMatch: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			match, ok := matcher.Match(test.domain)
			if ok != test.wantMatch {
				t.Fatalf("match = %v, want %v", ok, test.wantMatch)
			}
			if ok && match.Rule.ID != 1 {
				t.Errorf("matched rule ID = %d, want 1", match.Rule.ID)
			}
		})
	}
}

func TestMatcherAllowOverridesDeny(t *testing.T) {
	t.Parallel()

	matcher := rules.NewMatcher([]rules.Rule{
		{ID: 1, Domain: "example.com", Kind: rules.KindDeny},
		{ID: 2, Domain: "safe.example.com", Kind: rules.KindAllow},
		{ID: 3, Domain: "home.example.com", Kind: rules.KindAllow},
		{ID: 4, Domain: "ads.home.example.com", Kind: rules.KindDeny},
	})

	match, ok := matcher.Match("safe.example.com")
	if !ok || match.Rule.Kind != rules.KindAllow || match.Rule.ID != 2 {
		t.Fatalf("safe.example.com match = %#v, %v", match, ok)
	}

	match, ok = matcher.Match("ads.home.example.com")
	if !ok || match.Rule.Kind != rules.KindAllow {
		t.Fatalf("allow did not override more-specific deny: %#v, %v", match, ok)
	}

	match, ok = matcher.Match("other.example.com")
	if !ok || match.Rule.Kind != rules.KindDeny || match.Rule.ID != 1 {
		t.Fatalf("other.example.com match = %#v, %v", match, ok)
	}
}

func TestMatcherReturnsMostSpecificDeny(t *testing.T) {
	t.Parallel()

	matcher := rules.NewMatcher([]rules.Rule{
		{ID: 1, Domain: "example.com", Kind: rules.KindDeny},
		{ID: 2, Domain: "ads.example.com", Kind: rules.KindDeny},
	})

	match, ok := matcher.Match("tracker.ads.example.com")
	if !ok || match.Rule.ID != 2 {
		t.Fatalf("match = %#v, %v; want rule 2", match, ok)
	}
	if matcher.Count() != 2 {
		t.Errorf("Count = %d, want 2", matcher.Count())
	}
}
