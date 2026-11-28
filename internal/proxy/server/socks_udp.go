package server

import (
	"context"
	"io"
	"net"
	"net/netip"
	"strings"
	"sync"
	"time"

	socks5 "github.com/things-go/go-socks5"
	"github.com/things-go/go-socks5/statute"
)

const (
	maxUDPAssociations = 128
	maxUDPTargets      = 32
	udpIdleTimeout     = 90 * time.Second
)

func udpAssociateHandler(lifetime context.Context, dialer *net.Dialer, stats *TrafficStats, disabled bool) func(context.Context, io.Writer, *socks5.Request) error {
	slots := make(chan struct{}, maxUDPAssociations)
	return func(_ context.Context, writer io.Writer, request *socks5.Request) error {
		if disabled {
			return socks5.SendReply(writer, statute.RepCommandNotSupported, nil)
		}
		control, ok := writer.(net.Conn)
		peer, peerOK := request.RemoteAddr.(*net.TCPAddr)
		local, localOK := request.LocalAddr.(*net.TCPAddr)
		if !ok || !peerOK || !localOK || lifetime.Err() != nil {
			return socks5.SendReply(writer, statute.RepServerFailure, nil)
		}
		if request.RawDestAddr.IP != nil && !request.RawDestAddr.IP.IsUnspecified() && !request.RawDestAddr.IP.Equal(peer.IP) {
			return socks5.SendReply(writer, statute.RepRuleFailure, nil)
		}
		select {
		case slots <- struct{}{}:
			defer func() { <-slots }()
		default:
			return socks5.SendReply(writer, statute.RepServerFailure, nil)
		}

		relay, err := net.ListenUDP("udp", &net.UDPAddr{IP: local.IP, Zone: local.Zone})
		if err != nil {
			return socks5.SendReply(writer, statute.RepServerFailure, nil)
		}
		defer relay.Close()
		if err := socks5.SendReply(writer, statute.RepSuccess, relay.LocalAddr()); err != nil {
			return err
		}
		ctx, cancel := context.WithCancel(lifetime)
		stop := context.AfterFunc(ctx, func() {
			_ = relay.Close()
			_ = control.Close()
		})
		defer stop()
		defer cancel()
		defer control.Close()
		go func() {
			_, _ = io.Copy(io.Discard, request.Reader)
			cancel()
		}()

		stats.IncrConnection()
		defer stats.DecrActiveConn()
		runUDPAssociation(ctx, relay, peer.AddrPort().Addr().Unmap(), request.RawDestAddr.Port, dialer, stats)
		return nil
	}
}

func runUDPAssociation(ctx context.Context, relay *net.UDPConn, peer netip.Addr, port int, dialer *net.Dialer, stats *TrafficStats) {
	var client netip.AddrPort
	type target struct {
		conn net.Conn
		at   time.Time
	}
	targets := make(map[string]target)
	var readers sync.WaitGroup
	defer func() {
		for _, target := range targets {
			_ = target.conn.Close()
		}
		readers.Wait()
	}()
	buffer := make([]byte, 65535)
	_ = relay.SetReadDeadline(time.Now().Add(udpIdleTimeout))
	for ctx.Err() == nil {
		n, source, err := relay.ReadFromUDPAddrPort(buffer)
		if err != nil {
			return
		}
		source = netip.AddrPortFrom(source.Addr().Unmap(), source.Port())
		if source.Addr() != peer || (port != 0 && int(source.Port()) != port) ||
			(client.IsValid() && client != source) || n < 4 || buffer[0] != 0 || buffer[1] != 0 || buffer[2] != 0 {
			continue
		}
		packet, err := statute.ParseDatagram(buffer[:n])
		if err != nil || packet.DstAddr.Port == 0 ||
			(packet.DstAddr.FQDN == "" && (packet.DstAddr.IP == nil || packet.DstAddr.IP.IsUnspecified() || packet.DstAddr.IP.IsMulticast())) {
			continue
		}
		client = source
		_ = relay.SetReadDeadline(time.Now().Add(udpIdleTimeout))
		address := packet.DstAddr.String()
		now := time.Now()
		for key, target := range targets {
			if now.Sub(target.at) >= udpIdleTimeout {
				_ = target.conn.Close()
				delete(targets, key)
			}
		}
		entry, exists := targets[address]
		if !exists {
			if len(targets) >= maxUDPTargets {
				continue
			}
			dialCtx, finish := context.WithTimeout(ctx, 5*time.Second)
			conn, err := dialer.DialContext(dialCtx, "udp", address)
			finish()
			if err != nil {
				continue
			}
			entry.conn = conn
			readers.Go(func() { relayUDPReplies(relay, conn, source, stats) })
		}
		entry.at = now
		targets[address] = entry
		if n, err := entry.conn.Write(packet.Data); err == nil {
			stats.AddSent(int64(n))
		} else {
			_ = entry.conn.Close()
			delete(targets, address)
		}
	}
}

func relayUDPReplies(relay *net.UDPConn, conn net.Conn, client netip.AddrPort, stats *TrafficStats) {
	buffer := make([]byte, 65535)
	peer := conn.RemoteAddr().String()
	// Strip an optional IPv6 zone from the reply address; the SOCKS wire
	// format contains just the IPv6 bytes and port.
	if host, port, err := net.SplitHostPort(peer); err == nil {
		host, _, _ = strings.Cut(host, "%")
		peer = net.JoinHostPort(host, port)
	}
	for {
		n, err := conn.Read(buffer)
		if err != nil {
			return
		}
		reply, err := statute.NewDatagram(peer, buffer[:n])
		if err != nil {
			return
		}
		if _, err := relay.WriteToUDPAddrPort(reply.Bytes(), client); err != nil {
			return
		}
		stats.AddReceived(int64(n))
	}
}
