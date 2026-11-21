package dnsserver

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/miekg/dns"
	"github.com/raqeebanjum/network-wide-dns-sinkhole/internal/rules"
	"github.com/raqeebanjum/network-wide-dns-sinkhole/internal/testutil"
)

func TestMalformedPacketsDoNotStopServer(t *testing.T) {
	t.Parallel()

	upstream := startReplyUpstream(t, 0)
	server := startTestServer(t, []string{upstream.Address}, 2*time.Second, 8)

	udpConn, err := net.DialTimeout("udp", server.Address(), time.Second)
	if err != nil {
		t.Fatalf("dial test DNS UDP: %v", err)
	}
	payloads := [][]byte{
		{},
		{0x00},
		{0x01, 0x02, 0x03, 0x04, 0x05},
		make([]byte, 9000),
	}
	for _, payload := range payloads {
		_ = udpConn.SetWriteDeadline(time.Now().Add(time.Second))
		if _, err := udpConn.Write(payload); err != nil {
			t.Logf("write malformed UDP payload (%d bytes): %v", len(payload), err)
		}
	}
	_ = udpConn.Close()

	tcpConn, err := net.DialTimeout("tcp", server.Address(), time.Second)
	if err != nil {
		t.Fatalf("dial test DNS TCP: %v", err)
	}
	_, _ = tcpConn.Write([]byte{0x00, 0x04, 0xde, 0xad, 0xbe, 0xef})
	_ = tcpConn.Close()

	response, err := exchangeName(server.Address(), "udp", "healthy.example.")
	if err != nil {
		t.Fatalf("query after malformed packets: %v", err)
	}
	if response.Rcode != dns.RcodeSuccess {
		t.Fatalf("rcode = %s, want NOERROR", dns.RcodeToString[response.Rcode])
	}
}

func TestLargeResponsesTruncateOverUDP(t *testing.T) {
	t.Parallel()

	const records = 30
	upstream := startUpstream(t, dns.HandlerFunc(func(w dns.ResponseWriter, request *dns.Msg) {
		reply := new(dns.Msg)
		reply.SetReply(request)
		reply.RecursionAvailable = true
		for index := range records {
			reply.Answer = append(reply.Answer, &dns.TXT{
				Hdr: dns.RR_Header{
					Name:   request.Question[0].Name,
					Rrtype: dns.TypeTXT,
					Class:  dns.ClassINET,
					Ttl:    60,
				},
				Txt: []string{fmt.Sprintf("%02d-%s", index, strings.Repeat("x", 90))},
			})
		}
		_ = w.WriteMsg(reply)
	}))
	server := startTestServer(t, []string{upstream.Address}, 2*time.Second, 8)

	plainRequest := new(dns.Msg)
	plainRequest.SetQuestion("large.example.", dns.TypeTXT)
	plainClient := &dns.Client{Net: "udp", Timeout: 2 * time.Second}
	plainResponse, _, err := plainClient.Exchange(plainRequest, server.Address())
	if err != nil {
		t.Fatalf("plain UDP query: %v", err)
	}
	if !plainResponse.Truncated {
		t.Fatalf("large UDP response was not truncated: %d answers", len(plainResponse.Answer))
	}

	tcpResponse, err := exchangeType(server.Address(), "tcp", "large.example.", dns.TypeTXT)
	if err != nil {
		t.Fatalf("TCP query: %v", err)
	}
	if tcpResponse.Truncated || len(tcpResponse.Answer) != records {
		t.Fatalf("TCP response = %d answers, truncated=%v", len(tcpResponse.Answer), tcpResponse.Truncated)
	}

	ednsRequest := new(dns.Msg)
	ednsRequest.SetQuestion("large.example.", dns.TypeTXT)
	ednsRequest.SetEdns0(4096, false)
	ednsClient := &dns.Client{Net: "udp", Timeout: 2 * time.Second, UDPSize: 4096}
	ednsResponse, _, err := ednsClient.Exchange(ednsRequest, server.Address())
	if err != nil {
		t.Fatalf("EDNS UDP query: %v", err)
	}
	if ednsResponse.Truncated || len(ednsResponse.Answer) != records {
		t.Fatalf("EDNS response = %d answers, truncated=%v", len(ednsResponse.Answer), ednsResponse.Truncated)
	}
}

func TestConcurrentQueries(t *testing.T) {
	t.Parallel()

	var upstreamCalls atomic.Int64
	upstream := startUpstream(t, dns.HandlerFunc(func(w dns.ResponseWriter, request *dns.Msg) {
		upstreamCalls.Add(1)
		time.Sleep(time.Millisecond)
		writeAReply(w, request)
	}))
	server := startTestServer(t, []string{upstream.Address}, 3*time.Second, 32)

	const (
		workers = 12
		perWork = 6
	)
	errs := make(chan error, workers*perWork)
	var waitGroup sync.WaitGroup
	for worker := range workers {
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			network := "udp"
			if worker%2 == 1 {
				network = "tcp"
			}
			for query := range perWork {
				name := fmt.Sprintf("q-%d-%d.example.", worker, query)
				response, err := exchangeName(server.Address(), network, name)
				if err != nil {
					errs <- fmt.Errorf("%s query %s: %w", network, name, err)
					continue
				}
				if response.Rcode != dns.RcodeSuccess {
					errs <- fmt.Errorf("%s query %s returned %s", network, name, dns.RcodeToString[response.Rcode])
				}
			}
		}()
	}
	waitGroup.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}

	if got := upstreamCalls.Load(); got != workers*perWork {
		t.Errorf("upstream calls = %d, want %d", got, workers*perWork)
	}
}

type transientPanicMatcher struct {
	panicOnce atomic.Bool
}

func (m *transientPanicMatcher) Match(string) (rules.Match, bool) {
	if m.panicOnce.CompareAndSwap(false, true) {
		panic("simulated matcher bug")
	}
	return rules.Match{}, false
}

func TestPanicInMatcherReturnsSERVFAILAndKeepsServing(t *testing.T) {
	t.Parallel()

	upstream := startReplyUpstream(t, 0)
	matcher := &transientPanicMatcher{}
	server := startTestServerWithMatcher(t, []string{upstream.Address}, 2*time.Second, 8, matcher)

	response, err := exchangeName(server.Address(), "udp", "panic.example.")
	if err != nil {
		t.Fatalf("panic query: %v", err)
	}
	if response.Rcode != dns.RcodeServerFailure {
		t.Fatalf("panic rcode = %s, want SERVFAIL", dns.RcodeToString[response.Rcode])
	}

	response, err = exchangeName(server.Address(), "udp", "healthy.example.")
	if err != nil {
		t.Fatalf("query after panic: %v", err)
	}
	if response.Rcode != dns.RcodeSuccess {
		t.Fatalf("post-panic rcode = %s, want NOERROR", dns.RcodeToString[response.Rcode])
	}
}

func exchangeType(address, network, name string, queryType uint16) (*dns.Msg, error) {
	request := new(dns.Msg)
	request.SetQuestion(name, queryType)
	client := &dns.Client{Net: network, Timeout: 2 * time.Second}
	response, _, err := client.Exchange(request, address)
	return response, err
}

func BenchmarkDNSForwardUDP(b *testing.B) {
	upstream, err := testutil.StartDNSUpstream(dns.HandlerFunc(func(w dns.ResponseWriter, request *dns.Msg) {
		writeAReply(w, request)
	}))
	if err != nil {
		b.Fatalf("start upstream: %v", err)
	}
	b.Cleanup(func() { _ = upstream.Close() })

	server, err := New(Config{
		Listen:         "127.0.0.1:0",
		Upstreams:      []string{upstream.Address},
		RequestTimeout: 2 * time.Second,
		MaxInflight:    64,
		Logger:         slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		b.Fatalf("create server: %v", err)
	}
	if err := server.Start(); err != nil {
		b.Fatalf("start server: %v", err)
	}
	b.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = server.Shutdown(ctx)
	})

	client := &dns.Client{Net: "udp", Timeout: 2 * time.Second}
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		request := new(dns.Msg)
		request.SetQuestion("benchmark.example.", dns.TypeA)
		if _, _, err := client.Exchange(request, server.Address()); err != nil {
			b.Fatalf("query: %v", err)
		}
	}
}
