package network

import (
	"bufio"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/SgonnovDmGit/LenSA_Proxy/internal/domain/proxy"
	"github.com/elazarl/goproxy"
)

type serverTestResolver struct {
	mutex     sync.Mutex
	addresses []netip.Addr
	err       error
	networks  []string
	hosts     []string
}

func (r *serverTestResolver) LookupNetIP(_ context.Context, network, host string) ([]netip.Addr, error) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	r.networks = append(r.networks, network)
	r.hosts = append(r.hosts, host)
	return append([]netip.Addr(nil), r.addresses...), r.err
}

func (r *serverTestResolver) calls() int {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	return len(r.hosts)
}

func (r *serverTestResolver) lastHost() string {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	if len(r.hosts) == 0 {
		return ""
	}
	return r.hosts[len(r.hosts)-1]
}

type serverTestPolicy func(netip.Addr) bool

func (p serverTestPolicy) Allow(address netip.Addr) bool {
	return p(address)
}

type serverTestDialAttempt struct {
	network     string
	destination string
}

type serverTestDialer struct {
	mutex    sync.Mutex
	attempts []serverTestDialAttempt
	dial     DialContextFunc
}

func (d *serverTestDialer) DialContext(ctx context.Context, network, destination string) (net.Conn, error) {
	d.mutex.Lock()
	d.attempts = append(d.attempts, serverTestDialAttempt{network: network, destination: destination})
	dial := d.dial
	d.mutex.Unlock()
	if dial == nil {
		return nil, errors.New("unexpected dial")
	}
	return dial(ctx, network, destination)
}

func (d *serverTestDialer) calls() int {
	d.mutex.Lock()
	defer d.mutex.Unlock()
	return len(d.attempts)
}

func (d *serverTestDialer) lastDestination() string {
	d.mutex.Lock()
	defer d.mutex.Unlock()
	if len(d.attempts) == 0 {
		return ""
	}
	return d.attempts[len(d.attempts)-1].destination
}

type serverTestSourceListener struct {
	net.Listener
	remote net.Addr
}

func (l *serverTestSourceListener) Accept() (net.Conn, error) {
	connection, err := l.Listener.Accept()
	if err != nil {
		return nil, err
	}
	return &serverTestRemoteConn{Conn: connection, remote: l.remote}, nil
}

type serverTestRemoteConn struct {
	net.Conn
	remote net.Addr
}

func (c *serverTestRemoteConn) RemoteAddr() net.Addr {
	return c.remote
}

type serverTestNilResolver struct{}

func (*serverTestNilResolver) LookupNetIP(context.Context, string, string) ([]netip.Addr, error) {
	return nil, nil
}

type serverTestNilPolicy struct{}

func (*serverTestNilPolicy) Allow(netip.Addr) bool {
	return true
}

func TestNewServerValidatesConfigAndOptions(t *testing.T) {
	if _, err := NewServer(proxy.Config{}); !errors.Is(err, proxy.ErrInterfaceNameRequired) {
		t.Fatalf("NewServer() error = %v, want %v", err, proxy.ErrInterfaceNameRequired)
	}

	config := serverTestConfig(false)
	var typedNilResolver *serverTestNilResolver
	var typedNilPolicy *serverTestNilPolicy
	tests := []struct {
		name   string
		option ServerOption
	}{
		{name: "nil option", option: nil},
		{name: "nil resolver", option: WithResolver(nil)},
		{name: "typed nil resolver", option: WithResolver(typedNilResolver)},
		{name: "nil address policy", option: WithAddressPolicy(nil)},
		{name: "typed nil address policy", option: WithAddressPolicy(typedNilPolicy)},
		{name: "nil dial context", option: WithDialContext(nil)},
		{name: "empty connect ports", option: WithConnectPorts()},
		{name: "zero connect port", option: WithConnectPorts(443, 0)},
		{name: "nil listen", option: WithListen(nil)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := NewServer(config, test.option); !errors.Is(err, errInvalidServerOption) {
				t.Fatalf("NewServer() error = %v, want %v", err, errInvalidServerOption)
			}
		})
	}
}

func TestNewServerConfiguresProxyDefaults(t *testing.T) {
	resolver := &serverTestResolver{addresses: []netip.Addr{netip.MustParseAddr("93.184.216.34")}}
	dialer := &serverTestDialer{}
	server := serverTestNew(t, serverTestConfig(false), resolver, serverTestPolicy(func(netip.Addr) bool { return true }), dialer)

	if server.proxyServer.Tr != server.transport {
		t.Fatal("goproxy transport was not replaced")
	}
	if server.transport.Proxy != nil {
		t.Fatal("transport uses an upstream or environment proxy")
	}
	if server.transport.DialContext == nil {
		t.Fatal("transport DialContext is nil")
	}
	if server.transport.TLSClientConfig == nil || server.transport.TLSClientConfig.InsecureSkipVerify {
		t.Fatal("transport TLS verification is not enabled")
	}
	if !server.transport.DisableCompression {
		t.Fatal("transport compression is enabled")
	}
	if server.transport.ForceAttemptHTTP2 {
		t.Fatal("transport HTTP/2 is enabled")
	}
	if server.proxyServer.Verbose || server.proxyServer.KeepHeader || !server.proxyServer.KeepAcceptEncoding || server.proxyServer.AllowHTTP2 {
		t.Fatalf("unexpected goproxy settings: Verbose=%t KeepHeader=%t KeepAcceptEncoding=%t AllowHTTP2=%t", server.proxyServer.Verbose, server.proxyServer.KeepHeader, server.proxyServer.KeepAcceptEncoding, server.proxyServer.AllowHTTP2)
	}
	if server.proxyServer.ConnectDial != nil || server.proxyServer.ConnectDialWithReq != nil {
		t.Fatal("goproxy retained its default CONNECT dialer")
	}
	if server.httpServer.ReadHeaderTimeout != 10*time.Second {
		t.Fatalf("ReadHeaderTimeout = %v", server.httpServer.ReadHeaderTimeout)
	}
	if server.httpServer.MaxHeaderBytes != 64*1024 {
		t.Fatalf("MaxHeaderBytes = %d", server.httpServer.MaxHeaderBytes)
	}
	if server.httpServer.IdleTimeout != 30*time.Minute {
		t.Fatalf("IdleTimeout = %v", server.httpServer.IdleTimeout)
	}
	if server.Address() != "" || server.Clients() != 0 {
		t.Fatalf("new server Address=%q Clients=%d", server.Address(), server.Clients())
	}
}

func TestServerHTTPAuthenticationAndValidation(t *testing.T) {
	config := serverTestConfig(true)
	resolver := &serverTestResolver{addresses: []netip.Addr{netip.MustParseAddr("93.184.216.34")}}
	dialer := &serverTestDialer{}
	server := serverTestNew(t, config, resolver, serverTestPolicy(func(netip.Addr) bool { return true }), dialer)
	var roundTrips int
	server.proxyServer.OnRequest().DoFunc(func(request *http.Request, ctx *goproxy.ProxyCtx) (*http.Request, *http.Response) {
		ctx.RoundTripper = goproxy.RoundTripperFunc(func(request *http.Request, _ *goproxy.ProxyCtx) (*http.Response, error) {
			roundTrips++
			return serverTestUpstreamResponse(request), nil
		})
		return request, nil
	})

	t.Run("missing authentication", func(t *testing.T) {
		response, body := serverTestServeHTTP(t, server, httptest.NewRequest(http.MethodGet, "http://target.test/private", nil))
		if response.StatusCode != http.StatusProxyAuthRequired {
			t.Fatalf("StatusCode = %d, want %d", response.StatusCode, http.StatusProxyAuthRequired)
		}
		if got := response.Header.Get("Proxy-Authenticate"); got != proxyAuthenticationChallenge {
			t.Fatalf("Proxy-Authenticate = %q", got)
		}
		if strings.Contains(body, "target.test") {
			t.Fatalf("response exposes target: %q", body)
		}
	})

	t.Run("invalid authentication", func(t *testing.T) {
		request := httptest.NewRequest(http.MethodGet, "http://target.test/private", nil)
		request.Header.Set("Proxy-Authorization", serverTestBasicAuthorization("user", "wrong"))
		response, _ := serverTestServeHTTP(t, server, request)
		if response.StatusCode != http.StatusProxyAuthRequired {
			t.Fatalf("StatusCode = %d, want %d", response.StatusCode, http.StatusProxyAuthRequired)
		}
	})

	t.Run("non http scheme", func(t *testing.T) {
		request := httptest.NewRequest(http.MethodGet, "https://target.test/private", nil)
		request.Header.Set("Proxy-Authorization", serverTestBasicAuthorization("user", "password"))
		response, body := serverTestServeHTTP(t, server, request)
		if response.StatusCode != http.StatusBadRequest {
			t.Fatalf("StatusCode = %d, want %d", response.StatusCode, http.StatusBadRequest)
		}
		if strings.Contains(body, "target.test") {
			t.Fatalf("response exposes target: %q", body)
		}
	})

	t.Run("missing hostname", func(t *testing.T) {
		request := httptest.NewRequest(http.MethodGet, "http://placeholder.test/", nil)
		request.URL = &url.URL{Scheme: "http", Path: "/private"}
		request.RequestURI = "http:///private"
		request.Header.Set("Proxy-Authorization", serverTestBasicAuthorization("user", "password"))
		response, _ := serverTestServeHTTP(t, server, request)
		if response.StatusCode != http.StatusBadRequest {
			t.Fatalf("StatusCode = %d, want %d", response.StatusCode, http.StatusBadRequest)
		}
	})

	t.Run("nonproxy request", func(t *testing.T) {
		request := httptest.NewRequest(http.MethodGet, "/health", nil)
		response, body := serverTestServeHTTP(t, server, request)
		if response.StatusCode != http.StatusBadRequest {
			t.Fatalf("StatusCode = %d, want %d", response.StatusCode, http.StatusBadRequest)
		}
		if strings.Contains(strings.ToLower(body), "proxy server") {
			t.Fatalf("response contains implementation detail: %q", body)
		}
	})

	t.Run("authenticated absolute http", func(t *testing.T) {
		request := httptest.NewRequest(http.MethodGet, "http://target.test/resource", nil)
		request.Header.Set("Proxy-Authorization", serverTestBasicAuthorization("user", "password"))
		response, body := serverTestServeHTTP(t, server, request)
		if response.StatusCode != http.StatusOK || body != "upstream" {
			t.Fatalf("response = %d %q", response.StatusCode, body)
		}
	})

	if roundTrips != 1 {
		t.Fatalf("round trips = %d, want 1", roundTrips)
	}
	if dialer.calls() != 0 || resolver.calls() != 0 {
		t.Fatalf("unexpected network calls: resolve=%d dial=%d", resolver.calls(), dialer.calls())
	}
}

func TestServerHTTPSanitizesRequestAndResponseHeaders(t *testing.T) {
	server := serverTestNew(
		t,
		serverTestConfig(true),
		&serverTestResolver{},
		serverTestPolicy(func(netip.Addr) bool { return true }),
		&serverTestDialer{},
	)
	var upstreamHeader http.Header
	server.proxyServer.OnRequest().DoFunc(func(request *http.Request, ctx *goproxy.ProxyCtx) (*http.Request, *http.Response) {
		ctx.RoundTripper = goproxy.RoundTripperFunc(func(request *http.Request, _ *goproxy.ProxyCtx) (*http.Response, error) {
			upstreamHeader = request.Header.Clone()
			response := serverTestUpstreamResponse(request)
			response.Header = http.Header{
				"Connection":         {"X-Response-Hop"},
				"X-Response-Hop":     {"remove"},
				"Proxy-Authenticate": {"remove"},
				"X-End-To-End":       {"retain"},
			}
			return response, nil
		})
		return request, nil
	})

	request := httptest.NewRequest(http.MethodGet, "http://target.test/resource", nil)
	request.Header = http.Header{
		"pRoXy-AuThOrIzAtIoN": {serverTestBasicAuthorization("user", "password")},
		"Connection":          {"X-Request-Hop"},
		"X-Request-Hop":       {"remove"},
		"Accept-Encoding":     {"br"},
		"X-End-To-End":        {"retain"},
	}
	response, _ := serverTestServeHTTP(t, server, request)

	for _, name := range []string{"Proxy-Authorization", "Connection", "X-Request-Hop"} {
		if containsHeaderForTest(upstreamHeader, name) {
			t.Errorf("upstream request retained %q", name)
		}
	}
	if got := upstreamHeader.Get("Accept-Encoding"); got != "br" {
		t.Errorf("upstream Accept-Encoding = %q", got)
	}
	if got := upstreamHeader.Get("X-End-To-End"); got != "retain" {
		t.Errorf("upstream X-End-To-End = %q", got)
	}
	for _, name := range []string{"Connection", "X-Response-Hop", "Proxy-Authenticate"} {
		if containsHeaderForTest(response.Header, name) {
			t.Errorf("client response retained %q", name)
		}
	}
	if got := response.Header.Get("X-End-To-End"); got != "retain" {
		t.Errorf("response X-End-To-End = %q", got)
	}
}

func TestServerHTTPMapsSafeDialErrors(t *testing.T) {
	tests := []struct {
		name       string
		resolver   *serverTestResolver
		policy     serverTestPolicy
		dialError  error
		wantStatus int
		wantDials  int
	}{
		{
			name:       "blocked address",
			resolver:   &serverTestResolver{addresses: []netip.Addr{netip.MustParseAddr("10.1.2.3")}},
			policy:     serverTestPolicy(func(netip.Addr) bool { return false }),
			wantStatus: http.StatusForbidden,
		},
		{
			name:       "resolution failure",
			resolver:   &serverTestResolver{err: errors.New("secret resolver detail")},
			policy:     serverTestPolicy(func(netip.Addr) bool { return true }),
			wantStatus: http.StatusBadGateway,
		},
		{
			name:       "dial failure",
			resolver:   &serverTestResolver{addresses: []netip.Addr{netip.MustParseAddr("93.184.216.34")}},
			policy:     serverTestPolicy(func(netip.Addr) bool { return true }),
			dialError:  errors.New("secret dial detail"),
			wantStatus: http.StatusBadGateway,
			wantDials:  1,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dialer := &serverTestDialer{dial: func(context.Context, string, string) (net.Conn, error) {
				return nil, test.dialError
			}}
			server := serverTestNew(t, serverTestConfig(false), test.resolver, test.policy, dialer)
			request := httptest.NewRequest(http.MethodGet, "http://target.test/private?token=secret", nil)
			response, body := serverTestServeHTTP(t, server, request)
			if response.StatusCode != test.wantStatus {
				t.Fatalf("StatusCode = %d, want %d", response.StatusCode, test.wantStatus)
			}
			if strings.Contains(body, "target.test") || strings.Contains(body, "secret") || strings.Contains(body, "10.1.2.3") {
				t.Fatalf("response exposes target or error: %q", body)
			}
			if dialer.calls() != test.wantDials {
				t.Fatalf("dial calls = %d, want %d", dialer.calls(), test.wantDials)
			}
			if test.wantDials == 1 && dialer.lastDestination() != "93.184.216.34:80" {
				t.Fatalf("dial destination = %q", dialer.lastDestination())
			}
		})
	}
}

func TestServerConnectAuthenticationAuthorityAndPortPolicy(t *testing.T) {
	resolver := &serverTestResolver{addresses: []netip.Addr{netip.MustParseAddr("93.184.216.34")}}
	dialer := &serverTestDialer{}
	server := serverTestNew(t, serverTestConfig(true), resolver, serverTestPolicy(func(netip.Addr) bool { return true }), dialer)
	proxyHTTP := httptest.NewServer(server.proxyServer)
	defer proxyHTTP.Close()
	proxyAddress := serverTestHTTPServerAddress(t, proxyHTTP.URL)

	tests := []struct {
		name          string
		authority     string
		authorization string
		wantStatus    int
		wantChallenge bool
		wantClose     bool
	}{
		{name: "missing authentication", authority: "target.test:443", wantStatus: http.StatusProxyAuthRequired, wantChallenge: true, wantClose: true},
		{name: "invalid authentication", authority: "target.test:443", authorization: serverTestBasicAuthorization("user", "wrong"), wantStatus: http.StatusProxyAuthRequired, wantChallenge: true, wantClose: true},
		{name: "missing explicit port", authority: "target.test", authorization: serverTestBasicAuthorization("user", "password"), wantStatus: http.StatusBadRequest},
		{name: "disallowed port", authority: "target.test:80", authorization: serverTestBasicAuthorization("user", "password"), wantStatus: http.StatusForbidden},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			connection, _, response, body := serverTestConnect(t, proxyAddress, test.authority, test.authorization)
			defer connection.Close()
			if response.StatusCode != test.wantStatus {
				t.Fatalf("StatusCode = %d, want %d", response.StatusCode, test.wantStatus)
			}
			if got := response.Header.Get("Proxy-Authenticate"); (got != "") != test.wantChallenge {
				t.Fatalf("Proxy-Authenticate = %q", got)
			}
			if response.Close != test.wantClose {
				t.Fatalf("Close = %t, want %t", response.Close, test.wantClose)
			}
			if strings.Contains(body, "target.test") || strings.Contains(body, "password") {
				t.Fatalf("response exposes request details: %q", body)
			}
		})
	}
	if resolver.calls() != 0 || dialer.calls() != 0 {
		t.Fatalf("rejected CONNECT reached network: resolve=%d dial=%d", resolver.calls(), dialer.calls())
	}
}

func TestServerConnectUsesSafeDialerAndSanitizesAuthentication(t *testing.T) {
	resolver := &serverTestResolver{addresses: []netip.Addr{netip.MustParseAddr("93.184.216.34")}}
	peerReady := make(chan net.Conn, 1)
	dialer := &serverTestDialer{dial: func(context.Context, string, string) (net.Conn, error) {
		connection, peer := net.Pipe()
		peerReady <- peer
		return connection, nil
	}}
	server, err := NewServer(
		serverTestConfig(true),
		WithResolver(resolver),
		WithAddressPolicy(serverTestPolicy(func(netip.Addr) bool { return true })),
		WithDialContext(dialer.DialContext),
		WithConnectPorts(8443),
	)
	if err != nil {
		t.Fatalf("NewServer() error = %v", err)
	}

	var dialRequestHeader http.Header
	server.proxyServer.ConnectDialWithReq = func(request *http.Request, network, destination string) (net.Conn, error) {
		dialRequestHeader = request.Header.Clone()
		return server.transport.DialContext(request.Context(), network, destination)
	}
	proxyHTTP := httptest.NewServer(server.proxyServer)
	defer proxyHTTP.Close()

	connection, reader, response, _ := serverTestConnect(
		t,
		serverTestHTTPServerAddress(t, proxyHTTP.URL),
		"target.test:8443",
		serverTestBasicAuthorization("user", "password"),
	)
	defer connection.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("StatusCode = %d, want %d", response.StatusCode, http.StatusOK)
	}
	if resolver.lastHost() != "target.test" {
		t.Fatalf("resolved host = %q", resolver.lastHost())
	}
	if dialer.lastDestination() != "93.184.216.34:8443" {
		t.Fatalf("dial destination = %q", dialer.lastDestination())
	}
	if containsHeaderForTest(dialRequestHeader, "Proxy-Authorization") {
		t.Fatal("CONNECT dial request retained Proxy-Authorization")
	}

	peer := <-peerReady
	defer peer.Close()
	peerDone := make(chan error, 1)
	go func() {
		buffer := make([]byte, 4)
		if _, err := io.ReadFull(peer, buffer); err != nil {
			peerDone <- err
			return
		}
		if string(buffer) != "ping" {
			peerDone <- fmt.Errorf("tunnel received %q", buffer)
			return
		}
		_, err := peer.Write([]byte("pong"))
		peerDone <- err
	}()
	if err := connection.SetDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatalf("SetDeadline() error = %v", err)
	}
	if _, err := connection.Write([]byte("ping")); err != nil {
		t.Fatalf("tunnel Write() error = %v", err)
	}
	buffer := make([]byte, 4)
	if _, err := io.ReadFull(reader, buffer); err != nil {
		t.Fatalf("tunnel Read() error = %v", err)
	}
	if string(buffer) != "pong" {
		t.Fatalf("tunnel response = %q", buffer)
	}
	if err := <-peerDone; err != nil {
		t.Fatalf("target peer error = %v", err)
	}
}

func TestServerConnectDialErrorsAreGeneric(t *testing.T) {
	tests := []struct {
		name       string
		resolver   *serverTestResolver
		policy     serverTestPolicy
		dialError  error
		wantStatus int
		wantDials  int
	}{
		{
			name:       "policy rejection",
			resolver:   &serverTestResolver{addresses: []netip.Addr{netip.MustParseAddr("10.2.3.4")}},
			policy:     serverTestPolicy(func(netip.Addr) bool { return false }),
			wantStatus: http.StatusForbidden,
		},
		{
			name:       "dial failure",
			resolver:   &serverTestResolver{addresses: []netip.Addr{netip.MustParseAddr("93.184.216.34")}},
			policy:     serverTestPolicy(func(netip.Addr) bool { return true }),
			dialError:  errors.New("secret target connection detail"),
			wantStatus: http.StatusBadGateway,
			wantDials:  1,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dialer := &serverTestDialer{dial: func(context.Context, string, string) (net.Conn, error) {
				return nil, test.dialError
			}}
			server := serverTestNew(t, serverTestConfig(false), test.resolver, test.policy, dialer)
			proxyHTTP := httptest.NewServer(server.proxyServer)
			defer proxyHTTP.Close()

			connection, _, response, body := serverTestConnect(
				t,
				serverTestHTTPServerAddress(t, proxyHTTP.URL),
				"target.test:443",
				"",
			)
			defer connection.Close()
			if response.StatusCode != test.wantStatus {
				t.Fatalf("StatusCode = %d, want %d", response.StatusCode, test.wantStatus)
			}
			if strings.Contains(body, "target.test") || strings.Contains(body, "secret") || strings.Contains(body, "10.2.3.4") {
				t.Fatalf("response exposes target or error: %q", body)
			}
			if dialer.calls() != test.wantDials {
				t.Fatalf("dial calls = %d, want %d", dialer.calls(), test.wantDials)
			}
		})
	}
}

func TestServerStartFailureAndDoubleStart(t *testing.T) {
	listenErr := errors.New("bind failed")
	server, err := NewServer(
		serverTestConfig(false),
		WithResolver(&serverTestResolver{}),
		WithAddressPolicy(serverTestPolicy(func(netip.Addr) bool { return true })),
		WithDialContext((&serverTestDialer{}).DialContext),
		WithListen(func(string, string) (net.Listener, error) { return nil, listenErr }),
	)
	if err != nil {
		t.Fatalf("NewServer() error = %v", err)
	}
	if err := server.Start(); !errors.Is(err, listenErr) {
		t.Fatalf("Start() error = %v, want %v", err, listenErr)
	}
	if err := server.Start(); !errors.Is(err, errServerAlreadyStarted) {
		t.Fatalf("second Start() error = %v, want %v", err, errServerAlreadyStarted)
	}
	if server.Address() != "" || server.Clients() != 0 {
		t.Fatalf("failed server Address=%q Clients=%d", server.Address(), server.Clients())
	}
	if err := server.Stop(context.Background()); err != nil {
		t.Fatalf("Stop() after failed start error = %v", err)
	}
}

func TestServerLifecycleAddressClientsAndIdempotentStop(t *testing.T) {
	config := serverTestConfig(false)
	resolver := &serverTestResolver{addresses: []netip.Addr{netip.MustParseAddr("93.184.216.34")}}
	dialer := &serverTestDialer{}
	var listenCalls int
	var listenNetwork string
	var listenAddress string
	var boundAddress string
	listen := func(network, address string) (net.Listener, error) {
		listenCalls++
		listenNetwork = network
		listenAddress = address
		inner, err := net.Listen("tcp4", "127.0.0.1:0")
		if err != nil {
			return nil, err
		}
		boundAddress = inner.Addr().String()
		return &serverTestSourceListener{
			Listener: inner,
			remote:   &net.TCPAddr{IP: net.ParseIP("192.168.50.25"), Port: 40000},
		}, nil
	}
	server, err := NewServer(
		config,
		WithResolver(resolver),
		WithAddressPolicy(serverTestPolicy(func(netip.Addr) bool { return true })),
		WithDialContext(dialer.DialContext),
		WithListen(listen),
	)
	if err != nil {
		t.Fatalf("NewServer() error = %v", err)
	}

	if err := server.Stop(context.Background()); err != nil {
		t.Fatalf("Stop() before Start error = %v", err)
	}
	if err := server.Start(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if listenCalls != 1 || listenNetwork != "tcp4" || listenAddress != config.BindAddress() {
		t.Fatalf("listen calls=%d network=%q address=%q", listenCalls, listenNetwork, listenAddress)
	}
	if server.Address() != boundAddress {
		t.Fatalf("Address() = %q, want %q", server.Address(), boundAddress)
	}
	if err := server.Start(); !errors.Is(err, errServerAlreadyStarted) {
		t.Fatalf("second Start() error = %v, want %v", err, errServerAlreadyStarted)
	}
	if listenCalls != 1 {
		t.Fatalf("listen calls after second Start = %d, want 1", listenCalls)
	}

	first := serverTestOpenIdleHTTPConnection(t, server.Address())
	defer first.Close()
	serverTestWaitFor(t, func() bool { return server.Clients() == 1 })
	second := serverTestOpenIdleHTTPConnection(t, server.Address())
	defer second.Close()
	serverTestWaitFor(t, func() bool {
		server.mutex.Lock()
		listener := server.listener
		server.mutex.Unlock()
		return server.Clients() == 1 && listener != nil && listener.ConnectionCount() == 2
	})
	third, err := net.DialTimeout("tcp", server.Address(), 2*time.Second)
	if err != nil {
		t.Fatalf("dialing third connection: %v", err)
	}
	defer third.Close()
	serverTestWaitFor(t, func() bool {
		server.mutex.Lock()
		listener := server.listener
		server.mutex.Unlock()
		return server.Clients() == 1 && listener != nil && listener.ConnectionCount() == 3
	})

	server.mutex.Lock()
	serveDone := server.serveDone
	server.mutex.Unlock()
	stopContext, cancelStop := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancelStop()
	if err := server.Stop(stopContext); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
	if server.Address() != "" || server.Clients() != 0 {
		t.Fatalf("stopped server Address=%q Clients=%d", server.Address(), server.Clients())
	}
	select {
	case <-serveDone:
	default:
		t.Fatal("Serve goroutine did not exit before Stop returned")
	}
	for index, connection := range []net.Conn{first, second, third} {
		if err := connection.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
			t.Fatalf("connection %d SetReadDeadline() error = %v", index, err)
		}
		buffer := make([]byte, 1)
		if _, err := connection.Read(buffer); err == nil {
			t.Fatalf("connection %d remained open after Stop", index)
		}
	}
	if err := server.Stop(context.Background()); err != nil {
		t.Fatalf("second Stop() error = %v", err)
	}
}

func serverTestConfig(auth bool) proxy.Config {
	return proxy.Config{
		Interface: proxy.NetworkInterface{
			Index:   7,
			Name:    "Ethernet",
			Address: netip.MustParsePrefix("192.168.50.10/24"),
		},
		Port:        8080,
		AuthEnabled: auth,
		Credentials: proxy.Credentials{Username: "user", Password: "password"},
	}
}

func serverTestNew(t *testing.T, config proxy.Config, resolver Resolver, policy AddressPolicy, dialer *serverTestDialer) *Server {
	t.Helper()
	server, err := NewServer(
		config,
		WithResolver(resolver),
		WithAddressPolicy(policy),
		WithDialContext(dialer.DialContext),
	)
	if err != nil {
		t.Fatalf("NewServer() error = %v", err)
	}
	return server
}

func serverTestUpstreamResponse(request *http.Request) *http.Response {
	return &http.Response{
		Status:        "200 OK",
		StatusCode:    http.StatusOK,
		Proto:         "HTTP/1.1",
		ProtoMajor:    1,
		ProtoMinor:    1,
		Header:        make(http.Header),
		Body:          io.NopCloser(strings.NewReader("upstream")),
		ContentLength: int64(len("upstream")),
		Request:       request,
	}
}

func serverTestServeHTTP(t *testing.T, server *Server, request *http.Request) (*http.Response, string) {
	t.Helper()
	recorder := httptest.NewRecorder()
	server.proxyServer.ServeHTTP(recorder, request)
	response := recorder.Result()
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("reading response body: %v", err)
	}
	return response, string(body)
}

func serverTestConnect(t *testing.T, proxyAddress, authority, authorization string) (net.Conn, *bufio.Reader, *http.Response, string) {
	t.Helper()
	connection, err := net.DialTimeout("tcp", proxyAddress, 2*time.Second)
	if err != nil {
		t.Fatalf("dialing proxy: %v", err)
	}
	if err := connection.SetDeadline(time.Now().Add(2 * time.Second)); err != nil {
		connection.Close()
		t.Fatalf("SetDeadline() error = %v", err)
	}
	var request strings.Builder
	fmt.Fprintf(&request, "CONNECT %s HTTP/1.1\r\nHost: %s\r\n", authority, authority)
	if authorization != "" {
		fmt.Fprintf(&request, "Proxy-Authorization: %s\r\n", authorization)
	}
	request.WriteString("\r\n")
	if _, err := io.WriteString(connection, request.String()); err != nil {
		connection.Close()
		t.Fatalf("writing CONNECT request: %v", err)
	}
	reader := bufio.NewReader(connection)
	response, err := http.ReadResponse(reader, &http.Request{Method: http.MethodConnect})
	if err != nil {
		connection.Close()
		t.Fatalf("reading CONNECT response: %v", err)
	}
	body := ""
	if response.StatusCode != http.StatusOK {
		bodyBytes, readErr := io.ReadAll(response.Body)
		if readErr != nil {
			connection.Close()
			t.Fatalf("reading CONNECT response body: %v", readErr)
		}
		_ = response.Body.Close()
		body = string(bodyBytes)
	}
	return connection, reader, response, body
}

func serverTestOpenIdleHTTPConnection(t *testing.T, address string) net.Conn {
	t.Helper()
	connection, err := net.DialTimeout("tcp", address, 2*time.Second)
	if err != nil {
		t.Fatalf("dialing server: %v", err)
	}
	if err := connection.SetDeadline(time.Now().Add(2 * time.Second)); err != nil {
		connection.Close()
		t.Fatalf("SetDeadline() error = %v", err)
	}
	if _, err := io.WriteString(connection, "GET /health HTTP/1.1\r\nHost: proxy.test\r\n\r\n"); err != nil {
		connection.Close()
		t.Fatalf("writing HTTP request: %v", err)
	}
	response, err := http.ReadResponse(bufio.NewReader(connection), &http.Request{Method: http.MethodGet})
	if err != nil {
		connection.Close()
		t.Fatalf("reading HTTP response: %v", err)
	}
	_, readErr := io.ReadAll(response.Body)
	closeErr := response.Body.Close()
	if readErr != nil || closeErr != nil {
		connection.Close()
		t.Fatalf("reading HTTP response body: read=%v close=%v", readErr, closeErr)
	}
	if response.StatusCode != http.StatusBadRequest {
		connection.Close()
		t.Fatalf("StatusCode = %d, want %d", response.StatusCode, http.StatusBadRequest)
	}
	if err := connection.SetDeadline(time.Time{}); err != nil {
		connection.Close()
		t.Fatalf("clearing deadline: %v", err)
	}
	return connection
}

func serverTestHTTPServerAddress(t *testing.T, rawURL string) string {
	t.Helper()
	parsed, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("parsing test server URL: %v", err)
	}
	return parsed.Host
}

func serverTestBasicAuthorization(username, password string) string {
	return "Basic " + serverTestBasicAuthorizationValue(username, password)
}

func serverTestBasicAuthorizationValue(username, password string) string {
	return base64.StdEncoding.EncodeToString([]byte(username + ":" + password))
}

func serverTestWaitFor(t *testing.T, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("condition was not met before timeout")
}
