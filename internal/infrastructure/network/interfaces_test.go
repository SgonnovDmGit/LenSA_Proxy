package network

import (
	"net"
	"net/netip"
	"reflect"
	"testing"

	"github.com/SgonnovDmGit/LenSA_Proxy/internal/domain/proxy"
)

func TestDiscoverInterfaceAddressesFiltersInterfaceFlags(t *testing.T) {
	address := mustIPNetwork(t, "192.168.1.42/24")
	expected := proxy.NetworkInterface{
		Index:   7,
		Name:    "Ethernet",
		Address: netip.MustParsePrefix("192.168.1.42/24"),
	}

	tests := []struct {
		name           string
		flags          net.Flags
		wantDiscovered bool
	}{
		{name: "up", flags: net.FlagUp, wantDiscovered: true},
		{name: "down"},
		{name: "loopback", flags: net.FlagUp | net.FlagLoopback},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			systemInterface := net.Interface{Index: 7, Name: "Ethernet", Flags: tt.flags}
			got := discoverInterfaceAddresses(systemInterface, []net.Addr{address})
			if tt.wantDiscovered {
				assertNetworkInterfaces(t, got, []proxy.NetworkInterface{expected})
				return
			}
			if len(got) != 0 {
				t.Fatalf("discoverInterfaceAddresses() = %v, want no interfaces", got)
			}
		})
	}
}

func TestDiscoverInterfaceAddressesFiltersAddressTypes(t *testing.T) {
	systemInterface := net.Interface{Index: 4, Name: "Wi-Fi", Flags: net.FlagUp}
	addresses := []net.Addr{
		mustIPNetwork(t, "2001:db8::42/64"),
		mustIPNetwork(t, "172.32.0.1/12"),
		mustIPNetwork(t, "192.168.4.5/24"),
		mustIPNetwork(t, "169.254.10.20/16"),
		mustIPNetwork(t, "172.31.255.254/12"),
		mustIPNetwork(t, "8.8.8.8/24"),
		mustIPNetwork(t, "fd00::42/64"),
		mustIPNetwork(t, "10.1.2.3/8"),
		mustIPNetwork(t, "127.0.0.1/8"),
		mustIPNetwork(t, "172.15.255.254/12"),
		mustIPNetwork(t, "fe80::42/64"),
		mustIPNetwork(t, "172.16.0.1/12"),
	}

	got := discoverInterfaceAddresses(systemInterface, addresses)
	want := []proxy.NetworkInterface{
		{Index: 4, Name: "Wi-Fi", Address: netip.MustParsePrefix("10.1.2.3/8")},
		{Index: 4, Name: "Wi-Fi", Address: netip.MustParsePrefix("172.16.0.1/12")},
		{Index: 4, Name: "Wi-Fi", Address: netip.MustParsePrefix("172.31.255.254/12")},
		{Index: 4, Name: "Wi-Fi", Address: netip.MustParsePrefix("192.168.4.5/24")},
	}
	assertNetworkInterfaces(t, got, want)
}

func TestDiscoverInterfaceAddressesPreservesHostMaskAndMultipleAddresses(t *testing.T) {
	systemInterface := net.Interface{Index: 12, Name: "LAN", Flags: net.FlagUp}
	addresses := []net.Addr{
		mustIPNetwork(t, "192.168.200.123/20"),
		mustIPNetwork(t, "10.23.45.67/16"),
		mustIPNetwork(t, "192.168.200.123/20"),
	}

	got := discoverInterfaceAddresses(systemInterface, addresses)
	want := []proxy.NetworkInterface{
		{Index: 12, Name: "LAN", Address: netip.MustParsePrefix("10.23.45.67/16")},
		{Index: 12, Name: "LAN", Address: netip.MustParsePrefix("192.168.200.123/20")},
	}
	assertNetworkInterfaces(t, got, want)
}

func TestSortAndDeduplicateInterfaces(t *testing.T) {
	interfaces := []proxy.NetworkInterface{
		{Index: 8, Name: "Wi-Fi", Address: netip.MustParsePrefix("192.168.1.20/24")},
		{Index: 2, Name: "Ethernet", Address: netip.MustParsePrefix("192.168.1.10/24")},
		{Index: 2, Name: "Ethernet", Address: netip.MustParsePrefix("10.0.0.2/24")},
		{Index: 1, Name: "VPN", Address: netip.MustParsePrefix("192.168.50.5/24")},
		{Index: 2, Name: "Ethernet", Address: netip.MustParsePrefix("10.0.0.2/8")},
		{Index: 2, Name: "Ethernet", Address: netip.MustParsePrefix("192.168.1.10/24")},
		{Index: 8, Name: "Wi-Fi", Address: netip.MustParsePrefix("192.168.1.20/24")},
	}

	got := sortAndDeduplicateInterfaces(interfaces)
	want := []proxy.NetworkInterface{
		{Index: 1, Name: "VPN", Address: netip.MustParsePrefix("192.168.50.5/24")},
		{Index: 2, Name: "Ethernet", Address: netip.MustParsePrefix("10.0.0.2/8")},
		{Index: 2, Name: "Ethernet", Address: netip.MustParsePrefix("10.0.0.2/24")},
		{Index: 2, Name: "Ethernet", Address: netip.MustParsePrefix("192.168.1.10/24")},
		{Index: 8, Name: "Wi-Fi", Address: netip.MustParsePrefix("192.168.1.20/24")},
	}
	assertNetworkInterfaces(t, got, want)
}

func mustIPNetwork(t *testing.T, cidr string) net.Addr {
	t.Helper()

	ip, network, err := net.ParseCIDR(cidr)
	if err != nil {
		t.Fatalf("net.ParseCIDR(%q) error = %v", cidr, err)
	}
	network.IP = ip
	return network
}

func assertNetworkInterfaces(t *testing.T, got, want []proxy.NetworkInterface) {
	t.Helper()

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("interfaces = %v, want %v", got, want)
	}
}
