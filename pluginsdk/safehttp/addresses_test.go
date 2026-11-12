package safehttp

import (
	"net/netip"
	"testing"
)

func TestForbiddenReason(t *testing.T) {
	t.Parallel()
	denied := []string{
		"127.0.0.1", "127.255.255.254", "::1", "10.0.0.1", "172.16.0.1", "172.31.255.255", "192.168.1.1",
		"fd00::1", "fc00::1", "169.254.169.254", "169.254.0.1", "fe80::1", "fd00:ec2::254",
		"100.64.0.1", "100.127.255.255", "0.0.0.0", "0.1.2.3", "::", "224.0.0.1", "ff02::1", "ff05::2",
		"::ffff:127.0.0.1", "::ffff:10.0.0.1", "::ffff:169.254.169.254", "64:ff9b::7f00:1", "64:ff9b::a9fe:a9fe",
		"192.0.0.1", "198.18.0.1", "198.19.255.255", "240.0.0.1", "255.255.255.255",
		"64:ff9b:1::7f00:1", "2002:7f00:1::1", "2002:a9fe:a9fe::1", "2001:0:4136:e378::1", "::7f00:1", "::a9fe:a9fe",
	}
	for _, text := range denied {
		if forbiddenReason(netip.MustParseAddr(text)) == "" {
			t.Errorf("%s must be forbidden", text)
		}
	}
	allowed := []string{
		"8.8.8.8", "93.184.216.34", "172.15.255.255", "172.32.0.1", "100.63.255.255", "100.128.0.1",
		"2606:4700:4700::1111", "::ffff:8.8.8.8", "64:ff9b::808:808",
	}
	for _, text := range allowed {
		if reason := forbiddenReason(netip.MustParseAddr(text)); reason != "" {
			t.Errorf("%s must be allowed, got %q", text, reason)
		}
	}
}

func TestDeniedPort(t *testing.T) {
	t.Parallel()
	for port, want := range map[int]bool{22: true, 25: true, 1: true, 1023: true, 80: false, 443: false, 1024: false, 8443: false} {
		if got := deniedPort(port); got != want {
			t.Errorf("deniedPort(%d) = %v, want %v", port, got, want)
		}
	}
}
