package server

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"strconv"
	"strings"
	"time"

	"github.com/miekg/dns"
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
		items := strings.Split(strings.ReplaceAll(address, "，", ","), ",")
		if len(items) > 8 {
			return "", "", errors.New("DNS 服务器最多填写 8 个")
		}
		servers := make([]string, 0, len(items))
		seen := make(map[string]bool, len(items))
		for _, item := range items {
			addr, err := normalizeDNSServer(item)
			if err != nil {
				return "", "", err
			}
			if !seen[addr] {
				servers = append(servers, addr)
				seen[addr] = true
			}
		}
		return mode, strings.Join(servers, ","), nil
	default:
		return "", "", fmt.Errorf("DNS 模式仅支持 system 或 egress")
	}
}

func normalizeDNSServer(address string) (string, error) {
	address = strings.TrimSpace(address)
	if ip, err := netip.ParseAddr(address); err == nil && !ip.IsUnspecified() && !ip.IsMulticast() {
		return net.JoinHostPort(ip.String(), "53"), nil
	}
	host, port, err := net.SplitHostPort(address)
	ip, ipErr := netip.ParseAddr(host)
	n, portErr := strconv.Atoi(port)
	if err == nil && ipErr == nil && portErr == nil && n > 0 && n <= 65535 && !ip.IsUnspecified() && !ip.IsMulticast() {
		return net.JoinHostPort(ip.String(), strconv.Itoa(n)), nil
	}
	return "", fmt.Errorf("DNS 服务器必须是有效的 IP 地址，可附带端口")
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
	servers := strings.Split(address, ",")
	dialer.Resolver = &net.Resolver{PreferGo: true, Dial: func(ctx context.Context, network, _ string) (net.Conn, error) {
		if len(servers) == 1 {
			return dnsDialer.DialContext(ctx, network, servers[0])
		}

		// A stream lets the standard resolver keep using its DNS framing and
		// leaves the target connection's IPv4/IPv6 fallback to net.Dialer.
		local, relay := net.Pipe()
		go func() {
			defer relay.Close()
			conn := &dns.Conn{Conn: relay}
			query, err := conn.ReadMsg()
			if err == nil {
				_ = conn.WriteMsg(queryEgressDNS(ctx, &dnsDialer, network, servers, query))
			}
		}()
		return local, nil
	}}
}

func queryEgressDNS(ctx context.Context, dialer *net.Dialer, network string, servers []string, query *dns.Msg) *dns.Msg {
	for i, server := range servers {
		// UDP writes can succeed even if the server never replies.
		budget := 2 * time.Second
		if deadline, ok := ctx.Deadline(); ok {
			budget = min(budget, time.Until(deadline)/time.Duration(len(servers)-i))
		}
		queryCtx, cancel := context.WithTimeout(ctx, budget)
		client := dns.Client{Net: network, Dialer: dialer}
		reply, _, err := client.ExchangeContext(queryCtx, query, server)
		if err == nil && reply.Truncated && strings.HasPrefix(network, "udp") {
			client.Net = "tcp" + strings.TrimPrefix(network, "udp")
			reply, _, err = client.ExchangeContext(queryCtx, query, server)
		}
		cancel()
		if err == nil && (reply.Rcode == dns.RcodeSuccess || reply.Rcode == dns.RcodeNameError) {
			return reply
		}
		if ctx.Err() != nil {
			break
		}
	}
	return new(dns.Msg).SetRcode(query, dns.RcodeServerFailure)
}

// The connection dialer resolves SOCKS destinations. Deferring lookup here
// also avoids resolving a UDP ASSOCIATE client's claimed endpoint.
type deferredSOCKSResolver struct{}

func (deferredSOCKSResolver) Resolve(ctx context.Context, _ string) (context.Context, net.IP, error) {
	return ctx, nil, nil
}
