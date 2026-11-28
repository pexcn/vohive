//go:build linux

package imsbearer

import (
	"context"
	"encoding/binary"
	"fmt"
	"net"
	"os"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

type PacketEndpoint struct {
	file    *os.File
	socket  syscall.RawConn
	ifindex int
	hwaddr  [8]byte
	hwlen   uint8
}

func networkProtocol(v uint16) uint16 {
	var b [2]byte
	binary.BigEndian.PutUint16(b[:], v)
	return binary.NativeEndian.Uint16(b[:])
}

// OpenPacketEndpoint attaches a raw-IP datagram socket to an isolated modem
// mux. The kernel must not own the bearer IP addresses.
func OpenPacketEndpoint(iface string) (*PacketEndpoint, error) {
	link, err := net.InterfaceByName(iface)
	if err != nil {
		return nil, err
	}
	protocol := networkProtocol(unix.ETH_P_ALL)
	fd, err := unix.Socket(unix.AF_PACKET, unix.SOCK_DGRAM|unix.SOCK_NONBLOCK|unix.SOCK_CLOEXEC, int(protocol))
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(fd), "ims:"+iface)
	ready := false
	defer func() {
		if !ready {
			_ = file.Close()
		}
	}()
	if err := unix.Bind(fd, &unix.SockaddrLinklayer{Ifindex: link.Index, Protocol: protocol}); err != nil {
		return nil, err
	}
	socket, err := file.SyscallConn()
	if err != nil {
		return nil, err
	}
	ready = true
	endpoint := &PacketEndpoint{file: file, socket: socket, ifindex: link.Index}
	if len(link.HardwareAddr) == 6 {
		endpoint.hwlen = uint8(copy(endpoint.hwaddr[:], link.HardwareAddr))
	}
	return endpoint, nil
}

func (e *PacketEndpoint) Close() error { return e.file.Close() }

func (e *PacketEndpoint) ReadPacket(ctx context.Context) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if deadline, ok := ctx.Deadline(); ok {
		if err := e.file.SetReadDeadline(deadline); err != nil {
			return nil, err
		}
	}
	stop := context.AfterFunc(ctx, func() { _ = e.file.SetReadDeadline(time.Now()) })
	defer stop()
	packet := make([]byte, 65575)
	for {
		var n int
		var from unix.Sockaddr
		var recvErr error
		err := e.socket.Read(func(fd uintptr) bool {
			for {
				n, from, recvErr = unix.Recvfrom(int(fd), packet, unix.MSG_TRUNC)
				if recvErr != unix.EINTR {
					return recvErr != unix.EAGAIN
				}
			}
		})
		if err != nil {
			return nil, err
		}
		if recvErr != nil {
			return nil, recvErr
		}
		source, ok := from.(*unix.SockaddrLinklayer)
		if !ok || source.Pkttype != unix.PACKET_HOST || n == 0 || n > len(packet) {
			continue
		}
		if version := packet[0] >> 4; version == 4 || version == 6 {
			return packet[:n], nil
		}
	}
}

func (e *PacketEndpoint) WritePacket(ctx context.Context, packet []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if len(packet) == 0 {
		return fmt.Errorf("empty IMS IP packet")
	}
	var protocol uint16
	switch packet[0] >> 4 {
	case 4:
		protocol = unix.ETH_P_IP
	case 6:
		protocol = unix.ETH_P_IPV6
	default:
		return fmt.Errorf("invalid IMS IP packet version")
	}
	if deadline, ok := ctx.Deadline(); ok {
		if err := e.file.SetWriteDeadline(deadline); err != nil {
			return err
		}
	}
	stop := context.AfterFunc(ctx, func() { _ = e.file.SetWriteDeadline(time.Now()) })
	defer stop()
	destination := &unix.SockaddrLinklayer{Ifindex: e.ifindex, Protocol: networkProtocol(protocol), Halen: e.hwlen, Addr: e.hwaddr}
	var sendErr error
	err := e.socket.Write(func(fd uintptr) bool {
		for {
			sendErr = unix.Sendto(int(fd), packet, 0, destination)
			if sendErr != unix.EINTR {
				return sendErr != unix.EAGAIN
			}
		}
	})
	if err != nil {
		return err
	}
	return sendErr
}
