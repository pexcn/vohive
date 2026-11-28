package device

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/iniwex5/vohive/internal/backend"
	"github.com/iniwex5/vohive/internal/db"
	"github.com/iniwex5/vohive/internal/modem"
	"github.com/iniwex5/vohive/pkg/logger"
	"github.com/iniwex5/vowifi-go/runtimehost"
	"github.com/iniwex5/vowifi-go/runtimehost/carrier"
	"github.com/iniwex5/vowifi-go/runtimehost/identity"
)

func (p *Pool) buildVoWiFiStartProfile(worker *Worker, traceID string) (identity.Profile, error) {
	if worker == nil {
		return identity.Profile{}, fmt.Errorf("worker_nil")
	}
	if worker.Backend == nil {
		return identity.Profile{}, fmt.Errorf("backend_not_available")
	}

	reader, ok := worker.Backend.(liveSIMIdentityReader)
	if !ok {
		return identity.Profile{}, fmt.Errorf("live_identity_not_supported")
	}

	liveCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cardCtx, cancelCard := context.WithTimeout(liveCtx, time.Second)
	firstICCID, firstCardErr := reader.GetICCIDLive(cardCtx)
	cancelCard()
	firstICCID = db.CanonicalICCID(firstICCID)
	imsi, err := reader.GetIMSILive(liveCtx)
	if err != nil {
		return identity.Profile{}, fmt.Errorf("实时读取 IMSI 失败: %w", err)
	}
	imsi, err = backend.NormalizeIMSI(imsi)
	if err != nil {
		return identity.Profile{}, fmt.Errorf("实时 IMSI 不可用: %w", err)
	}
	if imsi == "" {
		return identity.Profile{}, fmt.Errorf("实时 IMSI 为空")
	}

	status := worker.ProjectDeviceStatus()
	iccid, cardErr := reader.GetICCIDLive(liveCtx)
	iccid = db.CanonicalICCID(iccid)
	if cardErr == nil && firstCardErr == nil && iccid != "" && firstICCID != "" && firstICCID != iccid {
		return identity.Profile{}, fmt.Errorf("VoWiFi 启动期间 SIM 身份发生变化")
	}
	if cardErr != nil || iccid == "" {
		iccid = ""
		if cached := db.CanonicalICCID(status.ICCID); strings.TrimSpace(status.IMSI) == imsi && (firstICCID == "" || firstICCID == cached) {
			iccid = cached
		}
	}
	status.ICCID = iccid
	mcc, mnc, plmnSource, homeIMSI := resolveVoWiFiProfileMCCMNC(liveCtx, worker, status, imsi, traceID)
	cardVerified := cardErr == nil && firstCardErr == nil && iccid != "" && firstICCID == iccid
	if plmnSource == "sim_home" && cardVerified {
		if err := db.RecordSIMHomeIdentity(iccid, imsi, mcc, mnc); err != nil {
			logger.Debug("保存 SIM 归属身份失败", "trace_id", traceID, "err", err)
		} else if home, err := db.GetSIMHomeIdentity(iccid); err == nil {
			homeIMSI = home.IMSI
		}
	}
	if mcc == "" || mnc == "" {
		return identity.Profile{}, fmt.Errorf("缺少 SIM 归属 MCC/MNC，无法构建 VoWiFi 启动画像: %s", imsi)
	}

	imei := strings.TrimSpace(status.IMEI)

	smscCtx, smscCancel := context.WithTimeout(context.Background(), 5*time.Second)
	smsc, smscErr := worker.getSMSCWithContext(smscCtx)
	smscCancel()
	smsc = strings.TrimSpace(smsc)
	switch {
	case smsc != "":
		logger.Info("VoWiFi 启动前获取 SMSC 成功", "trace_id", traceID, "device", worker.ID, "smsc", smsc)
	case smscErr != nil:
		logger.Warn("VoWiFi 启动前获取 SMSC 失败，将以空 SMSC 继续启动", "trace_id", traceID, "device", worker.ID, "err", smscErr)
	default:
		logger.Warn("VoWiFi 启动前未获取到 SMSC，将以空 SMSC 继续启动", "trace_id", traceID, "device", worker.ID)
	}

	logger.Info("VoWiFi 启动画像将基于实时 IMSI 构建",
		"trace_id", traceID,
		"device", worker.ID,
		"source", "live_imsi",
		"plmn_source", plmnSource,
		"native_mcc", strings.TrimSpace(status.NativeMCC),
		"native_mnc", strings.TrimSpace(status.NativeMNC),
		"iccid", iccid,
		"imsi", imsi,
		"mcc", mcc,
		"mnc", mnc,
		"imei", imei)

	profile := buildVoWiFiRawProfile(imsi, mcc, mnc, imei, smsc)
	profile.ICCID = iccid
	profile.HomeIMSI = homeIMSI
	withSPN, withGID := carrier.SIMOverrideInputs(mcc, mnc)
	if withSPN || withGID {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if withSPN {
			profile.SPN, _ = worker.Backend.GetNativeSPN(ctx)
		}
		if withGID {
			if metadata, _ := worker.Backend.GetSIMMetadata(ctx); metadata != nil {
				profile.GID1, profile.GID2 = metadata.GID1, metadata.GID2
			}
		}
		current, err := reader.GetICCIDLive(ctx)
		if iccid != "" && err == nil && db.CanonicalICCID(current) != iccid {
			return identity.Profile{}, fmt.Errorf("读取 VoWiFi 运营商配置期间 SIM 身份发生变化")
		}
		if iccid == "" || err != nil {
			profile.SPN, profile.GID1, profile.GID2 = "", "", ""
		}
	}
	if cardVerified {
		p.prepareCarrierIdentities(worker, reader, &profile)
	}
	return profile, nil
}

func (p *Pool) prepareCarrierIdentities(worker *Worker, reader liveSIMIdentityReader, profile *identity.Profile) {
	cfg, err := carrier.ResolveCarrierConfigForSIM(carrier.EffectiveCarrierConfigInput{
		MCC: profile.MCC, MNC: profile.MNC, IMSI: profile.IMSI, ICCID: profile.ICCID,
		SPN: profile.SPN, GID1: profile.GID1, GID2: profile.GID2,
	})
	if err != nil {
		return
	}
	if format := cfg.Parameters.IMSPublicIdentityFormat; format == "msisdn" || format == "+msisdn" {
		ctx, cancel := context.WithTimeout(p.Context(), 3*time.Second)
		profile.MSISDN, _ = db.GetPhoneNumberByIMSIOrICCID(profile.IMSI, profile.ICCID)
		if profile.MSISDN == "" {
			number, err := worker.Backend.GetMSISDN(ctx)
			if iccid, cardErr := reader.GetICCIDLive(ctx); err == nil && cardErr == nil && db.CanonicalICCID(iccid) == profile.ICCID {
				profile.MSISDN = number
			}
		}
		cancel()
	}
	plmn := cfg.VoWiFiHomePLMN
	if worker.voLTERequested() || !strings.EqualFold(strings.TrimSpace(cfg.VoWiFiIMSI), "capture") ||
		(len(plmn) != 5 && len(plmn) != 6) || strings.HasPrefix(profile.HomeIMSI, plmn) {
		return
	}
	if imsi := p.observeVoWiFiHomeIMSI(worker, reader, profile, plmn); imsi != "" {
		profile.HomeIMSI = imsi
	} else {
		p.scheduleHomeIMSICapture(worker, reader, *profile, plmn)
	}
}

// Observe only while the same card is in low power. A home-prefix IMSI
// must be followed by a live ICCID confirmation before persistence.
func (p *Pool) observeVoWiFiHomeIMSI(worker *Worker, reader liveSIMIdentityReader, profile *identity.Profile, plmn string) string {
	return p.observeVoWiFiHomeIMSIWithin(p.Context(), worker, reader, profile, plmn)
}

func (p *Pool) observeVoWiFiHomeIMSIWithin(parent context.Context, worker *Worker, reader liveSIMIdentityReader, profile *identity.Profile, plmn string) string {
	ctx, cancel := context.WithTimeout(parent, 5*time.Second)
	defer cancel()
	for attempt := 0; attempt < 4; attempt++ {
		if ctx.Err() != nil || p.IsESIMSwitching(worker.ID) {
			return ""
		}
		mode, err := worker.Backend.GetOperatingMode(ctx)
		if err != nil || !isFlightOperatingMode(mode) {
			return ""
		}
		imsi, err := reader.GetIMSILive(ctx)
		if err == nil {
			imsi, err = backend.NormalizeIMSI(imsi)
			if err == nil && strings.HasPrefix(imsi, plmn) {
				iccid, err := reader.GetICCIDLive(ctx)
				if err == nil && db.CanonicalICCID(iccid) == profile.ICCID && !p.IsESIMSwitching(worker.ID) {
					if err := db.RecordPreferredSIMHomeIdentity(profile.ICCID, imsi, plmn[:3], plmn[3:]); err != nil {
						logger.Debug("保存运营商所需的 SIM 归属身份失败", "err", err)
					}
					return imsi
				}
				if err == nil && db.CanonicalICCID(iccid) != "" {
					return ""
				}
			}
		}
		if attempt < 3 && sleepQMIRegistrationPoll(ctx, 500*time.Millisecond) != nil {
			return ""
		}
	}
	return ""
}

func buildVoWiFiRawProfile(imsi, mcc, mnc, imei, smsc string) identity.Profile {
	return identity.Profile{
		IMSI: strings.TrimSpace(imsi),
		MCC:  strings.TrimSpace(mcc),
		MNC:  strings.TrimSpace(mnc),
		IMEI: strings.TrimSpace(imei),
		SMSC: strings.TrimSpace(smsc),
	}
}

func resolveVoWiFiProfileMCCMNC(ctx context.Context, worker *Worker, status modem.DeviceStatus, imsi, traceID string) (mcc, mnc, source, homeIMSI string) {
	imsi = strings.TrimSpace(imsi)
	home, err := db.GetSIMHomeIdentity(status.ICCID)
	if err != nil {
		logger.Debug("读取 SIM 归属身份缓存失败", "trace_id", traceID, "err", err)
	}
	if home.MCC != "" && home.MNC != "" {
		return home.MCC, home.MNC, "sim_home_iccid", home.IMSI
	}
	if worker != nil && worker.Backend != nil {
		if liveMCC, liveMNC, err := worker.Backend.GetNativeMCCMNC(ctx); err == nil {
			liveMCC = strings.TrimSpace(liveMCC)
			liveMNC = strings.TrimSpace(liveMNC)
			if liveMCC != "" && liveMNC != "" {
				cacheVoWiFiProfileMCCMNC(worker, liveMCC, liveMNC)
				return liveMCC, liveMNC, "sim_home", ""
			}
		} else {
			logger.Debug("VoWiFi 启动前读取 SIM 归属 MCC/MNC 失败，将回退已缓存 SIM 归属信息",
				"trace_id", traceID, "device", worker.ID, "err", err)
		}
	}

	statusIMSI := strings.TrimSpace(status.IMSI)
	if statusIMSI == imsi && strings.TrimSpace(status.NativeMCC) != "" && strings.TrimSpace(status.NativeMNC) != "" {
		return strings.TrimSpace(status.NativeMCC), strings.TrimSpace(status.NativeMNC), "sim_home_cache", ""
	}

	if statusIMSI != "" && statusIMSI != imsi {
		return "", "", "", ""
	}
	mcc, mnc, source = vowifiProfileMCCMNC(status)
	return mcc, mnc, source, ""
}

func cacheVoWiFiProfileMCCMNC(worker *Worker, mcc, mnc string) {
	if worker == nil || strings.TrimSpace(mcc) == "" || strings.TrimSpace(mnc) == "" {
		return
	}
	worker.cacheMu.Lock()
	worker.state.Identity.NativeMCC = strings.TrimSpace(mcc)
	worker.state.Identity.NativeMNC = strings.TrimSpace(mnc)
	worker.cacheMu.Unlock()
}

func vowifiProfileMCCMNC(status modem.DeviceStatus) (mcc, mnc, source string) {
	mcc = strings.TrimSpace(status.NativeMCC)
	mnc = strings.TrimSpace(status.NativeMNC)
	if mcc != "" && mnc != "" {
		return mcc, mnc, "sim_home_cache"
	}
	return "", "", ""
}

func newVoWiFiSIMReadyStartupState(deviceID, dataplaneMode, networkMode string, now time.Time) runtimehost.State {
	return runtimehost.State{
		Phase:         runtimehost.PhaseSIMReady,
		DeviceID:      deviceID,
		DataplaneMode: dataplaneMode,
		NetworkMode:   strings.TrimSpace(networkMode),
		SIMReady:      true,
		LastReason:    "sim_ready",
		UpdatedAt:     now,
	}
}

func (p *Pool) BuildVoWiFiStartProfile(worker *Worker) (identity.Profile, error) {
	return p.buildVoWiFiStartProfile(worker, "")
}
