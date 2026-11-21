package dnsserver

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/miekg/dns"
)

const upstreamUDPSize = 4096

type forwarder struct {
	upstreams []string
	timeout   time.Duration
	slots     chan struct{}
	next      atomic.Uint64
	udpClient *dns.Client
	tcpClient *dns.Client
}

func newForwarder(upstreams []string, timeout time.Duration, maxInflight int) *forwarder {
	return &forwarder{
		upstreams: upstreams,
		timeout:   timeout,
		slots:     make(chan struct{}, maxInflight),
		udpClient: &dns.Client{
			Net:     "udp",
			Timeout: timeout,
			UDPSize: upstreamUDPSize,
		},
		tcpClient: &dns.Client{
			Net:     "tcp",
			Timeout: timeout,
		},
	}
}

func (f *forwarder) resolve(ctx context.Context, request *dns.Msg, network string) (*dns.Msg, error) {
	requestCtx, cancel := context.WithTimeout(ctx, f.timeout)
	defer cancel()

	select {
	case f.slots <- struct{}{}:
		defer func() { <-f.slots }()
	case <-requestCtx.Done():
		return nil, fmt.Errorf("wait for an upstream slot: %w", requestCtx.Err())
	}

	client := f.udpClient
	if network == "tcp" {
		client = f.tcpClient
	}

	start := int(f.next.Add(1)-1) % len(f.upstreams)
	attemptErrors := make([]error, 0, len(f.upstreams))

	for offset := range len(f.upstreams) {
		if err := requestCtx.Err(); err != nil {
			attemptErrors = append(attemptErrors, err)
			break
		}

		upstream := f.upstreams[(start+offset)%len(f.upstreams)]
		attemptCtx, attemptCancel := f.attemptContext(requestCtx, len(f.upstreams)-offset)
		response, _, err := client.ExchangeContext(attemptCtx, request, upstream)
		attemptCancel()

		if err == nil {
			if response == nil {
				err = errors.New("upstream returned no response")
			} else {
				return response, nil
			}
		}
		attemptErrors = append(attemptErrors, fmt.Errorf("%s: %w", upstream, err))
	}

	return nil, fmt.Errorf("all upstream resolvers failed: %w", errors.Join(attemptErrors...))
}

// attemptContext reserves an equal share of the remaining request deadline for
// each unused upstream, so one unresponsive resolver cannot consume all of it.
func (f *forwarder) attemptContext(parent context.Context, remainingUpstreams int) (context.Context, context.CancelFunc) {
	deadline, ok := parent.Deadline()
	if !ok || remainingUpstreams <= 1 {
		return context.WithCancel(parent)
	}

	budget := time.Until(deadline) / time.Duration(remainingUpstreams)
	if budget <= 0 {
		return context.WithCancel(parent)
	}
	return context.WithTimeout(parent, budget)
}
