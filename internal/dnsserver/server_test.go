package dnsserver

import (
	"context"
	"io"
	"log/slog"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/miekg/dns"
	"github.com/raqeebanjum/network-wide-dns-sinkhole/internal/querylog"
	"github.com/raqeebanjum/network-wide-dns-sinkhole/internal/rules"
	"github.com/raqeebanjum/network-wide-dns-sinkhole/internal/testutil"
)

func TestForwardsUDPAndTCP(t *testing.T) {
	t.Parallel()

	upstream := startReplyUpstream(t, 0)
	server := startTestServer(t, []string{upstream.Address}, 2*time.Second, 8)

	for _, network := range []string{"udp", "tcp"} {
		t.Run(network, func(t *testing.T) {
			response := query(t, server.Address(), network)

			if response.Rcode != dns.RcodeSuccess {
				t.Fatalf("rcode = %s, want NOERROR", dns.RcodeToString[response.Rcode])
			}
			if len(response.Answer) != 1 {
				t.Fatalf("answer count = %d, want 1", len(response.Answer))
			}
			answer, ok := response.Answer[0].(*dns.A)
			if !ok {
				t.Fatalf("answer type = %T, want *dns.A", response.Answer[0])
			}
			if got := answer.A.String(); got != "192.0.2.20" {
				t.Errorf("answer = %s, want 192.0.2.20", got)
			}
		})
	}
}

func TestFallsBackToNextUpstream(t *testing.T) {
	t.Parallel()

	deadAddress := unusedUDPAddress(t)
	upstream := startReplyUpstream(t, 0)
	server := startTestServer(t, []string{deadAddress, upstream.Address}, 2*time.Second, 8)

	response := query(t, server.Address(), "udp")

	if response.Rcode != dns.RcodeSuccess {
		t.Fatalf("rcode = %s, want NOERROR", dns.RcodeToString[response.Rcode])
	}
	if len(response.Answer) != 1 {
		t.Fatalf("answer count = %d, want 1", len(response.Answer))
	}
}

func TestReturnsNXDOMAINForDeniedRuleWithoutCallingUpstream(t *testing.T) {
	t.Parallel()

	var upstreamCalls atomic.Int32
	upstream := startUpstream(t, dns.HandlerFunc(func(w dns.ResponseWriter, request *dns.Msg) {
		upstreamCalls.Add(1)
		writeAReply(w, request)
	}))
	matcher := rules.NewMatcher([]rules.Rule{
		{ID: 1, Domain: "blocked.example", Kind: rules.KindDeny},
	}, nil)
	server := startTestServerWithMatcher(
		t,
		[]string{upstream.Address},
		2*time.Second,
		8,
		matcher,
	)

	blocked, err := exchangeName(server.Address(), "udp", "ads.blocked.example.")
	if err != nil {
		t.Fatalf("query blocked name: %v", err)
	}
	if blocked.Rcode != dns.RcodeNameError {
		t.Fatalf("blocked rcode = %s, want NXDOMAIN", dns.RcodeToString[blocked.Rcode])
	}
	if !blocked.RecursionAvailable {
		t.Error("blocked response does not set recursion available")
	}
	if calls := upstreamCalls.Load(); calls != 0 {
		t.Errorf("upstream calls after blocked query = %d, want 0", calls)
	}

	allowed, err := exchangeName(server.Address(), "udp", "allowed.example.")
	if err != nil {
		t.Fatalf("query allowed name: %v", err)
	}
	if allowed.Rcode != dns.RcodeSuccess {
		t.Fatalf("allowed rcode = %s, want NOERROR", dns.RcodeToString[allowed.Rcode])
	}
	if calls := upstreamCalls.Load(); calls != 1 {
		t.Errorf("upstream calls after allowed query = %d, want 1", calls)
	}
}

func TestRecordsEachQueryWithDecision(t *testing.T) {
	t.Parallel()

	upstream := startReplyUpstream(t, 0)
	matcher := rules.NewMatcher([]rules.Rule{
		{ID: 7, Domain: "blocked.example", Kind: rules.KindDeny},
	}, nil)
	sink := &recordingQuerySink{}
	server := startTestServerFull(
		t,
		[]string{upstream.Address},
		2*time.Second,
		8,
		matcher,
		sink,
	)

	if _, err := exchangeName(server.Address(), "udp", "ads.blocked.example."); err != nil {
		t.Fatalf("query blocked name: %v", err)
	}
	if _, err := exchangeName(server.Address(), "tcp", "example.test."); err != nil {
		t.Fatalf("query forwarded name: %v", err)
	}

	events := sink.Events()
	if len(events) != 2 {
		t.Fatalf("recorded events = %d, want 2", len(events))
	}

	blocked := events[0]
	if blocked.Outcome != querylog.OutcomeBlocked {
		t.Errorf("blocked outcome = %q", blocked.Outcome)
	}
	if blocked.Domain != "ads.blocked.example" || blocked.QueryType != "A" {
		t.Errorf("blocked event domain/type = %q/%q", blocked.Domain, blocked.QueryType)
	}
	if blocked.ClientIP != "127.0.0.1" {
		t.Errorf("blocked client = %q, want 127.0.0.1", blocked.ClientIP)
	}
	if blocked.RuleID != 7 || blocked.RuleKind != "deny" || blocked.RuleDomain != "blocked.example" {
		t.Errorf("blocked rule metadata = %#v", blocked)
	}
	if blocked.ResponseCode != "NXDOMAIN" || blocked.Upstream != "" {
		t.Errorf("blocked response/upstream = %q/%q", blocked.ResponseCode, blocked.Upstream)
	}
	if blocked.DurationMS < 0 || blocked.Timestamp.IsZero() {
		t.Errorf("blocked timing metadata = %#v", blocked)
	}

	forwarded := events[1]
	if forwarded.Outcome != querylog.OutcomeForwarded {
		t.Errorf("forwarded outcome = %q", forwarded.Outcome)
	}
	if forwarded.Domain != "example.test" || forwarded.ResponseCode != "NOERROR" {
		t.Errorf("forwarded event = %#v", forwarded)
	}
	if forwarded.Upstream != upstream.Address {
		t.Errorf("forwarded upstream = %q, want %q", forwarded.Upstream, upstream.Address)
	}
	if forwarded.RuleID != 0 || forwarded.RuleKind != "" {
		t.Errorf("forwarded event unexpectedly has rule metadata: %#v", forwarded)
	}
}

func TestRecordsBlocklistSource(t *testing.T) {
	t.Parallel()

	upstream := startReplyUpstream(t, 0)
	matcher := rules.NewMatcher(nil, []rules.BlocklistDomain{
		{Domain: "listed.example", BlocklistID: 12, BlocklistName: "Test List"},
	})
	sink := &recordingQuerySink{}
	server := startTestServerFull(
		t,
		[]string{upstream.Address},
		2*time.Second,
		8,
		matcher,
		sink,
	)

	response, err := exchangeName(server.Address(), "udp", "ads.listed.example.")
	if err != nil {
		t.Fatalf("query blocklisted name: %v", err)
	}
	if response.Rcode != dns.RcodeNameError {
		t.Fatalf("rcode = %s, want NXDOMAIN", dns.RcodeToString[response.Rcode])
	}

	events := sink.Events()
	if len(events) != 1 {
		t.Fatalf("recorded events = %d, want 1", len(events))
	}
	event := events[0]
	if event.Outcome != querylog.OutcomeBlocked {
		t.Errorf("outcome = %q", event.Outcome)
	}
	if event.BlocklistID != 12 || event.BlocklistName != "Test List" {
		t.Errorf("blocklist source = %d/%q", event.BlocklistID, event.BlocklistName)
	}
	if event.RuleID != 0 || event.RuleKind != "" {
		t.Errorf("blocklist event unexpectedly has a manual rule: %#v", event)
	}
}

func TestReturnsSERVFAILWhenUpstreamTimesOut(t *testing.T) {
	t.Parallel()

	upstream := startReplyUpstream(t, 250*time.Millisecond)
	server := startTestServer(t, []string{upstream.Address}, 50*time.Millisecond, 8)

	response := query(t, server.Address(), "udp")

	if response.Rcode != dns.RcodeServerFailure {
		t.Fatalf("rcode = %s, want SERVFAIL", dns.RcodeToString[response.Rcode])
	}
	if !response.RecursionAvailable {
		t.Error("SERVFAIL response does not set recursion available")
	}
}

func TestLimitsConcurrentUpstreamRequests(t *testing.T) {
	t.Parallel()

	var active atomic.Int32
	var maxActive atomic.Int32

	upstream := startUpstream(t, dns.HandlerFunc(func(w dns.ResponseWriter, request *dns.Msg) {
		current := active.Add(1)
		for {
			observed := maxActive.Load()
			if current <= observed || maxActive.CompareAndSwap(observed, current) {
				break
			}
		}
		time.Sleep(100 * time.Millisecond)
		active.Add(-1)
		writeAReply(w, request)
	}))
	server := startTestServer(t, []string{upstream.Address}, 2*time.Second, 1)

	var waitGroup sync.WaitGroup
	for range 2 {
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			response, err := exchange(server.Address(), "udp")
			if err != nil {
				t.Errorf("query DNS server: %v", err)
				return
			}
			if response.Rcode != dns.RcodeSuccess {
				t.Errorf("rcode = %s, want NOERROR", dns.RcodeToString[response.Rcode])
			}
		}()
	}
	waitGroup.Wait()

	if got := maxActive.Load(); got != 1 {
		t.Errorf("maximum concurrent upstream requests = %d, want 1", got)
	}
}

func TestNewRejectsInvalidConfig(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		cfg  Config
	}{
		{
			name: "missing listen address",
			cfg:  Config{Upstreams: []string{"127.0.0.1:53"}, RequestTimeout: time.Second, MaxInflight: 1},
		},
		{
			name: "missing upstream",
			cfg:  Config{Listen: "127.0.0.1:0", RequestTimeout: time.Second, MaxInflight: 1},
		},
		{
			name: "missing timeout",
			cfg:  Config{Listen: "127.0.0.1:0", Upstreams: []string{"127.0.0.1:53"}, MaxInflight: 1},
		},
		{
			name: "missing concurrency limit",
			cfg:  Config{Listen: "127.0.0.1:0", Upstreams: []string{"127.0.0.1:53"}, RequestTimeout: time.Second},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			if _, err := New(test.cfg); err == nil {
				t.Fatal("New returned nil error")
			}
		})
	}
}

func startReplyUpstream(t *testing.T, delay time.Duration) *testutil.DNSUpstream {
	t.Helper()

	return startUpstream(t, dns.HandlerFunc(func(w dns.ResponseWriter, request *dns.Msg) {
		if delay > 0 {
			time.Sleep(delay)
		}
		writeAReply(w, request)
	}))
}

func startUpstream(t *testing.T, handler dns.Handler) *testutil.DNSUpstream {
	t.Helper()

	upstream, err := testutil.StartDNSUpstream(handler)
	if err != nil {
		t.Fatalf("start fake upstream: %v", err)
	}
	t.Cleanup(func() {
		if err := upstream.Close(); err != nil {
			t.Errorf("close fake upstream: %v", err)
		}
	})
	return upstream
}

func startTestServer(t *testing.T, upstreams []string, timeout time.Duration, maxInflight int) *Server {
	t.Helper()

	return startTestServerFull(t, upstreams, timeout, maxInflight, nil, nil)
}

func startTestServerWithMatcher(
	t *testing.T,
	upstreams []string,
	timeout time.Duration,
	maxInflight int,
	matcher RuleMatcher,
) *Server {
	t.Helper()

	return startTestServerFull(t, upstreams, timeout, maxInflight, matcher, nil)
}

func startTestServerFull(
	t *testing.T,
	upstreams []string,
	timeout time.Duration,
	maxInflight int,
	matcher RuleMatcher,
	queryLog QuerySink,
) *Server {
	t.Helper()

	server, err := New(Config{
		Listen:         "127.0.0.1:0",
		Upstreams:      upstreams,
		RequestTimeout: timeout,
		MaxInflight:    maxInflight,
		Matcher:        matcher,
		QueryLog:       queryLog,
		Logger:         slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		t.Fatalf("create DNS server: %v", err)
	}
	if err := server.Start(); err != nil {
		t.Fatalf("start DNS server: %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if err := server.Shutdown(ctx); err != nil {
			t.Errorf("shut down DNS server: %v", err)
		}
	})
	return server
}

func query(t *testing.T, address, network string) *dns.Msg {
	t.Helper()

	response, err := exchange(address, network)
	if err != nil {
		t.Fatalf("query DNS server over %s: %v", network, err)
	}
	return response
}

func exchange(address, network string) (*dns.Msg, error) {
	return exchangeName(address, network, "www.example.test.")
}

func exchangeName(address, network, name string) (*dns.Msg, error) {
	request := new(dns.Msg)
	request.SetQuestion(name, dns.TypeA)
	client := &dns.Client{Net: network, Timeout: 2 * time.Second}
	response, _, err := client.Exchange(request, address)
	return response, err
}

func writeAReply(w dns.ResponseWriter, request *dns.Msg) {
	reply := new(dns.Msg)
	reply.SetReply(request)
	reply.RecursionAvailable = true
	reply.Answer = append(reply.Answer, &dns.A{
		Hdr: dns.RR_Header{
			Name:   request.Question[0].Name,
			Rrtype: dns.TypeA,
			Class:  dns.ClassINET,
			Ttl:    60,
		},
		A: net.ParseIP("192.0.2.20").To4(),
	})
	_ = w.WriteMsg(reply)
}

type recordingQuerySink struct {
	mu     sync.Mutex
	events []querylog.Event
}

func (s *recordingQuerySink) Record(event querylog.Event) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, event)
}

func (s *recordingQuerySink) Events() []querylog.Event {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]querylog.Event(nil), s.events...)
}

func unusedUDPAddress(t *testing.T) string {
	t.Helper()

	conn, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve unused UDP address: %v", err)
	}
	address := conn.LocalAddr().String()
	if err := conn.Close(); err != nil {
		t.Fatalf("release unused UDP address: %v", err)
	}
	return address
}
