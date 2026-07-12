package network

import (
	"net/netip"
	"testing"
)

func TestPublicAddressPolicyBlocksNonPublicAddresses(t *testing.T) {
	policy := NewPublicAddressPolicy(nil)

	tests := []struct {
		name    string
		address netip.Addr
	}{
		{name: "invalid", address: netip.Addr{}},
		{name: "unspecified IPv4", address: netip.MustParseAddr("0.0.0.0")},
		{name: "current network IPv4", address: netip.MustParseAddr("0.1.2.3")},
		{name: "unspecified IPv6", address: netip.MustParseAddr("::")},
		{name: "loopback IPv4", address: netip.MustParseAddr("127.0.0.1")},
		{name: "loopback IPv6", address: netip.MustParseAddr("::1")},
		{name: "RFC1918 10", address: netip.MustParseAddr("10.1.2.3")},
		{name: "RFC1918 172", address: netip.MustParseAddr("172.16.2.3")},
		{name: "RFC1918 192", address: netip.MustParseAddr("192.168.2.3")},
		{name: "ULA", address: netip.MustParseAddr("fd12:3456::1")},
		{name: "link-local IPv4", address: netip.MustParseAddr("169.254.10.20")},
		{name: "link-local IPv6", address: netip.MustParseAddr("fe80::1")},
		{name: "multicast IPv4", address: netip.MustParseAddr("239.1.2.3")},
		{name: "multicast IPv6", address: netip.MustParseAddr("ff02::1")},
		{name: "CGNAT", address: netip.MustParseAddr("100.64.1.2")},
		{name: "IETF protocol assignments", address: netip.MustParseAddr("192.0.0.8")},
		{name: "documentation IPv4 TEST-NET-1", address: netip.MustParseAddr("192.0.2.1")},
		{name: "deprecated relay IPv4", address: netip.MustParseAddr("192.88.99.1")},
		{name: "documentation IPv4 TEST-NET-2", address: netip.MustParseAddr("198.51.100.1")},
		{name: "documentation IPv4 TEST-NET-3", address: netip.MustParseAddr("203.0.113.1")},
		{name: "documentation IPv6", address: netip.MustParseAddr("2001:db8::1")},
		{name: "documentation IPv6 second range", address: netip.MustParseAddr("3fff::1")},
		{name: "benchmarking IPv4", address: netip.MustParseAddr("198.18.1.2")},
		{name: "benchmarking IPv6", address: netip.MustParseAddr("2001:2::1")},
		{name: "reserved IPv4", address: netip.MustParseAddr("240.0.0.1")},
		{name: "reserved IPv6 discard-only", address: netip.MustParseAddr("100::1")},
		{name: "reserved IPv6 unallocated", address: netip.MustParseAddr("4000::1")},
		{name: "NAT64", address: netip.MustParseAddr("64:ff9b::808:808")},
		{name: "local-use translation IPv6", address: netip.MustParseAddr("64:ff9b:1::1")},
		{name: "IETF protocol assignments IPv6", address: netip.MustParseAddr("2001::1")},
		{name: "6to4", address: netip.MustParseAddr("2002:0808:0808::1")},
		{name: "segment routing IPv6", address: netip.MustParseAddr("5f00::1")},
		{name: "IPv4-mapped private", address: netip.MustParseAddr("::ffff:10.0.0.1")},
		{name: "zoned address", address: netip.MustParseAddr("fe80::1%Ethernet")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if policy.Allow(tt.address) {
				t.Fatalf("Allow(%v) = true, want false", tt.address)
			}
		})
	}
}

func TestPublicAddressPolicyBlocksExactLocalAddresses(t *testing.T) {
	localIPv4 := netip.MustParseAddr("8.8.4.4")
	localIPv6 := netip.MustParseAddr("2606:4700:4700::1001")
	policy := NewPublicAddressPolicy([]netip.Addr{
		netip.MustParseAddr("::ffff:8.8.4.4"),
		localIPv6,
	})

	if policy.Allow(localIPv4) {
		t.Fatalf("Allow(%v) = true, want false", localIPv4)
	}
	if policy.Allow(localIPv6) {
		t.Fatalf("Allow(%v) = true, want false", localIPv6)
	}
	if adjacent := netip.MustParseAddr("8.8.4.5"); !policy.Allow(adjacent) {
		t.Fatalf("Allow(%v) = false, want true", adjacent)
	}
}

func TestPublicAddressPolicyAllowsPublicAddresses(t *testing.T) {
	policy := NewPublicAddressPolicy(nil)
	addresses := []netip.Addr{
		netip.MustParseAddr("1.1.1.1"),
		netip.MustParseAddr("8.8.8.8"),
		netip.MustParseAddr("2001:4860:4860::8888"),
		netip.MustParseAddr("2606:4700:4700::1111"),
		netip.MustParseAddr("::ffff:8.8.8.8"),
	}

	for _, address := range addresses {
		if !policy.Allow(address) {
			t.Errorf("Allow(%v) = false, want true", address)
		}
	}
}
