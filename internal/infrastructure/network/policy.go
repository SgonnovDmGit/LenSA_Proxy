package network

import (
	"errors"
	"net"
	"net/netip"
)

var ErrLocalAddressLookup = errors.New("не удалось получить локальные сетевые адреса")

var blockedAddressPrefixes = [...]netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"),
	netip.MustParsePrefix("10.0.0.0/8"),
	netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("127.0.0.0/8"),
	netip.MustParsePrefix("169.254.0.0/16"),
	netip.MustParsePrefix("172.16.0.0/12"),
	netip.MustParsePrefix("192.0.0.0/24"),
	netip.MustParsePrefix("192.0.2.0/24"),
	netip.MustParsePrefix("192.88.99.0/24"),
	netip.MustParsePrefix("192.168.0.0/16"),
	netip.MustParsePrefix("198.18.0.0/15"),
	netip.MustParsePrefix("198.51.100.0/24"),
	netip.MustParsePrefix("203.0.113.0/24"),
	netip.MustParsePrefix("224.0.0.0/4"),
	netip.MustParsePrefix("240.0.0.0/4"),
	netip.MustParsePrefix("::/128"),
	netip.MustParsePrefix("::1/128"),
	netip.MustParsePrefix("::ffff:0:0/96"),
	netip.MustParsePrefix("64:ff9b::/96"),
	netip.MustParsePrefix("64:ff9b:1::/48"),
	netip.MustParsePrefix("100::/64"),
	netip.MustParsePrefix("2001::/23"),
	netip.MustParsePrefix("2001:db8::/32"),
	netip.MustParsePrefix("2002::/16"),
	netip.MustParsePrefix("3fff::/20"),
	netip.MustParsePrefix("5f00::/16"),
	netip.MustParsePrefix("fc00::/7"),
	netip.MustParsePrefix("fe80::/10"),
	netip.MustParsePrefix("ff00::/8"),
}

var publicIPv6Prefix = netip.MustParsePrefix("2000::/3")

type PublicAddressPolicy struct {
	localAddresses map[netip.Addr]struct{}
}

func NewPublicAddressPolicy(localAddresses []netip.Addr) *PublicAddressPolicy {
	policy := &PublicAddressPolicy{
		localAddresses: make(map[netip.Addr]struct{}, len(localAddresses)),
	}
	for _, address := range localAddresses {
		address = normalizedAddress(address)
		if address.IsValid() {
			policy.localAddresses[address] = struct{}{}
		}
	}
	return policy
}

func NewSystemPublicAddressPolicy() (*PublicAddressPolicy, error) {
	localAddresses, err := systemLocalAddresses()
	if err != nil {
		return nil, err
	}
	return NewPublicAddressPolicy(localAddresses), nil
}

func (p *PublicAddressPolicy) Allow(address netip.Addr) bool {
	if p == nil || !address.IsValid() || address.Zone() != "" {
		return false
	}

	address = address.Unmap()
	if address.IsUnspecified() || address.IsLoopback() || address.IsPrivate() ||
		address.IsLinkLocalUnicast() || address.IsMulticast() || !address.IsGlobalUnicast() {
		return false
	}
	if address.Is6() && !publicIPv6Prefix.Contains(address) {
		return false
	}
	for _, prefix := range blockedAddressPrefixes {
		if prefix.Contains(address) {
			return false
		}
	}
	_, local := p.localAddresses[address]
	return !local
}

func systemLocalAddresses() ([]netip.Addr, error) {
	interfaceAddresses, err := net.InterfaceAddrs()
	if err != nil {
		return nil, ErrLocalAddressLookup
	}

	localAddresses := make([]netip.Addr, 0, len(interfaceAddresses))
	for _, interfaceAddress := range interfaceAddresses {
		if prefix, parseErr := netip.ParsePrefix(interfaceAddress.String()); parseErr == nil {
			localAddresses = append(localAddresses, normalizedAddress(prefix.Addr()))
			continue
		}
		if address, parseErr := netip.ParseAddr(interfaceAddress.String()); parseErr == nil {
			localAddresses = append(localAddresses, normalizedAddress(address))
		}
	}
	return localAddresses, nil
}

func normalizedAddress(address netip.Addr) netip.Addr {
	if !address.IsValid() {
		return address
	}
	address = address.Unmap()
	if address.Zone() != "" {
		address = address.WithZone("")
	}
	return address
}
