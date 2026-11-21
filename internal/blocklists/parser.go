package blocklists

import (
	"bufio"
	"bytes"
	"fmt"
	"net"
	"sort"
	"strings"
	"unicode"

	"github.com/raqeebanjum/network-wide-dns-sinkhole/internal/rules"
)

const (
	maxLineSize = 1 << 20
	maxDomains  = 2_000_000
)

var ignoredHostNames = map[string]struct{}{
	"0.0.0.0":                 {},
	"local":                   {},
	"localhost":               {},
	"localhost.localdomain":   {},
	"localhost6":              {},
	"localhost6.localdomain6": {},
	"broadcasthost":           {},
	"ip6-localhost":           {},
	"ip6-loopback":            {},
	"ip6-localnet":            {},
	"ip6-mcastprefix":         {},
	"ip6-allnodes":            {},
	"ip6-allrouters":          {},
	"ip6-allhosts":            {},
}

// Parse extracts supported domains from hosts, plain-domain, or domain-anchored
// Adblock list content. Unsupported lines are counted and skipped.
func Parse(data []byte) (ParseResult, error) {
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 64*1024), maxLineSize)

	domains := make(map[string]struct{})
	ignored := 0
	hasHosts := false
	hasAdblock := false
	hasPlain := false

	for scanner.Scan() {
		line := strings.TrimPrefix(scanner.Text(), "\uFEFF")
		line = strings.TrimSpace(line)
		if line == "" ||
			strings.HasPrefix(line, "#") ||
			strings.HasPrefix(line, "!") ||
			strings.HasPrefix(line, "[") {
			continue
		}

		line = stripInlineComment(line)
		if line == "" {
			continue
		}

		if strings.HasPrefix(line, "@@") {
			ignored++
			continue
		}

		if strings.HasPrefix(line, "||") {
			domain, ok := parseAdblockDomain(line)
			if !ok {
				ignored++
				continue
			}
			addDomain(domains, domain)
			hasAdblock = true
			if len(domains) > maxDomains {
				return ParseResult{}, fmt.Errorf("list exceeds %d domains", maxDomains)
			}
			continue
		}

		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		if net.ParseIP(fields[0]) != nil {
			hasHosts = true
			for _, field := range fields[1:] {
				if _, skip := ignoredHostNames[strings.ToLower(field)]; skip {
					continue
				}
				domain, err := rules.NormalizeDomain(field)
				if err != nil {
					ignored++
					continue
				}
				addDomain(domains, domain)
			}
			if len(domains) > maxDomains {
				return ParseResult{}, fmt.Errorf("list exceeds %d domains", maxDomains)
			}
			continue
		}

		if len(fields) != 1 {
			ignored++
			continue
		}

		domain, err := rules.NormalizeDomain(fields[0])
		if err != nil {
			ignored++
			continue
		}
		if _, skip := ignoredHostNames[domain]; skip {
			continue
		}
		addDomain(domains, domain)
		hasPlain = true
		if len(domains) > maxDomains {
			return ParseResult{}, fmt.Errorf("list exceeds %d domains", maxDomains)
		}
	}
	if err := scanner.Err(); err != nil {
		return ParseResult{}, fmt.Errorf("scan blocklist: %w", err)
	}
	if len(domains) == 0 {
		return ParseResult{}, fmt.Errorf("list contains no supported domains")
	}

	result := make([]string, 0, len(domains))
	for domain := range domains {
		result = append(result, domain)
	}
	sort.Strings(result)

	formats := 0
	if hasHosts {
		formats++
	}
	if hasAdblock {
		formats++
	}
	if hasPlain {
		formats++
	}

	format := "domains"
	switch {
	case formats > 1:
		format = "mixed"
	case hasHosts:
		format = "hosts"
	case hasAdblock:
		format = "adblock"
	}

	return ParseResult{Domains: result, Format: format, Ignored: ignored}, nil
}

func parseAdblockDomain(line string) (string, bool) {
	if !strings.HasSuffix(line, "^") {
		return "", false
	}
	body := strings.TrimSuffix(strings.TrimPrefix(line, "||"), "^")
	if body == "" || strings.ContainsAny(body, "*/$|^") {
		return "", false
	}
	domain, err := rules.NormalizeDomain(body)
	if err != nil {
		return "", false
	}
	if _, skip := ignoredHostNames[domain]; skip {
		return "", false
	}
	return domain, true
}

func addDomain(domains map[string]struct{}, domain string) {
	domains[domain] = struct{}{}
}

func stripInlineComment(line string) string {
	for index := 1; index < len(line); index++ {
		if line[index] == '#' && unicode.IsSpace(rune(line[index-1])) {
			return strings.TrimSpace(line[:index])
		}
	}
	return line
}
