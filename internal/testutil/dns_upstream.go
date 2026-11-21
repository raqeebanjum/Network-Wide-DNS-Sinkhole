// Package testutil provides local test servers that keep unit tests off the
// public network.
package testutil

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/miekg/dns"
)

// DNSUpstream is a local UDP and TCP DNS server sharing one address.
type DNSUpstream struct {
	Address string

	udp       *dns.Server
	tcp       *dns.Server
	wg        sync.WaitGroup
	closeOnce sync.Once
	closeErr  error
}

// StartDNSUpstream starts the handler on an ephemeral loopback port for both
// UDP and TCP. Callers must close the returned server.
func StartDNSUpstream(handler dns.Handler) (*DNSUpstream, error) {
	if handler == nil {
		return nil, errors.New("DNS upstream handler must not be nil")
	}

	packetConn, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("listen for test DNS UDP: %w", err)
	}

	address := packetConn.LocalAddr().String()
	listener, err := net.Listen("tcp", address)
	if err != nil {
		_ = packetConn.Close()
		return nil, fmt.Errorf("listen for test DNS TCP on %s: %w", address, err)
	}

	upstream := &DNSUpstream{
		Address: address,
		udp:     &dns.Server{PacketConn: packetConn, Handler: handler},
		tcp:     &dns.Server{Listener: listener, Handler: handler},
	}

	upstream.wg.Add(2)
	go func() {
		defer upstream.wg.Done()
		_ = upstream.udp.ActivateAndServe()
	}()
	go func() {
		defer upstream.wg.Done()
		_ = upstream.tcp.ActivateAndServe()
	}()

	return upstream, nil
}

// Close stops both listeners. It is safe to call more than once.
func (u *DNSUpstream) Close() error {
	u.closeOnce.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		u.closeErr = errors.Join(
			u.udp.ShutdownContext(ctx),
			u.tcp.ShutdownContext(ctx),
		)
		u.wg.Wait()
	})
	return u.closeErr
}
