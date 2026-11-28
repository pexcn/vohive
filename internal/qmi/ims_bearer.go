package qmicore

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"strings"
	"sync"
	"time"

	qmimanager "github.com/iniwex5/qmi-go/pkg/manager"
	"github.com/iniwex5/qmi-go/pkg/qmi"
	"github.com/iniwex5/vohive/internal/imsbearer"
	"github.com/iniwex5/vohive/pkg/logger"
	"github.com/iniwex5/vowifi-go/runtimehost"
)

const imsMuxID = 2

type imsEndpoint struct {
	*imsbearer.PacketEndpoint
	pdn       qmimanager.PDNSession
	closeOnce sync.Once
}

func (e *imsEndpoint) Close() error {
	var err error
	e.closeOnce.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()
		// qmi-go releases ownership and caches the first close result even
		// when an individual teardown operation fails.
		err = errors.Join(e.PacketEndpoint.Close(), e.pdn.Close(ctx))
	})
	return err
}

// OpenIMSBearer creates a secondary QMAP session with no host IP addresses
// or routes. The default data connection remains on its own mux.
func (m *Manager) OpenIMSBearer(ctx context.Context, apn, ipStack string) (runtimehost.IMSBearer, error) {
	if m == nil || m.qmiMgr == nil {
		return runtimehost.IMSBearer{}, errors.New("QMI manager is unavailable")
	}
	if apn = strings.TrimSpace(apn); apn == "" {
		apn = "ims"
	}
	var families []uint8
	switch strings.ToLower(ipStack) {
	case "ipv4":
		families = []uint8{4}
	case "ipv6":
		families = []uint8{6}
	case "", "ipv4v6":
		families = []uint8{6, 4}
	default:
		return runtimehost.IMSBearer{}, fmt.Errorf("unsupported IMS IP stack %q", ipStack)
	}
	check, cancel := context.WithTimeout(ctx, 2*time.Second)
	native, err := m.qmiMgr.IMSAGetIMSRegistrationStatus(check)
	cancel()
	if err != nil {
		logger.Debug("模组原生 IMS 状态不可读", "device", m.cfg.ID, "err", err)
	}
	if nativeIMSOwnsWWAN(native) {
		return runtimehost.IMSBearer{}, errors.New("模组原生 IMS 已在蜂窝网络注册，IMS APN 可能已被占用")
	}
	snapshot, ok := m.qmiMgr.CurrentDataPlaneSnapshot()
	if !ok || snapshot.Mode != qmimanager.DataPlaneModeQMAP {
		var err error
		snapshot, err = m.qmiMgr.ConvergeDataPlane(ctx, qmimanager.DataPlaneSpec{Mode: qmimanager.DataPlaneModeQMAP, DefaultMuxID: 1})
		if err != nil {
			return runtimehost.IMSBearer{}, err
		}
		if snapshot.Mode != qmimanager.DataPlaneModeQMAP {
			return runtimehost.IMSBearer{}, fmt.Errorf("VoLTE requires QMAP: %s", snapshot.DegradedReason)
		}
	}
	if snapshot.DefaultMuxID == imsMuxID {
		return runtimehost.IMSBearer{}, errors.New("VoLTE IMS mux overlaps the default data mux")
	}
	var openErr error
	for _, family := range families {
		if err := ctx.Err(); err != nil {
			return runtimehost.IMSBearer{}, err
		}
		callType := qmi.WDSCallTypeEmbedded
		pdn, err := m.qmiMgr.OpenPDN(ctx, qmimanager.PDNRequest{
			APN: apn, MuxID: imsMuxID, IPFamily: family, CallType: &callType,
			EndpointType: 2, UserspaceOnly: true,
		})
		if err != nil {
			openErr = errors.Join(openErr, err)
			if errors.Is(err, qmimanager.ErrPDNStart) {
				continue
			}
			return runtimehost.IMSBearer{}, openErr
		}
		settings := pdn.Snapshot()
		bearer, err := imsBearerSettings(settings.Settings, family)
		if err != nil {
			closeCtx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
			closeErr := pdn.Close(closeCtx)
			cancel()
			if closeErr != nil {
				return runtimehost.IMSBearer{}, errors.Join(err, closeErr)
			}
			openErr = errors.Join(openErr, err)
			continue
		}
		endpoint, err := imsbearer.OpenPacketEndpoint(settings.InterfaceName)
		if err != nil {
			closeCtx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
			defer cancel()
			return runtimehost.IMSBearer{}, errors.Join(err, pdn.Close(closeCtx))
		}
		bearer.PacketEndpoint = &imsEndpoint{PacketEndpoint: endpoint, pdn: pdn}
		locationCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		if loc, err := m.qmiMgr.NASGetCellLocationInfo(locationCtx); err == nil && loc != nil {
			bearer.PANI = ltePANI(loc.LTE)
		}
		cancel()
		return bearer, nil
	}
	return runtimehost.IMSBearer{}, openErr
}

func nativeIMSOwnsWWAN(status *qmi.IMSARegistrationStatus) bool {
	return status != nil && status.HasStatus && status.Status == qmi.IMSARegistrationStateRegistered &&
		status.HasTechnology && status.Technology == qmi.IMSARegistrationTechnologyWWAN
}

func imsBearerSettings(settings qmi.RuntimeSettings, family uint8) (runtimehost.IMSBearer, error) {
	address, dns, pcscf := settings.IPv4Address, []net.IP{settings.IPv4DNS1, settings.IPv4DNS2}, settings.PCSCFv4
	if family == 6 {
		address, dns, pcscf = settings.IPv6Address, []net.IP{settings.IPv6DNS1, settings.IPv6DNS2}, settings.PCSCFv6
	}
	ip, ok := netip.AddrFromSlice(address)
	if !ok || !ip.IsGlobalUnicast() || ip.Unmap().Is4() != (family == 4) {
		return runtimehost.IMSBearer{}, fmt.Errorf("IMS PDN has no IPv%d address", family)
	}
	bearer := runtimehost.IMSBearer{Addresses: []netip.Addr{ip.Unmap()}}
	for _, addr := range dns {
		if v, ok := netip.AddrFromSlice(addr); ok && v.IsGlobalUnicast() && v.Unmap().Is4() == (family == 4) {
			bearer.DNS = append(bearer.DNS, v.Unmap())
		}
	}
	for _, addr := range pcscf {
		if v, ok := netip.AddrFromSlice(addr); ok && v.IsGlobalUnicast() && v.Unmap().Is4() == (family == 4) {
			bearer.PCSCF = append(bearer.PCSCF, v.Unmap().String())
		}
	}
	for _, domain := range settings.PCSCFDomains {
		if domain = strings.TrimSpace(domain); domain != "" {
			bearer.PCSCF = append(bearer.PCSCF, domain)
		}
	}
	if settings.MTU > 0 {
		bearer.MTU = uint32(settings.MTU)
	}
	return bearer, nil
}

func ltePANI(cell *qmi.LTECellLocationInfo) string {
	if cell == nil || !cell.HasEARFCN || cell.GlobalCellID == 0 || cell.GlobalCellID >= 1<<28 ||
		len(cell.MCC) != 3 || len(cell.MNC) < 2 || len(cell.MNC) > 3 || strings.Trim(cell.MCC+cell.MNC, "0123456789") != "" {
		return ""
	}
	duplex := "FDD"
	switch {
	case cell.EARFCN < 36000:
	case cell.EARFCN <= 60254:
		duplex = "TDD"
	default:
		return ""
	}
	return fmt.Sprintf("3GPP-E-UTRAN-%s;utran-cell-id-3gpp=%s%s%04X%07X", duplex, cell.MCC, cell.MNC, cell.TAC, cell.GlobalCellID)
}
