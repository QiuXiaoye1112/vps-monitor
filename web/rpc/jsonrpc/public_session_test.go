package jsonrpc

import (
	"testing"

	"github.com/monitor-monitor/monitor/database/models"
)

func TestPublicClientInfoForSessionProtectsIPAddresses(t *testing.T) {
	node := models.Client{
		IPv4: "192.0.2.10",
		IPv6: "2001:db8::10",
	}

	guest := publicClientInfoForSession(node, false)
	if guest["ipv4"] != "" || guest["ipv6"] != "" {
		t.Fatalf("guest must not receive node IP addresses: %#v", guest)
	}

	authenticated := publicClientInfoForSession(node, true)
	if authenticated["ipv4"] != node.IPv4 || authenticated["ipv6"] != node.IPv6 {
		t.Fatalf("authenticated session must receive node IP addresses: %#v", authenticated)
	}
}
