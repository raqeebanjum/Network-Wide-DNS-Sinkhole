package blocklists

import (
	"bufio"
	"bytes"
	"fmt"
	"net"
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

// Parse extracts domains from hosts-style or plain-domain list content.
func Parse(data []byte) ([]string, error) {
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 64*1024), maxLineSize)

	domains := make([]string, 0)
	for scanner.Scan() {
		line := strings.TrimPrefix(scanner.Text(), "\uFEFF")
		line = strings.TrimSpace(line)
		if line == "" ||
			strings.HasPrefix(line, "#") ||
			strings.HasPrefix(line, "!") ||
			strings.HasPrefix(line, "[") ||
			strings.HasPrefix(line, "||") ||
			strings.HasPrefix(line, "@@") {
			continue
		}

		line = stripInlineComment(line)
		if line == "" {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		if net.ParseIP(fields[0]) != nil {
			for _, field := range fields[1:] {
				if _, skip := ignoredHostNames[strings.ToLower(field)]; skip {
					continue
				}
				domain, err := rules.NormalizeDomain(field)
				if err != nil {
					continue
				}
				domains = append(domains, domain)
			}
		} else if len(fields) == 1 {
			if _, skip := ignoredHostNames[fields[0]]; skip {
				continue
			}
			domain, err := rules.NormalizeDomain(fields[0])
			if err != nil {
				continue
			}
			domains = append(domains, domain)
		}

		if len(domains) > maxDomains {
			return nil, fmt.Errorf("list exceeds %d domains", maxDomains)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan blocklist: %w", err)
	}
	if len(domains) == 0 {
		return nil, fmt.Errorf("list contains no supported domains")
	}
	return domains, nil
}

func stripInlineComment(line string) string {
	for index := 1; index < len(line); index++ {
		if line[index] == '#' && unicode.IsSpace(rune(line[index-1])) {
			return strings.TrimSpace(line[:index])
		}
	}
	return line
}
