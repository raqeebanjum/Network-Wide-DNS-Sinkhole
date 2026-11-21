package dnsserver

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/miekg/dns"
	"github.com/raqeebanjum/network-wide-dns-sinkhole/internal/querylog"
	"github.com/raqeebanjum/network-wide-dns-sinkhole/internal/rules"
)

const startTimeout = 5 * time.Second

// RuleMatcher checks a query name against the active rule snapshot.
type RuleMatcher interface {
	Match(name string) (rules.Match, bool)
}

// QuerySink accepts completed query telemetry without waiting for persistence.
type QuerySink interface {
	Record(event querylog.Event)
}

// Config describes the DNS listener and upstream forwarding behavior.
type Config struct {
	Listen         string
	Upstreams      []string
	RequestTimeout time.Duration
	MaxInflight    int
	Matcher        RuleMatcher
	QueryLog       QuerySink
	Logger         *slog.Logger
}

// Server listens for UDP and TCP DNS queries and forwards them to configured
// upstream resolvers.
type Server struct {
	config    Config
	logger    *slog.Logger
	forwarder *forwarder

	ctx         context.Context
	cancel      context.CancelFunc
	udpServer   *dns.Server
	tcpServer   *dns.Server
	serveWG     sync.WaitGroup
	startMu     sync.Mutex
	errors      chan error
	address     string
	started     bool
	shutdown    sync.Once
	shutdownErr error
}

// New validates cfg and creates a DNS server. Call Start to bind the sockets.
func New(cfg Config) (*Server, error) {
	if cfg.Listen == "" {
		return nil, errors.New("DNS listen address must not be empty")
	}
	if len(cfg.Upstreams) == 0 {
		return nil, errors.New("at least one upstream resolver is required")
	}
	if cfg.RequestTimeout <= 0 {
		return nil, errors.New("DNS request timeout must be greater than zero")
	}
	if cfg.MaxInflight <= 0 {
		return nil, errors.New("DNS max in-flight requests must be greater than zero")
	}

	logger := cfg.Logger
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(discardWriter{}, nil))
	}

	ctx, cancel := context.WithCancel(context.Background())
	return &Server{
		config:    cfg,
		logger:    logger,
		forwarder: newForwarder(cfg.Upstreams, cfg.RequestTimeout, cfg.MaxInflight),
		ctx:       ctx,
		cancel:    cancel,
		errors:    make(chan error, 2),
	}, nil
}

// Start binds the same ephemeral or configured address for UDP and TCP.
func (s *Server) Start() error {
	s.startMu.Lock()
	defer s.startMu.Unlock()

	if s.started {
		return errors.New("DNS server is already started")
	}

	packetConn, err := net.ListenPacket("udp", s.config.Listen)
	if err != nil {
		return fmt.Errorf("listen for DNS over UDP on %q: %w", s.config.Listen, err)
	}

	address := packetConn.LocalAddr().String()
	listener, err := net.Listen("tcp", address)
	if err != nil {
		_ = packetConn.Close()
		return fmt.Errorf("listen for DNS over TCP on %q: %w", address, err)
	}

	started := make(chan struct{}, 2)
	newServer := func() *dns.Server {
		return &dns.Server{
			Handler:      s,
			UDPSize:      upstreamUDPSize,
			ReadTimeout:  s.config.RequestTimeout,
			WriteTimeout: s.config.RequestTimeout,
			NotifyStartedFunc: func() {
				started <- struct{}{}
			},
		}
	}

	s.udpServer = newServer()
	s.udpServer.PacketConn = packetConn
	s.tcpServer = newServer()
	s.tcpServer.Listener = listener

	s.serveWG.Add(2)
	go s.serve("udp", s.udpServer)
	go s.serve("tcp", s.tcpServer)

	timer := time.NewTimer(startTimeout)
	defer timer.Stop()

	for range 2 {
		select {
		case <-started:
		case err := <-s.errors:
			s.cancel()
			_ = packetConn.Close()
			_ = listener.Close()
			s.serveWG.Wait()
			return fmt.Errorf("start DNS server: %w", err)
		case <-timer.C:
			s.cancel()
			_ = packetConn.Close()
			_ = listener.Close()
			s.serveWG.Wait()
			return errors.New("timed out waiting for DNS listeners to start")
		}
	}

	s.started = true
	s.address = address
	return nil
}

func (s *Server) serve(network string, server *dns.Server) {
	defer s.serveWG.Done()

	if err := server.ActivateAndServe(); err != nil {
		select {
		case s.errors <- fmt.Errorf("%s listener: %w", network, err):
		default:
			s.logger.Error("DNS listener stopped with an error", "network", network, "error", err)
		}
	}
}

// ServeDNS forwards one query and converts forwarding failures into SERVFAIL.
// It implements dns.Handler.
func (s *Server) ServeDNS(w dns.ResponseWriter, request *dns.Msg) {
	defer func() {
		if recovered := recover(); recovered != nil {
			s.logger.Error("panic while handling DNS query", "panic", fmt.Sprint(recovered))
			s.writeRcode(w, request, dns.RcodeServerFailure)
		}
	}()

	startedAt := time.Now()

	if request.Opcode != dns.OpcodeQuery {
		s.writeRcode(w, request, dns.RcodeNotImplemented)
		return
	}
	if len(request.Question) != 1 {
		s.writeRcode(w, request, dns.RcodeFormatError)
		return
	}

	question := request.Question[0]
	event := querylog.Event{
		Timestamp: startedAt.UTC(),
		ClientIP:  remoteIP(w.RemoteAddr()),
		Domain:    rules.CanonicalName(question.Name),
		QueryType: queryTypeName(question.Qtype),
	}

	if s.config.Matcher != nil {
		if match, ok := s.config.Matcher.Match(question.Name); ok {
			event.RuleID = match.Rule.ID
			event.RuleKind = string(match.Rule.Kind)
			event.RuleDomain = match.Rule.Domain
			event.BlocklistID = match.Blocklist.BlocklistID
			event.BlocklistName = match.Blocklist.BlocklistName

			if match.Blocks() {
				event.Outcome = querylog.OutcomeBlocked
				event.ResponseCode = dns.RcodeToString[dns.RcodeNameError]
				s.writeRcode(w, request, dns.RcodeNameError)
				s.record(startedAt, event)
				return
			}
		}
	}

	network := "udp"
	if strings.HasPrefix(w.RemoteAddr().Network(), "tcp") {
		network = "tcp"
	}

	response, upstream, err := s.forwarder.resolve(s.ctx, request.Copy(), network)
	if err != nil {
		if s.ctx.Err() == nil {
			s.logger.Warn("DNS resolution failed", "error", err)
		}
		event.Outcome = querylog.OutcomeFailed
		event.Upstream = upstream
		event.ResponseCode = dns.RcodeToString[dns.RcodeServerFailure]
		s.writeRcode(w, request, dns.RcodeServerFailure)
		s.record(startedAt, event)
		return
	}

	response.Id = request.Id
	response.Compress = true
	event.Outcome = querylog.OutcomeForwarded
	event.Upstream = upstream
	event.ResponseCode = rcodeName(response.Rcode)
	s.writeMessage(w, request, response)
	s.record(startedAt, event)
}

func (s *Server) record(startedAt time.Time, event querylog.Event) {
	if s.config.QueryLog == nil {
		return
	}
	event.DurationMS = max(time.Since(startedAt).Milliseconds(), 0)
	s.config.QueryLog.Record(event)
}

func remoteIP(address net.Addr) string {
	if address == nil {
		return ""
	}
	host, _, err := net.SplitHostPort(address.String())
	if err != nil {
		return address.String()
	}
	return host
}

func queryTypeName(queryType uint16) string {
	if name, ok := dns.TypeToString[queryType]; ok {
		return name
	}
	return fmt.Sprintf("TYPE%d", queryType)
}

func rcodeName(rcode int) string {
	if name, ok := dns.RcodeToString[rcode]; ok {
		return name
	}
	return fmt.Sprintf("RCODE%d", rcode)
}

// Shutdown stops accepting queries, cancels in-flight upstream work, and waits
// for both listeners to stop.
func (s *Server) Shutdown(ctx context.Context) error {
	s.shutdown.Do(func() {
		s.cancel()

		var shutdownErrors []error
		if s.udpServer != nil {
			shutdownErrors = append(shutdownErrors, s.udpServer.ShutdownContext(ctx))
		}
		if s.tcpServer != nil {
			shutdownErrors = append(shutdownErrors, s.tcpServer.ShutdownContext(ctx))
		}

		s.serveWG.Wait()
		s.shutdownErr = errors.Join(shutdownErrors...)
	})
	return s.shutdownErr
}

// Address returns the bound address after Start. With an ephemeral port, the
// UDP and TCP listeners share this port.
func (s *Server) Address() string {
	return s.address
}

// Errors reports unexpected failures from either listener.
func (s *Server) Errors() <-chan error {
	return s.errors
}

func (s *Server) writeRcode(w dns.ResponseWriter, request *dns.Msg, rcode int) {
	response := new(dns.Msg)
	response.SetRcode(request, rcode)
	response.RecursionAvailable = true
	s.writeMessage(w, request, response)
}

func (s *Server) writeMessage(w dns.ResponseWriter, request, response *dns.Msg) {
	if strings.HasPrefix(w.RemoteAddr().Network(), "udp") {
		response.Truncate(udpResponseSize(request))
	}
	if err := w.WriteMsg(response); err != nil {
		s.logger.Debug("write DNS response", "error", err)
	}
}

func udpResponseSize(request *dns.Msg) int {
	size := dns.MinMsgSize
	if opt := request.IsEdns0(); opt != nil {
		size = int(opt.UDPSize())
		if size < dns.MinMsgSize {
			size = dns.MinMsgSize
		}
		if size > dns.MaxMsgSize {
			size = dns.MaxMsgSize
		}
	}
	return size
}

type discardWriter struct{}

func (discardWriter) Write(p []byte) (int, error) {
	return len(p), nil
}
