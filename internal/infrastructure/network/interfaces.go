package network

import (
	"net"
	"net/netip"
	"sort"

	"github.com/SgonnovDmGit/LenSA_Proxy/internal/domain/proxy"
)

func DiscoverInterfaces() ([]proxy.NetworkInterface, error) {
	systemInterfaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}

	discovered := make([]proxy.NetworkInterface, 0)
	for _, systemInterface := range systemInterfaces {
		if !eligibleInterface(systemInterface) {
			continue
		}

		addresses, err := systemInterface.Addrs()
		if err != nil {
			return nil, err
		}
		discovered = append(discovered, discoverInterfaceAddresses(systemInterface, addresses)...)
	}

	return sortAndDeduplicateInterfaces(discovered), nil
}

func discoverInterfaceAddresses(systemInterface net.Interface, addresses []net.Addr) []proxy.NetworkInterface {
	if !eligibleInterface(systemInterface) {
		return nil
	}

	discovered := make([]proxy.NetworkInterface, 0, len(addresses))
	for _, address := range addresses {
		prefix, ok := ipv4Prefix(address)
		if !ok || !prefix.Addr().IsPrivate() {
			continue
		}

		discovered = append(discovered, proxy.NetworkInterface{
			Index:   systemInterface.Index,
			Name:    systemInterface.Name,
			Address: prefix,
		})
	}

	return sortAndDeduplicateInterfaces(discovered)
}

func eligibleInterface(systemInterface net.Interface) bool {
	return systemInterface.Flags&net.FlagUp != 0 && systemInterface.Flags&net.FlagLoopback == 0
}

func ipv4Prefix(address net.Addr) (netip.Prefix, bool) {
	if address == nil {
		return netip.Prefix{}, false
	}

	if ipNetwork, ok := address.(*net.IPNet); ok {
		if ipNetwork == nil {
			return netip.Prefix{}, false
		}

		ones, bits := ipNetwork.Mask.Size()
		if bits != net.IPv4len*8 {
			return netip.Prefix{}, false
		}

		ip, ok := netip.AddrFromSlice(ipNetwork.IP)
		if !ok {
			return netip.Prefix{}, false
		}
		ip = ip.Unmap()
		if !ip.Is4() {
			return netip.Prefix{}, false
		}

		return netip.PrefixFrom(ip, ones), true
	}

	prefix, err := netip.ParsePrefix(address.String())
	if err != nil || !prefix.Addr().Is4() {
		return netip.Prefix{}, false
	}
	return prefix, true
}

func sortAndDeduplicateInterfaces(interfaces []proxy.NetworkInterface) []proxy.NetworkInterface {
	unique := make([]proxy.NetworkInterface, 0, len(interfaces))
	seen := make(map[proxy.NetworkInterface]struct{}, len(interfaces))
	for _, networkInterface := range interfaces {
		if _, ok := seen[networkInterface]; ok {
			continue
		}
		seen[networkInterface] = struct{}{}
		unique = append(unique, networkInterface)
	}

	sort.Slice(unique, func(i, j int) bool {
		left := unique[i]
		right := unique[j]
		if left.Index != right.Index {
			return left.Index < right.Index
		}
		if comparison := left.Address.Addr().Compare(right.Address.Addr()); comparison != 0 {
			return comparison < 0
		}
		if left.Address.Bits() != right.Address.Bits() {
			return left.Address.Bits() < right.Address.Bits()
		}
		return left.Name < right.Name
	})

	return unique
}
