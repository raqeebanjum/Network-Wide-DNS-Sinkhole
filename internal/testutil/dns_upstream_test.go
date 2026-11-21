package testutil

import (
	"net"
	"testing"
	"time"

	"github.com/miekg/dns"
)

func TestDNSUpstreamServesUDPAndTCP(t *testing.T) {
	t.Parallel()

	upstream, err := StartDNSUpstream(dns.HandlerFunc(func(w dns.ResponseWriter, request *dns.Msg) {
		reply := new(dns.Msg)
		reply.SetReply(request)
		reply.Answer = append(reply.Answer, &dns.A{
			Hdr: dns.RR_Header{
				Name:   request.Question[0].Name,
				Rrtype: dns.TypeA,
				Class:  dns.ClassINET,
				Ttl:    60,
			},
			A: net.ParseIP("192.0.2.10").To4(),
		})
		_ = w.WriteMsg(reply)
	}))
	if err != nil {
		t.Fatalf("start DNS upstream: %v", err)
	}
	t.Cleanup(func() {
		if err := upstream.Close(); err != nil {
			t.Errorf("close DNS upstream: %v", err)
		}
	})

	query := new(dns.Msg)
	query.SetQuestion(dns.Fqdn("example.test"), dns.TypeA)

	for _, network := range []string{"udp", "tcp"} {
		t.Run(network, func(t *testing.T) {
			client := &dns.Client{Net: network, Timeout: 2 * time.Second}
			response, _, err := client.Exchange(query, upstream.Address)
			if err != nil {
				t.Fatalf("exchange over %s: %v", network, err)
			}
			if len(response.Answer) != 1 {
				t.Fatalf("answer count = %d, want 1", len(response.Answer))
			}
			answer, ok := response.Answer[0].(*dns.A)
			if !ok {
				t.Fatalf("answer type = %T, want *dns.A", response.Answer[0])
			}
			if got := answer.A.String(); got != "192.0.2.10" {
				t.Errorf("answer = %s, want 192.0.2.10", got)
			}
		})
	}
}
