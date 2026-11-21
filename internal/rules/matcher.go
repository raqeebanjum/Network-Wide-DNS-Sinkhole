package rules

import "strings"

// Match describes the rule that determined a domain's classification.
type Match struct {
	Rule Rule
}

// Matcher is an immutable snapshot of all active rules.
type Matcher struct {
	allow map[string]Rule
	deny  map[string]Rule
}

// NewMatcher compiles rules into a label-aware suffix matcher.
func NewMatcher(entries []Rule) *Matcher {
	matcher := &Matcher{
		allow: make(map[string]Rule),
		deny:  make(map[string]Rule),
	}
	for _, rule := range entries {
		switch rule.Kind {
		case KindAllow:
			matcher.allow[rule.Domain] = rule
		case KindDeny:
			matcher.deny[rule.Domain] = rule
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
