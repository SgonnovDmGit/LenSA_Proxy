package integration_test

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
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

	proxyDomain "github.com/SgonnovDmGit/LenSA_Proxy/internal/domain/proxy"
	"github.com/SgonnovDmGit/LenSA_Proxy/internal/infrastructure/network"
)

type allowAddressPolicy struct {
	address netip.Addr
}

func (p allowAddressPolicy) Allow(address netip.Addr) bool {
	return address.Unmap() == p.address.Unmap()
}

func TestHTTPForwardingAndAuthentication(t *testing.T) {
	networkInterface := integrationInterface(t)
	var requestMutex sync.Mutex
	var receivedMethod string
	var receivedQuery string
	var receivedBody string
	var receivedEndToEnd string
	var receivedProxyAuthorization string
	backend := startHTTPBackend(t, networkInterface.Address.Addr(), http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		body, err := io.ReadAll(request.Body)
		if err != nil {
			t.Errorf("reading backend request: %v", err)
		}
		requestMutex.Lock()
		receivedMethod = request.Method
		receivedQuery = request.URL.RawQuery
		receivedBody = string(body)
		receivedEndToEnd = request.Header.Get("X-End-To-End")
		receivedProxyAuthorization = request.Header.Get("Proxy-Authorization")
		requestMutex.Unlock()
		writer.Header().Set("X-Upstream", "retained")
		writer.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(writer, "forwarded")
	}))
	defer backend.Close(t)

	server := startProxy(t, networkInterface, true, allowAddressPolicy{address: networkInterface.Address.Addr()}, backend.Port())
	defer stopProxy(t, server)

	unauthenticated := integrationHTTPClient(t, server.Address(), "", "")
	response, err := unauthenticated.Get(backend.URL("/without-auth"))
	if err != nil {
		t.Fatalf("unauthenticated request: %v", err)
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusProxyAuthRequired {
		t.Fatalf("unauthenticated status = %d, want %d", response.StatusCode, http.StatusProxyAuthRequired)
	}
	if got := response.Header.Get("Proxy-Authenticate"); got != `Basic realm="LenSA Proxy"` {
		t.Fatalf("Proxy-Authenticate = %q", got)
	}

	authenticated := integrationHTTPClient(t, server.Address(), "user", "password")
	request, err := http.NewRequest(http.MethodPost, backend.URL("/resource?item=42"), strings.NewReader("payload"))
	if err != nil {
		t.Fatalf("creating request: %v", err)
	}
	request.Header.Set("X-End-To-End", "retained")
	response, err = authenticated.Do(request)
	if err != nil {
		t.Fatalf("authenticated request: %v", err)
	}
	body, err := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if err != nil {
		t.Fatalf("reading response: %v", err)
	}
	if response.StatusCode != http.StatusCreated || string(body) != "forwarded" {
		t.Fatalf("response = %d %q", response.StatusCode, body)
	}
	if got := response.Header.Get("X-Upstream"); got != "retained" {
		t.Fatalf("X-Upstream = %q", got)
	}

	requestMutex.Lock()
	defer requestMutex.Unlock()
	if receivedMethod != http.MethodPost || receivedQuery != "item=42" || receivedBody != "payload" {
		t.Fatalf("backend request = %s ?%s %q", receivedMethod, receivedQuery, receivedBody)
	}
	if receivedEndToEnd != "retained" {
		t.Fatalf("backend X-End-To-End = %q", receivedEndToEnd)
	}
	if receivedProxyAuthorization != "" {
		t.Fatal("Proxy-Authorization reached backend")
	}
}

func TestSSEStreamsBeforeUpstreamClose(t *testing.T) {
	networkInterface := integrationInterface(t)
	release := make(chan struct{})
	backend := startHTTPBackend(t, networkInterface.Address.Addr(), http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(writer, "data: first\n\n")
		writer.(http.Flusher).Flush()
		<-release
	}))
	defer func() {
		close(release)
		backend.Close(t)
	}()

	server := startProxy(t, networkInterface, false, allowAddressPolicy{address: networkInterface.Address.Addr()}, backend.Port())
	defer stopProxy(t, server)
	client := integrationHTTPClient(t, server.Address(), "", "")

	requestContext, cancelRequest := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancelRequest()
	request, err := http.NewRequestWithContext(requestContext, http.MethodGet, backend.URL("/events"), nil)
	if err != nil {
		t.Fatalf("creating request: %v", err)
	}
	response, err := client.Do(request)
	if err != nil {
		t.Fatalf("SSE request: %v", err)
	}
	defer response.Body.Close()

	line, err := bufio.NewReader(response.Body).ReadString('\n')
	if err != nil {
		t.Fatalf("reading first SSE line: %v", err)
	}
	if line != "data: first\n" {
		t.Fatalf("first SSE line = %q", line)
	}
}

func TestHTTPSConnectCarriesTargetTLS(t *testing.T) {
	networkInterface := integrationInterface(t)
	listener, err := net.Listen("tcp4", netip.AddrPortFrom(networkInterface.Address.Addr(), 0).String())
	if err != nil {
		t.Fatalf("starting TLS backend: %v", err)
	}
	backend := httptest.NewUnstartedServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(writer, "secure")
	}))
	backend.Listener = listener
	backend.StartTLS()
	defer backend.Close()
	backendAddress := netip.MustParseAddrPort(listener.Addr().String())

	server := startProxy(t, networkInterface, false, allowAddressPolicy{address: networkInterface.Address.Addr()}, backendAddress.Port())
	defer stopProxy(t, server)
	proxyURL := &url.URL{Scheme: "http", Host: server.Address()}
	transport := &http.Transport{
		Proxy:           http.ProxyURL(proxyURL),
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 2 * time.Second}
	response, err := client.Get("https://" + backendAddress.String() + "/secure")
	if err != nil {
		t.Fatalf("HTTPS through proxy: %v", err)
	}
	body, err := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if err != nil {
		t.Fatalf("reading HTTPS response: %v", err)
	}
	if response.StatusCode != http.StatusOK || string(body) != "secure" {
		t.Fatalf("HTTPS response = %d %q", response.StatusCode, body)
	}
	if response.TLS == nil || len(response.TLS.PeerCertificates) == 0 {
		t.Fatal("HTTPS response has no target certificate")
	}
	if !bytes.Equal(response.TLS.PeerCertificates[0].Raw, backend.Certificate().Raw) {
		t.Fatal("proxy replaced the target TLS certificate")
	}
}

func TestConnectTunnelStopsAndPortCanBeReused(t *testing.T) {
	networkInterface := integrationInterface(t)
	echo := startEchoBackend(t, networkInterface.Address.Addr())
	defer echo.Close(t)

	server := startProxy(t, networkInterface, false, allowAddressPolicy{address: networkInterface.Address.Addr()}, echo.Port())
	connection, err := net.DialTimeout("tcp4", server.Address(), 2*time.Second)
	if err != nil {
		t.Fatalf("dialing proxy: %v", err)
	}
	reader := bufio.NewReader(connection)
	authority := netip.AddrPortFrom(networkInterface.Address.Addr(), echo.Port()).String()
	if _, err := fmt.Fprintf(connection, "CONNECT %s HTTP/1.1\r\nHost: %s\r\n\r\n", authority, authority); err != nil {
		t.Fatalf("writing CONNECT: %v", err)
	}
	request := &http.Request{Method: http.MethodConnect}
	response, err := http.ReadResponse(reader, request)
	if err != nil {
		t.Fatalf("reading CONNECT response: %v", err)
	}
	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("CONNECT status = %d body=%q", response.StatusCode, body)
	}
	if _, err := connection.Write([]byte("ping")); err != nil {
		t.Fatalf("writing tunnel: %v", err)
	}
	buffer := make([]byte, 4)
	if _, err := io.ReadFull(reader, buffer); err != nil {
		t.Fatalf("reading tunnel: %v", err)
	}
	if string(buffer) != "ping" {
		t.Fatalf("tunnel payload = %q", buffer)
	}
	integrationWaitFor(t, func() bool { return server.Clients() == 1 })

	proxyAddress := server.Address()
	stopProxy(t, server)
	if err := connection.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatalf("SetReadDeadline() error = %v", err)
	}
	if _, err := connection.Read(make([]byte, 1)); err == nil {
		t.Fatal("CONNECT remained open after Stop")
	}
	_ = connection.Close()

	restarted, err := network.NewServer(
		proxyDomain.Config{
			Interface: networkInterface,
			Port:      netip.MustParseAddrPort(proxyAddress).Port(),
		},
		network.WithAddressPolicy(allowAddressPolicy{address: networkInterface.Address.Addr()}),
		network.WithConnectPorts(echo.Port()),
	)
	if err != nil {
		t.Fatalf("creating restarted proxy: %v", err)
	}
	if err := restarted.Start(); err != nil {
		t.Fatalf("restarting on same port: %v", err)
	}
	stopProxy(t, restarted)
}

func TestProductionPolicyBlocksPrivateTarget(t *testing.T) {
	networkInterface := integrationInterface(t)
	backend := startHTTPBackend(t, networkInterface.Address.Addr(), http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Error("blocked backend received a request")
	}))
	defer backend.Close(t)

	server := startProxy(t, networkInterface, false, nil, 443)
	defer stopProxy(t, server)
	client := integrationHTTPClient(t, server.Address(), "", "")
	response, err := client.Get(backend.URL("/private"))
	if err != nil {
		t.Fatalf("blocked request: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusForbidden {
		t.Fatalf("blocked status = %d, want %d", response.StatusCode, http.StatusForbidden)
	}
}

type runningHTTPBackend struct {
	server   *http.Server
	listener net.Listener
	address  netip.AddrPort
}

func startHTTPBackend(t *testing.T, address netip.Addr, handler http.Handler) *runningHTTPBackend {
	t.Helper()
	listener, err := net.Listen("tcp4", netip.AddrPortFrom(address, 0).String())
	if err != nil {
		t.Fatalf("starting HTTP backend: %v", err)
	}
	addressPort := netip.MustParseAddrPort(listener.Addr().String())
	server := &http.Server{Handler: handler, ReadHeaderTimeout: time.Second}
	go func() {
		_ = server.Serve(listener)
	}()
	return &runningHTTPBackend{server: server, listener: listener, address: addressPort}
}

func (b *runningHTTPBackend) URL(path string) string {
	return "http://" + b.address.String() + path
}

func (b *runningHTTPBackend) Port() uint16 {
	return b.address.Port()
}

func (b *runningHTTPBackend) Close(t *testing.T) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_ = b.server.Shutdown(ctx)
	_ = b.listener.Close()
}

type runningEchoBackend struct {
	listener net.Listener
	address  netip.AddrPort
}

func startEchoBackend(t *testing.T, address netip.Addr) *runningEchoBackend {
	t.Helper()
	listener, err := net.Listen("tcp4", netip.AddrPortFrom(address, 0).String())
	if err != nil {
		t.Fatalf("starting echo backend: %v", err)
	}
	backend := &runningEchoBackend{
		listener: listener,
		address:  netip.MustParseAddrPort(listener.Addr().String()),
	}
	go func() {
		for {
			connection, acceptErr := listener.Accept()
			if acceptErr != nil {
				return
			}
			go func() {
				defer connection.Close()
				_, _ = io.Copy(connection, connection)
			}()
		}
	}()
	return backend
}

func (b *runningEchoBackend) Port() uint16 {
	return b.address.Port()
}

func (b *runningEchoBackend) Close(t *testing.T) {
	t.Helper()
	if err := b.listener.Close(); err != nil && !strings.Contains(err.Error(), "closed") {
		t.Errorf("closing echo backend: %v", err)
	}
}

func integrationInterface(t *testing.T) proxyDomain.NetworkInterface {
	t.Helper()
	interfaces, err := network.DiscoverInterfaces()
	if err != nil {
		t.Fatalf("discovering interfaces: %v", err)
	}
	if len(interfaces) == 0 {
		t.Skip("integration test requires an active RFC1918 IPv4 interface")
	}
	return interfaces[0]
}

func startProxy(t *testing.T, networkInterface proxyDomain.NetworkInterface, auth bool, policy network.AddressPolicy, connectPort uint16) *network.Server {
	t.Helper()
	port := reservePort(t, networkInterface.Address.Addr())
	config := proxyDomain.Config{
		Interface:   networkInterface,
		Port:        port,
		AuthEnabled: auth,
		Credentials: proxyDomain.Credentials{Username: "user", Password: "password"},
	}
	options := []network.ServerOption{network.WithConnectPorts(connectPort)}
	if policy != nil {
		options = append(options, network.WithAddressPolicy(policy))
	}
	server, err := network.NewServer(config, options...)
	if err != nil {
		t.Fatalf("creating proxy: %v", err)
	}
	if err := server.Start(); err != nil {
		t.Fatalf("starting proxy: %v", err)
	}
	return server
}

func stopProxy(t *testing.T, server *network.Server) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := server.Stop(ctx); err != nil {
		t.Errorf("stopping proxy: %v", err)
	}
}

func reservePort(t *testing.T, address netip.Addr) uint16 {
	t.Helper()
	listener, err := net.Listen("tcp4", netip.AddrPortFrom(address, 0).String())
	if err != nil {
		t.Fatalf("reserving proxy port: %v", err)
	}
	port := netip.MustParseAddrPort(listener.Addr().String()).Port()
	if err := listener.Close(); err != nil {
		t.Fatalf("releasing proxy port: %v", err)
	}
	return port
}

func integrationHTTPClient(t *testing.T, proxyAddress, username, password string) *http.Client {
	t.Helper()
	proxyURL := &url.URL{Scheme: "http", Host: proxyAddress}
	if username != "" {
		proxyURL.User = url.UserPassword(username, password)
	}
	transport := &http.Transport{
		Proxy:             http.ProxyURL(proxyURL),
		DisableKeepAlives: true,
	}
	t.Cleanup(transport.CloseIdleConnections)
	return &http.Client{Transport: transport, Timeout: 2 * time.Second}
}

func integrationWaitFor(t *testing.T, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("condition was not met before timeout")
}
