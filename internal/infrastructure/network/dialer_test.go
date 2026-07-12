package network

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"strings"
	"testing"
	"time"
)

type fakeResolver struct {
	addresses []netip.Addr
	err       error
	calls     int
	networks  []string
	hosts     []string
	deadlines []time.Time
}

func (f *fakeResolver) LookupNetIP(ctx context.Context, network, host string) ([]netip.Addr, error) {
	f.calls++
	f.networks = append(f.networks, network)
	f.hosts = append(f.hosts, host)
	deadline, _ := ctx.Deadline()
	f.deadlines = append(f.deadlines, deadline)
	return f.addresses, f.err
}

type fakeAddressPolicy struct {
	allow bool
}

func (f fakeAddressPolicy) Allow(netip.Addr) bool {
	return f.allow
}

type dialAttempt struct {
	network     string
	destination string
	deadline    time.Time
}

type fakeDialFunc struct {
	attempts []dialAttempt
	errors   []error
}

func (f *fakeDialFunc) DialContext(ctx context.Context, network, destination string) (net.Conn, error) {
	deadline, _ := ctx.Deadline()
	f.attempts = append(f.attempts, dialAttempt{
		network:     network,
		destination: destination,
		deadline:    deadline,
	})
	attempt := len(f.attempts) - 1
	if attempt < len(f.errors) && f.errors[attempt] != nil {
		return nil, f.errors[attempt]
	}
	connection, peer := net.Pipe()
	_ = peer.Close()
	return connection, nil
}

func TestSafeDialerResolvesHostnameOnce(t *testing.T) {
	resolver := &fakeResolver{addresses: []netip.Addr{netip.MustParseAddr("1.1.1.1")}}
	dial := &fakeDialFunc{}
	dialer := NewSafeDialer(resolver, fakeAddressPolicy{allow: true}, dial.DialContext, time.Second)

	connection, err := dialer.DialContext(context.Background(), "tcp", "example.com:443")
	if err != nil {
		t.Fatalf("DialContext() error = %v", err)
	}
	defer connection.Close()

	if resolver.calls != 1 {
		t.Fatalf("LookupNetIP() calls = %d, want 1", resolver.calls)
	}
	if got, want := resolver.networks[0], "ip"; got != want {
		t.Fatalf("LookupNetIP() network = %q, want %q", got, want)
	}
	if got, want := resolver.hosts[0], "example.com"; got != want {
		t.Fatalf("LookupNetIP() host = %q, want %q", got, want)
	}
}

func TestSafeDialerLiteralDoesNotResolve(t *testing.T) {
	tests := []struct {
		name        string
		destination string
	}{
		{name: "IPv4", destination: "8.8.8.8:443"},
		{name: "IPv6", destination: "[2606:4700:4700::1111]:443"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resolver := &fakeResolver{err: errors.New("unexpected resolution")}
			dial := &fakeDialFunc{}
			dialer := NewSafeDialer(resolver, fakeAddressPolicy{allow: true}, dial.DialContext, time.Second)

			connection, err := dialer.DialContext(context.Background(), "tcp", tt.destination)
			if err != nil {
				t.Fatalf("DialContext() error = %v", err)
			}
			defer connection.Close()

			if resolver.calls != 0 {
				t.Fatalf("LookupNetIP() calls = %d, want 0", resolver.calls)
			}
			if got := dial.attempts[0].destination; got != tt.destination {
				t.Fatalf("dial destination = %q, want %q", got, tt.destination)
			}
		})
	}
}

func TestSafeDialerDialsResolvedLiteral(t *testing.T) {
	resolver := &fakeResolver{addresses: []netip.Addr{netip.MustParseAddr("93.184.216.34")}}
	dial := &fakeDialFunc{}
	dialer := NewSafeDialer(resolver, fakeAddressPolicy{allow: true}, dial.DialContext, time.Second)

	connection, err := dialer.DialContext(context.Background(), "tcp", "example.com:8443")
	if err != nil {
		t.Fatalf("DialContext() error = %v", err)
	}
	defer connection.Close()

	if len(dial.attempts) != 1 {
		t.Fatalf("dial calls = %d, want 1", len(dial.attempts))
	}
	if got, want := dial.attempts[0].destination, "93.184.216.34:8443"; got != want {
		t.Fatalf("dial destination = %q, want %q", got, want)
	}
}

func TestSafeDialerFallsBackToSecondAddress(t *testing.T) {
	resolver := &fakeResolver{addresses: []netip.Addr{
		netip.MustParseAddr("1.1.1.1"),
		netip.MustParseAddr("8.8.8.8"),
	}}
	dial := &fakeDialFunc{errors: []error{errors.New("first address unavailable")}}
	dialer := NewSafeDialer(resolver, fakeAddressPolicy{allow: true}, dial.DialContext, time.Second)

	connection, err := dialer.DialContext(context.Background(), "tcp", "example.com:443")
	if err != nil {
		t.Fatalf("DialContext() error = %v", err)
	}
	defer connection.Close()

	if len(dial.attempts) != 2 {
		t.Fatalf("dial calls = %d, want 2", len(dial.attempts))
	}
	if got, want := dial.attempts[1].destination, "8.8.8.8:443"; got != want {
		t.Fatalf("second dial destination = %q, want %q", got, want)
	}
}

func TestSafeDialerNeverDialsBlockedMixedAddresses(t *testing.T) {
	resolver := &fakeResolver{addresses: []netip.Addr{
		netip.MustParseAddr("10.0.0.1"),
		netip.MustParseAddr("fd00::1"),
		netip.MustParseAddr("1.1.1.1"),
	}}
	dial := &fakeDialFunc{errors: []error{errors.New("public address unavailable")}}
	dialer := NewSafeDialer(resolver, NewPublicAddressPolicy(nil), dial.DialContext, time.Second)

	connection, err := dialer.DialContext(context.Background(), "tcp", "example.com:443")
	if connection != nil {
		connection.Close()
		t.Fatal("DialContext() connection is non-nil")
	}
	if !errors.Is(err, ErrDialFailed) {
		t.Fatalf("DialContext() error = %v, want %v", err, ErrDialFailed)
	}
	if len(dial.attempts) != 1 {
		t.Fatalf("dial calls = %d, want 1", len(dial.attempts))
	}
	if got, want := dial.attempts[0].destination, "1.1.1.1:443"; got != want {
		t.Fatalf("dial destination = %q, want %q", got, want)
	}
}

func TestSafeDialerRejectsAllBlockedAddresses(t *testing.T) {
	resolver := &fakeResolver{addresses: []netip.Addr{
		netip.MustParseAddr("127.0.0.1"),
		netip.MustParseAddr("192.168.1.1"),
		netip.MustParseAddr("::1"),
	}}
	dial := &fakeDialFunc{}
	dialer := NewSafeDialer(resolver, NewPublicAddressPolicy(nil), dial.DialContext, time.Second)

	connection, err := dialer.DialContext(context.Background(), "tcp", "example.com:443")
	if connection != nil {
		connection.Close()
		t.Fatal("DialContext() connection is non-nil")
	}
	if !errors.Is(err, ErrNoAllowedAddresses) {
		t.Fatalf("DialContext() error = %v, want %v", err, ErrNoAllowedAddresses)
	}
	if len(dial.attempts) != 0 {
		t.Fatalf("dial calls = %d, want 0", len(dial.attempts))
	}
}

func TestSafeDialerRejectsMalformedDestination(t *testing.T) {
	destinations := []string{
		"example.com",
		"example.com:not-a-port",
		"example.com:0",
		":443",
		"https://user:secret@example.com/path",
	}

	for _, destination := range destinations {
		t.Run(destination, func(t *testing.T) {
			resolver := &fakeResolver{}
			dial := &fakeDialFunc{}
			dialer := NewSafeDialer(resolver, fakeAddressPolicy{allow: true}, dial.DialContext, time.Second)

			connection, err := dialer.DialContext(context.Background(), "tcp", destination)
			if connection != nil {
				connection.Close()
				t.Fatal("DialContext() connection is non-nil")
			}
			if !errors.Is(err, ErrInvalidDestination) {
				t.Fatalf("DialContext() error = %v, want %v", err, ErrInvalidDestination)
			}
			if strings.Contains(err.Error(), destination) || strings.Contains(err.Error(), "secret") {
				t.Fatalf("DialContext() error exposes destination: %q", err)
			}
			if resolver.calls != 0 {
				t.Fatalf("LookupNetIP() calls = %d, want 0", resolver.calls)
			}
			if len(dial.attempts) != 0 {
				t.Fatalf("dial calls = %d, want 0", len(dial.attempts))
			}
		})
	}
}

func TestSafeDialerPrefersIPv4(t *testing.T) {
	resolver := &fakeResolver{addresses: []netip.Addr{
		netip.MustParseAddr("2606:4700:4700::1111"),
		netip.MustParseAddr("1.1.1.1"),
	}}
	dial := &fakeDialFunc{}
	dialer := NewSafeDialer(resolver, fakeAddressPolicy{allow: true}, dial.DialContext, time.Second)

	connection, err := dialer.DialContext(context.Background(), "tcp", "example.com:443")
	if err != nil {
		t.Fatalf("DialContext() error = %v", err)
	}
	defer connection.Close()

	if got, want := dial.attempts[0].destination, "1.1.1.1:443"; got != want {
		t.Fatalf("first dial destination = %q, want %q", got, want)
	}
}

func TestSafeDialerUsesOneTotalDeadline(t *testing.T) {
	resolver := &fakeResolver{addresses: []netip.Addr{
		netip.MustParseAddr("1.1.1.1"),
		netip.MustParseAddr("8.8.8.8"),
	}}
	dial := &fakeDialFunc{errors: []error{errors.New("first address unavailable")}}
	dialer := NewSafeDialer(resolver, fakeAddressPolicy{allow: true}, dial.DialContext, time.Second)

	connection, err := dialer.DialContext(context.Background(), "tcp", "example.com:443")
	if err != nil {
		t.Fatalf("DialContext() error = %v", err)
	}
	defer connection.Close()

	if len(resolver.deadlines) != 1 || resolver.deadlines[0].IsZero() {
		t.Fatalf("resolver deadlines = %v, want one deadline", resolver.deadlines)
	}
	if len(dial.attempts) != 2 {
		t.Fatalf("dial calls = %d, want 2", len(dial.attempts))
	}
	for _, attempt := range dial.attempts {
		if !attempt.deadline.Equal(resolver.deadlines[0]) {
			t.Fatalf("dial deadline = %v, want %v", attempt.deadline, resolver.deadlines[0])
		}
	}
}

func TestSafeDialerDoesNotExposeDependencyErrors(t *testing.T) {
	tests := []struct {
		name     string
		resolver *fakeResolver
		dial     *fakeDialFunc
		wantErr  error
	}{
		{
			name:     "resolver",
			resolver: &fakeResolver{err: errors.New("lookup https://user:secret@example.com/private")},
			dial:     &fakeDialFunc{},
			wantErr:  ErrResolutionFailed,
		},
		{
			name:     "dial",
			resolver: &fakeResolver{addresses: []netip.Addr{netip.MustParseAddr("1.1.1.1")}},
			dial:     &fakeDialFunc{errors: []error{errors.New("dial https://user:secret@example.com/private")}},
			wantErr:  ErrDialFailed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dialer := NewSafeDialer(tt.resolver, fakeAddressPolicy{allow: true}, tt.dial.DialContext, time.Second)
			connection, err := dialer.DialContext(context.Background(), "tcp", "example.com:443")
			if connection != nil {
				connection.Close()
				t.Fatal("DialContext() connection is non-nil")
			}
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("DialContext() error = %v, want %v", err, tt.wantErr)
			}
			if strings.Contains(err.Error(), "secret") || strings.Contains(err.Error(), "https://") {
				t.Fatalf("DialContext() error exposes dependency error: %q", err)
			}
		})
	}
}
