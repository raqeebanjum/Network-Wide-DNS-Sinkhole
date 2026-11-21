package rules

import "strings"

// BlocklistDomain associates one blocked domain with its source list.
type BlocklistDomain struct {
	Domain        string
	BlocklistID   int64
	BlocklistName string
}

// Match describes the rule or blocklist entry that determined a domain's
// classification.
type Match struct {
	Rule      Rule
	Blocklist BlocklistDomain
}

// Blocks reports whether the match should produce an NXDOMAIN response.
func (m Match) Blocks() bool {
	return m.Rule.Kind == KindDeny || m.Blocklist.Domain != ""
}

// Matcher is an immutable snapshot of all active rules and blocklist domains.
type Matcher struct {
	allow      map[string]Rule
	deny       map[string]Rule
	blocklists map[string]BlocklistDomain
}

// NewMatcher compiles rules into a label-aware suffix matcher.
func NewMatcher(entries []Rule, blocklistEntries []BlocklistDomain) *Matcher {
	matcher := &Matcher{
		allow:      make(map[string]Rule),
		deny:       make(map[string]Rule),
		blocklists: make(map[string]BlocklistDomain),
	}
	for _, rule := range entries {
		switch rule.Kind {
		case KindAllow:
			matcher.allow[rule.Domain] = rule
		case KindDeny:
			matcher.deny[rule.Domain] = rule
		}
	}
	for _, entry := range blocklistEntries {
		if _, exists := matcher.blocklists[entry.Domain]; !exists {
			matcher.blocklists[entry.Domain] = entry
		}
	}
	return matcher
}

// Match finds an allow or deny rule for name or one of its parent domains.
// Any matching allow rule takes precedence over every deny rule.
func (m *Matcher) Match(name string) (Match, bool) {
	domain := CanonicalName(name)
	if domain == "" {
		return Match{}, false
	}

	for candidate := domain; candidate != ""; candidate = parentDomain(candidate) {
		if rule, ok := m.allow[candidate]; ok {
			return Match{Rule: rule}, true
		}
	}
	for candidate := domain; candidate != ""; candidate = parentDomain(candidate) {
		if rule, ok := m.deny[candidate]; ok {
			return Match{Rule: rule}, true
		}
	}
	for candidate := domain; candidate != ""; candidate = parentDomain(candidate) {
		if entry, ok := m.blocklists[candidate]; ok {
			return Match{Blocklist: entry}, true
		}
	}

	return Match{}, false
}

// Count returns the number of compiled rules.
func (m *Matcher) Count() int {
	return len(m.allow) + len(m.deny)
}

func parentDomain(domain string) string {
	index := strings.IndexByte(domain, '.')
	if index < 0 || index == len(domain)-1 {
		return ""
	}
	return domain[index+1:]
}
