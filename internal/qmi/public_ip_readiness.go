package qmicore

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/iniwex5/netlink"
	"github.com/iniwex5/vohive/internal/netprobe"
	"github.com/iniwex5/vohive/pkg/logger"
)

const dataPlaneReadyTimeout = 3 * time.Second

// WaitDefaultDataPlaneReady checks the configured default data connection on
// the host after QMI reports it connected. Dual stack can become usable on
// either family since operators may reject the other PDP context.
func (m *Manager) WaitDefaultDataPlaneReady(ctx context.Context) error {
	if m == nil || m.qmiMgr == nil {
		return errors.New("QMI manager is unavailable")
	}
	ctx, cancel := context.WithTimeout(ctx, dataPlaneReadyTimeout)
	defer cancel()
	_, _, err := m.waitDefaultDataPlane(ctx, netprobe.FamilyAny)
	return err
}

func (m *Manager) probePublicIP(family netprobe.Family) string {
	ctx := context.Background()
	iface := strings.TrimSpace(m.cfg.Interface)
	var generation uint64
	if m.qmiMgr != nil {
		wait, cancel := context.WithTimeout(ctx, dataPlaneReadyTimeout)
		defer cancel()
		var err error
		iface, generation, err = m.waitDefaultDataPlane(wait, family)
		if err != nil {
			logger.Debug("QMI 公网 IP 探测等待数据面失败", "device", m.cfg.ID, "family", family, "err", err)
			return ""
		}
	}

	prober := netprobe.New(netprobe.Config{
		Interface: iface,
		URLs:      ipCheckURLs,
		Timeout:   publicIPRequestTimeout,
		Lookup: func(ctx context.Context, host string) ([]string, error) {
			return m.lookupPublicIPHost(ctx, iface, host, family)
		},
	})
	ip := prober.Probe(ctx, family)
	if m.qmiMgr != nil {
		current, ok := m.qmiMgr.CurrentDataPlaneSnapshot()
		if !ok || !m.qmiMgr.IsConnected() || current.Generation != generation || current.DefaultInterface != iface {
			return ""
		}
	}
	return ip
}

func (m *Manager) waitDefaultDataPlane(ctx context.Context, family netprobe.Family) (string, uint64, error) {
	tick := time.NewTicker(150 * time.Millisecond)
	defer tick.Stop()
	for {
		if err := ctx.Err(); err != nil {
			return "", 0, err
		}
		snapshot, ok := m.qmiMgr.CurrentDataPlaneSnapshot()
		var err error
		if !m.qmiMgr.IsConnected() {
			err = errors.New("QMI data connection is down")
		} else if !ok || snapshot.DefaultInterface == "" {
			err = errors.New("QMI default interface is unavailable")
		} else {
			err = checkDefaultDataLink(snapshot.DefaultInterface, family)
			if err == nil {
				current, published := m.qmiMgr.CurrentDataPlaneSnapshot()
				if published && current == snapshot && m.qmiMgr.IsConnected() {
					return snapshot.DefaultInterface, snapshot.Generation, nil
				}
				err = errors.New("QMI default data connection changed during the readiness check")
			}
		}
		select {
		case <-ctx.Done():
			return "", 0, errors.Join(ctx.Err(), err)
		case <-tick.C:
		}
	}
}

func checkDefaultDataLink(iface string, family netprobe.Family) error {
	link, err := netlink.LinkByName(iface)
	if err != nil {
		return fmt.Errorf("default interface %s: %w", iface, err)
	}
	if link.Attrs().Flags&net.FlagUp == 0 {
		return fmt.Errorf("default interface %s is down", iface)
	}
	if family == netprobe.FamilyAny {
		if err := checkDefaultDataFamily(link, netlink.FAMILY_V4); err == nil {
			return nil
		}
	}
	if family == netprobe.FamilyV4 {
		return checkDefaultDataFamily(link, netlink.FAMILY_V4)
	}
	return checkDefaultDataFamily(link, netlink.FAMILY_V6)
}

func checkDefaultDataFamily(link netlink.Link, family int) error {
	addresses, err := netlink.AddrList(link, family)
	if err != nil {
		return fmt.Errorf("data-plane addresses: %w", err)
	}
	for _, address := range addresses {
		if address.IPNet != nil && address.IP.IsGlobalUnicast() {
			routes, err := netlink.RouteList(link, family)
			if err != nil {
				return fmt.Errorf("data-plane routes: %w", err)
			}
			for _, route := range routes {
				if route.LinkIndex == link.Attrs().Index && isPublicIPDefaultRoute(route.Dst) {
					return nil
				}
			}
			return errors.New("data-plane default route is missing")
		}
	}
	return errors.New("data-plane address is missing")
}

func isPublicIPDefaultRoute(destination *net.IPNet) bool {
	if destination == nil {
		return true
	}
	ones, bits := destination.Mask.Size()
	return bits > 0 && ones == 0 && destination.IP.IsUnspecified()
}
