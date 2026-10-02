package server

import (
	"net"
	"testing"
)

func TestBlockedIP(t *testing.T) {
	for _, ip := range []string{"127.0.0.1", "10.1.2.3", "172.16.0.1", "192.168.1.1", "169.254.169.254", "100.64.0.1", "::1", "fc00::1", "fe80::1", "0.0.0.0"} {
		if !blockedIP(net.ParseIP(ip)) {
			t.Errorf("%s should be blocked", ip)
		}
	}
	for _, ip := range []string{"8.8.8.8", "1.1.1.1", "2606:4700:4700::1111"} {
		if blockedIP(net.ParseIP(ip)) {
			t.Errorf("%s should be allowed", ip)
		}
	}
}

func TestProxyClientRefusesPrivate(t *testing.T) {
	if _, err := proxyClient.Get("http://127.0.0.1:1/"); err == nil {
		t.Fatal("expected loopback to be refused")
	}
}
