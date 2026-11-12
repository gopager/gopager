package safehttp

import "net/netip"

// forbiddenPrefixes are the ranges that netip's predicates do not cover: the "this network"
// block, carrier-grade NAT, IETF protocol assignments, benchmarking and reserved IPv4 space,
// the documented cloud metadata addresses, and IPv6 transition ranges that tunnel an IPv4
// destination (local-use NAT64, 6to4, Teredo and the deprecated IPv4-compatible form). The
// transition ranges are denied outright because the IPv4 address they reach is chosen by
// relays outside this host, so it cannot be checked here.
var forbiddenPrefixes = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"),
	netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("192.0.0.0/24"),
	netip.MustParsePrefix("198.18.0.0/15"),
	netip.MustParsePrefix("240.0.0.0/4"),
	netip.MustParsePrefix("169.254.169.254/32"),
	netip.MustParsePrefix("fd00:ec2::254/128"),
	netip.MustParsePrefix("64:ff9b:1::/48"),
	netip.MustParsePrefix("2002::/16"),
	netip.MustParsePrefix("2001::/32"),
	netip.MustParsePrefix("::/96"),
}

// nat64Prefix is the well-known NAT64 prefix whose low 32 bits embed an IPv4 address.
var nat64Prefix = netip.MustParsePrefix("64:ff9b::/96")

// forbiddenReason returns why ip may not be contacted, or "" when it is a public address.
// IPv4-mapped and NAT64 IPv6 forms are judged by the IPv4 address they carry.
func forbiddenReason(ip netip.Addr) string {
	ip = ip.Unmap().WithZone("")
	if ip.Is6() && nat64Prefix.Contains(ip) {
		raw := ip.As16()
		return forbiddenReason(netip.AddrFrom4([4]byte{raw[12], raw[13], raw[14], raw[15]}))
	}
	switch {
	case ip.IsLoopback():
		return "loopback address"
	case ip.IsUnspecified():
		return "unspecified address"
	case ip.IsMulticast() || ip.IsLinkLocalMulticast() || ip.IsInterfaceLocalMulticast():
		return "multicast address"
	case ip.IsLinkLocalUnicast():
		return "link-local address"
	case ip.IsPrivate():
		return "private address"
	}
	for _, prefix := range forbiddenPrefixes {
		if prefix.Contains(ip) {
			return "reserved address"
		}
	}
	return ""
}

// deniedPort reports whether the port is a privileged one other than 80 and 443.
func deniedPort(port int) bool {
	return port < 1024 && port != 80 && port != 443
}
