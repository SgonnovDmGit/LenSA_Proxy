package network

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"strconv"
	"strings"
	"time"
)

var (
	ErrInvalidDestination = errors.New("некорректный адрес назначения")
	ErrResolutionFailed   = errors.New("не удалось разрешить адрес назначения")
	ErrNoAllowedAddresses = errors.New("адрес назначения запрещён")
	ErrDialFailed         = errors.New("не удалось подключиться к адресу назначения")
)

type Resolver interface {
	LookupNetIP(context.Context, string, string) ([]netip.Addr, error)
}

type AddressPolicy interface {
	Allow(netip.Addr) bool
}

type DialContextFunc func(context.Context, string, string) (net.Conn, error)

type SafeDialer struct {
	resolver     Resolver
	policy       AddressPolicy
	dial         DialContextFunc
	totalTimeout time.Duration
}

func NewSafeDialer(resolver Resolver, policy AddressPolicy, dial DialContextFunc, totalTimeout time.Duration) *SafeDialer {
	return &SafeDialer{
		resolver:     resolver,
		policy:       policy,
		dial:         dial,
		totalTimeout: totalTimeout,
	}
}

func (d *SafeDialer) DialContext(ctx context.Context, network, destination string) (net.Conn, error) {
	host, port, err := parseDestination(destination)
	if err != nil {
		return nil, ErrInvalidDestination
	}

	dialContext := ctx
	cancel := func() {}
	if d != nil && d.totalTimeout > 0 {
		dialContext, cancel = context.WithTimeout(ctx, d.totalTimeout)
	}
	defer cancel()

	addresses, err := d.resolve(dialContext, host)
	if err != nil {
		return nil, operationError(ErrResolutionFailed, dialContext)
	}

	allowedAddresses := d.allowedAddresses(addresses)
	if len(allowedAddresses) == 0 {
		return nil, operationError(ErrNoAllowedAddresses, dialContext)
	}
	if d == nil || d.dial == nil {
		return nil, ErrDialFailed
	}

	for _, address := range allowedAddresses {
		if dialContext.Err() != nil {
			return nil, operationError(ErrDialFailed, dialContext)
		}

		literalDestination := netip.AddrPortFrom(address, port).String()
		connection, dialErr := d.dial(dialContext, network, literalDestination)
		if dialErr == nil {
			return connection, nil
		}
		if connection != nil {
			_ = connection.Close()
		}
	}

	return nil, operationError(ErrDialFailed, dialContext)
}

func (d *SafeDialer) resolve(ctx context.Context, host string) ([]netip.Addr, error) {
	if address, err := netip.ParseAddr(host); err == nil {
		return []netip.Addr{address}, nil
	}
	if d == nil || d.resolver == nil {
		return nil, ErrResolutionFailed
	}
	return d.resolver.LookupNetIP(ctx, "ip", host)
}

func (d *SafeDialer) allowedAddresses(addresses []netip.Addr) []netip.Addr {
	if d == nil || d.policy == nil {
		return nil
	}

	ipv4Addresses := make([]netip.Addr, 0, len(addresses))
	ipv6Addresses := make([]netip.Addr, 0, len(addresses))
	seen := make(map[netip.Addr]struct{}, len(addresses))
	for _, address := range addresses {
		address = address.Unmap()
		if !address.IsValid() || !d.policy.Allow(address) {
			continue
		}
		if _, exists := seen[address]; exists {
			continue
		}
		seen[address] = struct{}{}
		if address.Is4() {
			ipv4Addresses = append(ipv4Addresses, address)
		} else {
			ipv6Addresses = append(ipv6Addresses, address)
		}
	}
	return append(ipv4Addresses, ipv6Addresses...)
}

func parseDestination(destination string) (string, uint16, error) {
	host, portValue, err := net.SplitHostPort(destination)
	if err != nil || host == "" || strings.TrimSpace(host) != host || strings.ContainsAny(host, "/\\@?#") {
		return "", 0, ErrInvalidDestination
	}
	port, err := strconv.ParseUint(portValue, 10, 16)
	if err != nil || port == 0 {
		return "", 0, ErrInvalidDestination
	}
	return host, uint16(port), nil
}

func operationError(operationErr error, ctx context.Context) error {
	if contextErr := ctx.Err(); contextErr != nil {
		return errors.Join(operationErr, contextErr)
	}
	return operationErr
}
