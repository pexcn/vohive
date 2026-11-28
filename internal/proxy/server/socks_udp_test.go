package server

import (
	"context"
	"io"
	"net"
	"strconv"
	"testing"
	"time"

	"github.com/miekg/dns"
	"github.com/things-go/go-socks5/statute"
)

func TestSOCKSUDPUsesConfiguredDNSAndPinnedClient(t *testing.T) {
	echo, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	defer echo.Close()
	go func() {
		buf := make([]byte, 2048)
		for {
			n, address, err := echo.ReadFromUDP(buf)
			if err != nil {
				return
			}
			_, _ = echo.WriteToUDP(buf[:n], address)
		}
	}()
	dnsSocket, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer dnsSocket.Close()
	queryReceived := make(chan struct{}, 2)
	dnsServer := &dns.Server{PacketConn: dnsSocket, Handler: dns.HandlerFunc(func(w dns.ResponseWriter, req *dns.Msg) {
		msg := new(dns.Msg).SetReply(req)
		if len(req.Question) == 1 && req.Question[0].Name == "proxy-only.test." && req.Question[0].Qtype == dns.TypeA {
			select {
			case queryReceived <- struct{}{}:
			default:
			}
			msg.Answer = []dns.RR{&dns.A{Hdr: dns.RR_Header{Name: "proxy-only.test.", Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 10}, A: net.IPv4(127, 0, 0, 1)}}
		}
		_ = w.WriteMsg(msg)
	})}
	go func() { _ = dnsServer.ActivateAndServe() }()
	defer dnsServer.ShutdownContext(context.Background())

	srv, err := newServer("test", ModeSocks5, "127.0.0.1", 0, "lo", false, "", "", EgressConfig{DNSMode: "egress", DNSServer: dnsSocket.LocalAddr().String()})
	if err != nil {
		t.Fatal(err)
	}
	exit := make(chan error, 1)
	go func() { exit <- srv.Start() }()
	defer func() {
		_ = srv.Shutdown()
		if err := <-exit; err != nil {
			t.Error(err)
		}
	}()
	var listener net.Listener
	for deadline := time.Now().Add(time.Second); listener == nil; {
		srv.mu.Lock()
		listener = srv.listener
		srv.mu.Unlock()
		if time.Now().After(deadline) {
			t.Fatal("SOCKS listener did not start")
		}
		if listener == nil {
			time.Sleep(5 * time.Millisecond)
		}
	}

	control, err := net.DialTimeout("tcp", listener.Addr().String(), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer control.Close()
	_ = control.SetDeadline(time.Now().Add(4 * time.Second))
	if _, err = control.Write([]byte{5, 1, 0}); err != nil {
		t.Fatal(err)
	}
	var auth [2]byte
	if _, err = io.ReadFull(control, auth[:]); err != nil || auth != [2]byte{5, 0} {
		t.Fatalf("SOCKS greeting %v: %v", auth, err)
	}
	client, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	port := client.LocalAddr().(*net.UDPAddr).Port
	if _, err = control.Write([]byte{5, 3, 0, 1, 0, 0, 0, 0, byte(port >> 8), byte(port)}); err != nil {
		t.Fatal(err)
	}
	reply, err := statute.ParseReply(control)
	if err != nil || reply.Response != statute.RepSuccess {
		t.Fatalf("UDP ASSOCIATE reply: %+v, %v", reply, err)
	}
	relay := &net.UDPAddr{IP: reply.BndAddr.IP, Port: reply.BndAddr.Port}
	address := net.JoinHostPort("proxy-only.test", strconv.Itoa(echo.LocalAddr().(*net.UDPAddr).Port))
	packet, err := statute.NewDatagram(address, []byte("queued via the proxy"))
	if err != nil {
		t.Fatal(err)
	}

	other, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	_, _ = other.WriteToUDP(packet.Bytes(), relay)
	_ = other.SetReadDeadline(time.Now().Add(80 * time.Millisecond))
	buffer := make([]byte, 2048)
	if _, _, err := other.ReadFromUDP(buffer); err == nil {
		t.Fatal("relay accepted another UDP client's port")
	}
	_, _ = client.WriteToUDP(packet.Bytes(), relay)
	_ = client.SetReadDeadline(time.Now().Add(3 * time.Second))
	n, _, err := client.ReadFromUDP(buffer)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := statute.ParseDatagram(buffer[:n])
	if err != nil || string(decoded.Data) != "queued via the proxy" {
		t.Fatalf("relay reply %q: %v", decoded.Data, err)
	}
	select {
	case <-queryReceived:
	default:
		t.Fatal("proxy did not query the configured DNS server")
	}
}
