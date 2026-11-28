package server

import (
	"context"
	"fmt"
	"net"
	"net/netip"
	"strconv"
	"strings"
	"time"
)

type EgressConfig struct {
	DNSMode    string
	DNSServer  string
	DisableUDP bool
}

func NormalizeDNS(mode, address string) (string, string, error) {
	mode, address = strings.ToLower(strings.TrimSpace(mode)), strings.TrimSpace(address)
	if mode == "" {
		mode = "system"
	}
	switch mode {
	case "system":
		return mode, "", nil
	case "egress":
		if address == "" {
			return mode, "", nil
		}
		if ip, err := netip.ParseAddr(address); err == nil && !ip.IsUnspecified() && !ip.IsMulticast() {
			return mode, net.JoinHostPort(ip.String(), "53"), nil
		}
		host, port, err := net.SplitHostPort(address)
		ip, ipErr := netip.ParseAddr(host)
		n, portErr := strconv.Atoi(port)
		if err == nil && ipErr == nil && portErr == nil && n > 0 && n <= 65535 && !ip.IsUnspecified() && !ip.IsMulticast() {
			return mode, net.JoinHostPort(ip.String(), strconv.Itoa(n)), nil
		}
		return "", "", fmt.Errorf("DNS 服务器必须是有效的 IP 地址，可附带端口")
	default:
		return "", "", fmt.Errorf("DNS 模式仅支持 system 或 egress")
	}
}

func setEgressDNS(dialer *net.Dialer, mode, address string) {
	if mode != "egress" {
		return
	}
	if address == "" {
		address = "1.1.1.1:53"
	}
	dnsDialer := *dialer
	dnsDialer.Timeout = 5 * time.Second
	dialer.Resolver = &net.Resolver{PreferGo: true, Dial: func(ctx context.Context, network, _ string) (net.Conn, error) {
		return dnsDialer.DialContext(ctx, network, address)
	}}
}

// The connection dialer resolves SOCKS destinations. Deferring lookup here
// also avoids resolving a UDP ASSOCIATE client's claimed endpoint.
type deferredSOCKSResolver struct{}

func (deferredSOCKSResolver) Resolve(ctx context.Context, _ string) (context.Context, net.IP, error) {
	return ctx, nil, nil
}
