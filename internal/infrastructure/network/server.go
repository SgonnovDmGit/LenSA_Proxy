package network

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/SgonnovDmGit/LenSA_Proxy/internal/domain/proxy"
	"github.com/elazarl/goproxy"
)

const (
	defaultMaxConnections    = 128
	defaultReadHeaderTimeout = 10 * time.Second
	defaultDialTimeout       = 10 * time.Second
	defaultMaxHeaderBytes    = 64 * 1024
	defaultIdleTimeout       = 30 * time.Minute
)

var (
	errInvalidServerOption  = errors.New("invalid server option")
	errServerAlreadyStarted = errors.New("server already started")
	errNilServer            = errors.New("server is nil")
	errNilStopContext       = errors.New("stop context is nil")
	errNilListener          = errors.New("listen returned a nil listener")
)

type ServerOption func(*serverOptions)

type serverOptions struct {
	resolver     Resolver
	policy       AddressPolicy
	dial         DialContextFunc
	connectPorts map[uint16]struct{}
	listen       func(string, string) (net.Listener, error)
	err          error
}

type serverState uint8

const (
	serverNew serverState = iota
	serverStarting
	serverRunning
	serverStopping
	serverStopped
)

type Server struct {
	mutex       sync.Mutex
	config      proxy.Config
	listen      func(string, string) (net.Listener, error)
	proxyServer *goproxy.ProxyHttpServer
	httpServer  *http.Server
	transport   *http.Transport
	state       serverState
	startDone   chan struct{}
	stopDone    chan struct{}
	stopErr     error
	listener    *managedListener
	address     string
	serveDone   chan struct{}
	serveErr    error
}

type localResponse struct{}

func WithResolver(resolver Resolver) ServerOption {
	return func(options *serverOptions) {
		if options == nil {
			return
		}
		if isNilValue(resolver) {
			options.reject(errInvalidServerOption)
			return
		}
		options.resolver = resolver
	}
}

func WithAddressPolicy(policy AddressPolicy) ServerOption {
	return func(options *serverOptions) {
		if options == nil {
			return
		}
		if isNilValue(policy) {
			options.reject(errInvalidServerOption)
			return
		}
		options.policy = policy
	}
}

func WithDialContext(dial DialContextFunc) ServerOption {
	return func(options *serverOptions) {
		if options == nil {
			return
		}
		if dial == nil {
			options.reject(errInvalidServerOption)
			return
		}
		options.dial = dial
	}
}

func WithConnectPorts(ports ...uint16) ServerOption {
	ports = append([]uint16(nil), ports...)
	return func(options *serverOptions) {
		if options == nil {
			return
		}
		if len(ports) == 0 {
			options.reject(errInvalidServerOption)
			return
		}
		allowed := make(map[uint16]struct{}, len(ports))
		for _, port := range ports {
			if port == 0 {
				options.reject(errInvalidServerOption)
				return
			}
			allowed[port] = struct{}{}
		}
		options.connectPorts = allowed
	}
}

func WithListen(listen func(string, string) (net.Listener, error)) ServerOption {
	return func(options *serverOptions) {
		if options == nil {
			return
		}
		if listen == nil {
			options.reject(errInvalidServerOption)
			return
		}
		options.listen = listen
	}
}

func NewServer(config proxy.Config, options ...ServerOption) (*Server, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}

	dialer := &net.Dialer{Timeout: defaultDialTimeout}
	serverOptions := serverOptions{
		resolver: net.DefaultResolver,
		dial:     dialer.DialContext,
		connectPorts: map[uint16]struct{}{
			443: {},
		},
		listen: net.Listen,
	}
	for _, option := range options {
		if option == nil {
			return nil, errInvalidServerOption
		}
		option(&serverOptions)
		if serverOptions.err != nil {
			return nil, serverOptions.err
		}
	}
	if isNilValue(serverOptions.resolver) || serverOptions.dial == nil ||
		serverOptions.listen == nil || len(serverOptions.connectPorts) == 0 {
		return nil, errInvalidServerOption
	}
	if isNilValue(serverOptions.policy) {
		policy, err := NewSystemPublicAddressPolicy()
		if err != nil {
			return nil, err
		}
		serverOptions.policy = policy
	}

	safeDialer := NewSafeDialer(serverOptions.resolver, serverOptions.policy, serverOptions.dial, defaultDialTimeout)
	proxyServer, transport := newForwardProxy(config, safeDialer, serverOptions.connectPorts)
	silentLogger := log.New(io.Discard, "", 0)
	httpServer := &http.Server{
		Handler:           proxyServer,
		ReadHeaderTimeout: defaultReadHeaderTimeout,
		MaxHeaderBytes:    defaultMaxHeaderBytes,
		IdleTimeout:       defaultIdleTimeout,
		ErrorLog:          silentLogger,
	}

	return &Server{
		config:      config,
		listen:      serverOptions.listen,
		proxyServer: proxyServer,
		httpServer:  httpServer,
		transport:   transport,
		state:       serverNew,
	}, nil
}

func (s *Server) Start() error {
	if s == nil {
		return errNilServer
	}

	s.mutex.Lock()
	if s.state != serverNew {
		s.mutex.Unlock()
		return errServerAlreadyStarted
	}
	s.state = serverStarting
	s.startDone = make(chan struct{})
	startDone := s.startDone
	listen := s.listen
	bindAddress := s.config.BindAddress()
	s.mutex.Unlock()

	listener, err := listen("tcp4", bindAddress)
	if err != nil {
		if !isNilValue(listener) {
			_ = listener.Close()
		}
		s.finishFailedStart(startDone)
		return err
	}
	if isNilValue(listener) {
		s.finishFailedStart(startDone)
		return errNilListener
	}
	listenerAddress := listener.Addr()
	if listenerAddress == nil {
		_ = listener.Close()
		s.finishFailedStart(startDone)
		return errNilListener
	}

	managed := newManagedListener(
		listener,
		s.config.Interface.Subnet(),
		defaultMaxConnections,
		defaultIdleTimeout,
	)
	serveDone := make(chan struct{})
	address := listenerAddress.String()

	s.mutex.Lock()
	s.listener = managed
	s.address = address
	s.serveDone = serveDone
	s.state = serverRunning
	go s.serve(managed, serveDone)
	close(startDone)
	s.mutex.Unlock()
	return nil
}

func (s *Server) Stop(ctx context.Context) error {
	if s == nil {
		return errNilServer
	}
	if ctx == nil {
		return errNilStopContext
	}

	for {
		s.mutex.Lock()
		switch s.state {
		case serverNew, serverStopped:
			s.mutex.Unlock()
			return nil
		case serverStarting:
			startDone := s.startDone
			s.mutex.Unlock()
			select {
			case <-startDone:
				continue
			case <-ctx.Done():
				return ctx.Err()
			}
		case serverStopping:
			stopDone := s.stopDone
			s.mutex.Unlock()
			select {
			case <-stopDone:
				s.mutex.Lock()
				err := s.stopErr
				s.mutex.Unlock()
				return err
			case <-ctx.Done():
				return ctx.Err()
			}
		case serverRunning:
			s.state = serverStopping
			s.stopDone = make(chan struct{})
			stopDone := s.stopDone
			listener := s.listener
			serveDone := s.serveDone
			s.mutex.Unlock()

			err := s.shutdown(ctx, listener, serveDone)

			s.mutex.Lock()
			s.stopErr = err
			s.listener = nil
			s.address = ""
			s.state = serverStopped
			close(stopDone)
			s.mutex.Unlock()
			return err
		}
	}
}

func (s *Server) Address() string {
	if s == nil {
		return ""
	}
	s.mutex.Lock()
	defer s.mutex.Unlock()
	return s.address
}

func (s *Server) Clients() int {
	if s == nil {
		return 0
	}
	s.mutex.Lock()
	listener := s.listener
	s.mutex.Unlock()
	if listener == nil {
		return 0
	}
	return listener.ClientCount()
}

func (options *serverOptions) reject(err error) {
	if options.err == nil {
		options.err = err
	}
}

func newForwardProxy(config proxy.Config, safeDialer *SafeDialer, connectPorts map[uint16]struct{}) (*goproxy.ProxyHttpServer, *http.Transport) {
	transport := &http.Transport{
		Proxy:              nil,
		DialContext:        safeDialer.DialContext,
		TLSClientConfig:    &tls.Config{},
		DisableCompression: true,
		ForceAttemptHTTP2:  false,
	}

	proxyServer := goproxy.NewProxyHttpServer()
	proxyServer.Verbose = false
	proxyServer.Logger = log.New(io.Discard, "", 0)
	proxyServer.Tr = transport
	proxyServer.ConnectDial = nil
	proxyServer.ConnectDialWithReq = nil
	proxyServer.KeepHeader = false
	proxyServer.KeepAcceptEncoding = true
	proxyServer.AllowHTTP2 = false
	proxyServer.NonproxyHandler = http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writeStatusResponse(writer, http.StatusBadRequest)
	})

	authenticator := NewBasicAuthenticator(config)
	proxyServer.OnRequest().DoFunc(func(request *http.Request, ctx *goproxy.ProxyCtx) (*http.Request, *http.Response) {
		if !authenticator.Authenticate(request) {
			ctx.UserData = localResponse{}
			return request, NewProxyAuthenticationRequiredResponse(request)
		}
		if !validHTTPProxyRequest(request) {
			ctx.UserData = localResponse{}
			return request, newStatusResponse(request, http.StatusBadRequest)
		}
		SanitizeHeaders(request.Header)
		return request, nil
	})
	proxyServer.OnResponse().DoFunc(func(response *http.Response, ctx *goproxy.ProxyCtx) *http.Response {
		if response == nil {
			return newStatusResponse(ctx.Req, safeDialErrorStatus(ctx.Error))
		}
		if _, local := ctx.UserData.(localResponse); !local {
			SanitizeHeaders(response.Header)
		}
		return response
	})
	proxyServer.OnRequest().HandleConnectFunc(func(authority string, ctx *goproxy.ProxyCtx) (*goproxy.ConnectAction, string) {
		request := ctx.Req
		authenticated := authenticator.Authenticate(request)
		if request != nil {
			SanitizeHeaders(request.Header)
		}
		if !authenticated {
			ctx.Resp = NewProxyAuthenticationRequiredResponse(request)
			ctx.Resp.Close = true
			return goproxy.RejectConnect, authority
		}

		host, port, err := parseDestination(authority)
		if err != nil {
			ctx.Resp = newStatusResponse(request, http.StatusBadRequest)
			return goproxy.RejectConnect, authority
		}
		if _, allowed := connectPorts[port]; !allowed {
			ctx.Resp = newStatusResponse(request, http.StatusForbidden)
			return goproxy.RejectConnect, authority
		}
		return goproxy.OkConnect, net.JoinHostPort(host, strconv.Itoa(int(port)))
	})
	proxyServer.ConnectionErrHandler = func(writer io.Writer, ctx *goproxy.ProxyCtx, err error) {
		response := newStatusResponse(ctx.Req, safeDialErrorStatus(err))
		_ = response.Write(writer)
	}

	return proxyServer, transport
}

func validHTTPProxyRequest(request *http.Request) bool {
	return request != nil && request.URL != nil && request.URL.IsAbs() &&
		strings.EqualFold(request.URL.Scheme, "http") && request.URL.Hostname() != ""
}

func safeDialErrorStatus(err error) int {
	if errors.Is(err, ErrInvalidDestination) || errors.Is(err, ErrNoAllowedAddresses) {
		return http.StatusForbidden
	}
	return http.StatusBadGateway
}

func newStatusResponse(request *http.Request, status int) *http.Response {
	bodyText := http.StatusText(status) + "\n"
	proto := "HTTP/1.1"
	protoMajor := 1
	protoMinor := 1
	if request != nil && request.Proto != "" {
		proto = request.Proto
		protoMajor = request.ProtoMajor
		protoMinor = request.ProtoMinor
	}
	return &http.Response{
		Status:        fmt.Sprintf("%d %s", status, http.StatusText(status)),
		StatusCode:    status,
		Proto:         proto,
		ProtoMajor:    protoMajor,
		ProtoMinor:    protoMinor,
		Header:        http.Header{"Content-Type": {"text/plain; charset=utf-8"}},
		Body:          io.NopCloser(strings.NewReader(bodyText)),
		ContentLength: int64(len(bodyText)),
		Request:       request,
	}
}

func writeStatusResponse(writer http.ResponseWriter, status int) {
	writer.Header().Set("Content-Type", "text/plain; charset=utf-8")
	writer.WriteHeader(status)
	_, _ = io.WriteString(writer, http.StatusText(status)+"\n")
}

func (s *Server) finishFailedStart(startDone chan struct{}) {
	s.mutex.Lock()
	s.state = serverStopped
	close(startDone)
	s.mutex.Unlock()
}

func (s *Server) serve(listener net.Listener, done chan struct{}) {
	err := s.httpServer.Serve(listener)
	err = normalizeServerError(err)
	s.mutex.Lock()
	s.serveErr = err
	s.mutex.Unlock()
	close(done)
}

func (s *Server) shutdown(ctx context.Context, listener *managedListener, serveDone <-chan struct{}) error {
	listenerErr := normalizeServerError(listener.Close())
	shutdownErr := normalizeServerError(s.httpServer.Shutdown(ctx))
	if errors.Is(shutdownErr, context.DeadlineExceeded) {
		shutdownErr = nil
	}
	listener.CloseConnections()
	s.transport.CloseIdleConnections()
	closeErr := normalizeServerError(s.httpServer.Close())

	waitErr := waitForServeExit(ctx, serveDone)
	var serveErr error
	if waitErr == nil {
		s.mutex.Lock()
		serveErr = s.serveErr
		s.mutex.Unlock()
	}
	return errors.Join(listenerErr, shutdownErr, closeErr, waitErr, serveErr)
}

func waitForServeExit(ctx context.Context, done <-chan struct{}) error {
	select {
	case <-done:
		return nil
	default:
	}
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func normalizeServerError(err error) error {
	if errors.Is(err, http.ErrServerClosed) || errors.Is(err, net.ErrClosed) {
		return nil
	}
	return err
}

func isNilValue(value any) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return reflected.IsNil()
	default:
		return false
	}
}
