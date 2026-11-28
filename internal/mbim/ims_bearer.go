package mbimcore

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"strings"
	"sync"
	"time"

	"github.com/iniwex5/netlink"
	"github.com/iniwex5/vohive/internal/imsbearer"
	"github.com/iniwex5/vohive/pkg/mbim"
	"github.com/iniwex5/vowifi-go/runtimehost"
)

const (
	imsSessionID              = 1
	ipv6AddressGenerationNone = 1 // IN6_ADDR_GEN_MODE_NONE from linux/if_link.h
	mbimContextNotActivated   = 16
)

type imsSession struct {
	*imsbearer.PacketEndpoint
	closeOnce sync.Once
	release   func() error
}

func (s *imsSession) Close() error {
	var err error
	s.closeOnce.Do(func() {
		err = errors.Join(s.PacketEndpoint.Close(), s.release())
	})
	return err
}

// OpenIMSBearer uses a separate IP session and a cdc_mbim VLAN. The host never
// installs IMS addresses or routes on the VLAN, so gVisor owns its IP traffic.
func (m *Manager) OpenIMSBearer(ctx context.Context, iface, apn, ipStack string) (runtimehost.IMSBearer, error) {
	if m == nil || !m.imsActive.CompareAndSwap(false, true) {
		return runtimehost.IMSBearer{}, errors.New("MBIM IMS session unavailable or already active")
	}
	opened := false
	defer func() {
		if !opened {
			m.imsActive.Store(false)
		}
	}()
	d, err := m.device()
	if err != nil {
		return runtimehost.IMSBearer{}, err
	}
	caps, err := mbim.DeviceCaps(ctx, d)
	if err != nil {
		return runtimehost.IMSBearer{}, err
	}
	if caps.MaxSessions <= imsSessionID {
		return runtimehost.IMSBearer{}, fmt.Errorf("MBIM modem advertises %d IP sessions, need at least 2", caps.MaxSessions)
	}
	link, err := netlink.LinkByName(iface)
	if err != nil {
		return runtimehost.IMSBearer{}, err
	}
	vlan := &netlink.Vlan{
		LinkAttrs: netlink.LinkAttrs{
			Name: fmt.Sprintf("vhims%d", link.Attrs().Index), ParentIndex: link.Attrs().Index, Alias: "vohive:mbim-ims",
		},
		VlanId: imsSessionID,
	}
	if err := netlink.LinkAdd(vlan); err != nil {
		return runtimehost.IMSBearer{}, fmt.Errorf("create dedicated MBIM session link: %w", err)
	}
	var activateAttempt bool
	release := func() error {
		var err error
		if activateAttempt {
			cleanupCtx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
			defer cancel()
			_, err = mbim.SetConnect(cleanupCtx, d, imsSessionID, mbim.ActivationCommandDeactivate, "", "", "", mbim.AuthProtocolNone, mbim.ContextIPTypeDefault, mbim.UUIDContextTypeIMS)
			if isConnectStatus(err, mbimContextNotActivated) {
				err = nil
			}
		}
		return errors.Join(err, netlink.LinkDel(vlan))
	}
	cleanupOnFailure := func(resultErr error) (runtimehost.IMSBearer, error) {
		return runtimehost.IMSBearer{}, errors.Join(resultErr, release())
	}
	if err := netlink.LinkSetIP6AddrGenMode(vlan, ipv6AddressGenerationNone); err != nil {
		return cleanupOnFailure(err)
	}
	if err := netlink.LinkSetUp(link); err != nil {
		return cleanupOnFailure(err)
	}
	if err := netlink.LinkSetUp(vlan); err != nil {
		return cleanupOnFailure(err)
	}
	endpoint, err := imsbearer.OpenPacketEndpoint(vlan.Name)
	if err != nil {
		return cleanupOnFailure(err)
	}
	var ipType uint32
	switch strings.ToLower(strings.TrimSpace(ipStack)) {
	case "ipv4":
		ipType = mbim.ContextIPTypeIPv4
	case "ipv6":
		ipType = mbim.ContextIPTypeIPv6
	case "", "ipv4v6":
		ipType = mbim.ContextIPTypeIPv4v6
	default:
		return cleanupOnFailure(errors.Join(endpoint.Close(), fmt.Errorf("unsupported IMS IP stack %q", ipStack)))
	}
	if apn = strings.TrimSpace(apn); apn == "" {
		apn = "ims"
	}
	activateAttempt = true
	status, err := mbim.SetConnect(ctx, d, imsSessionID, mbim.ActivationCommandActivate, apn, "", "", mbim.AuthProtocolNone, ipType, mbim.UUIDContextTypeIMS)
	if err != nil {
		return cleanupOnFailure(errors.Join(err, endpoint.Close()))
	}
	if status.SessionID != imsSessionID || status.ActivationState != mbim.ActivationStateActivated || status.NwError != 0 {
		return cleanupOnFailure(errors.Join(fmt.Errorf("MBIM IMS activation state %d, network error %d", status.ActivationState, status.NwError), endpoint.Close()))
	}
	queryCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	var bearer runtimehost.IMSBearer
	for {
		ip, err := mbim.QueryIPConfiguration(queryCtx, d, imsSessionID)
		if err != nil {
			return cleanupOnFailure(errors.Join(err, endpoint.Close()))
		}
		bearer = mbimIMSBearerConfig(ip, link.Attrs().MTU)
		if len(bearer.Addresses) != 0 {
			break
		}
		if err := sleepContext(queryCtx, 150*time.Millisecond); err != nil {
			return cleanupOnFailure(errors.Join(fmt.Errorf("MBIM IMS session did not assign an IP address: %w", err), endpoint.Close()))
		}
	}
	if caps := m.Capability(); caps != nil && caps.Services.Supports(mbim.UUIDMSBasicConnectExtensions, mbim.CIDMSBasicConnectExtPCO) {
		queryCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		pcscf, _ := mbim.QuerySessionPCSCF(queryCtx, d, imsSessionID)
		cancel()
		for _, server := range pcscf {
			bearer.PCSCF = append(bearer.PCSCF, server.String())
		}
	}
	if caps := m.Capability(); caps != nil && caps.Services.Supports(mbim.UUIDMSBasicConnectExtensions, mbim.CIDMSBasicConnectExtBaseStationsInfo) {
		cellCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		cell, err := mbim.QueryServingLTECell(cellCtx, d)
		cancel()
		if err == nil {
			bearer.PANI = imsbearer.LTEPANI(cell.PLMN, cell.TAC, cell.CellID, cell.EARFCN)
		}
	}
	session := &imsSession{PacketEndpoint: endpoint, release: func() error {
		defer m.imsActive.Store(false)
		m.mu.Lock()
		m.imsSession = nil
		m.mu.Unlock()
		return release()
	}}
	m.mu.Lock()
	m.imsSession = session
	m.mu.Unlock()
	bearer.PacketEndpoint = session
	opened = true
	return bearer, nil
}

func mbimIMSBearerConfig(ip mbim.IPConfiguration, mtu int) runtimehost.IMSBearer {
	b := runtimehost.IMSBearer{}
	if mtu > 0 {
		b.MTU = uint32(mtu)
	}
	for _, server := range append(append([]string(nil), ip.IPv4DNS...), ip.IPv6DNS...) {
		if addr, err := netip.ParseAddr(server); err == nil && addr.IsGlobalUnicast() {
			b.DNS = append(b.DNS, addr.Unmap())
		}
	}
	for _, candidate := range []string{ip.IPv4Address, ip.IPv6Address} {
		if addr, err := netip.ParseAddr(candidate); err == nil && addr.IsGlobalUnicast() {
			b.Addresses = append(b.Addresses, addr.Unmap())
		}
	}
	for _, offered := range []uint32{ip.IPv4MTU, ip.IPv6MTU} {
		if offered > 0 && (b.MTU == 0 || offered < b.MTU) {
			b.MTU = offered
		}
	}
	return b
}
