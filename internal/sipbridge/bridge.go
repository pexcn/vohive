package sipbridge

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
	"mime"
	"net"
	"net/netip"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/emiago/sipgo"
	"github.com/emiago/sipgo/sip"
	"github.com/iniwex5/vohive/internal/config"
	"github.com/iniwex5/vohive/internal/db"
	"github.com/iniwex5/vohive/pkg/logger"
)

type SMSCallbacks struct {
	CurrentICCID func(deviceID string) string
	Send         func(ctx context.Context, job db.SIPBridgeSMS, bearer string) (id, actualBearer, state string, err error)
}

type contact struct {
	uri     sip.Uri
	source  string
	expires time.Time
	callID  string
	cseq    uint32
}

type Bridge struct {
	ctx         context.Context
	cancel      context.CancelFunc
	done        chan struct{}
	ua          *sipgo.UserAgent
	client      *sipgo.Client
	listener    net.PacketConn
	advertise   string
	callbacks   SMSCallbacks
	accounts    map[string]config.SIPBridgeAccount
	allAccounts []string
	nonceKey    [32]byte

	mu         sync.Mutex
	contacts   map[string]contact
	digestUses map[string]digestUse
}

var (
	sipUser     = regexp.MustCompile(`^[A-Za-z0-9_.~-]{1,64}$`)
	phoneNumber = regexp.MustCompile(`^\+?[0-9]{1,20}$`)
	sipHost     = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9.-]{0,252}$`)
)

// Start listens for authenticated REGISTER and MESSAGE requests and starts
// the two durable delivery queues. Account configuration is immutable.
func Start(cfg config.SIPBridgeConfig, callbacks SMSCallbacks) (*Bridge, error) {
	if !cfg.Enabled {
		return nil, nil
	}
	if callbacks.CurrentICCID == nil || callbacks.Send == nil || len(cfg.Accounts) == 0 || db.DB == nil {
		return nil, errors.New("SIP 短信桥接需要数据库、短信接口和至少一个账号")
	}
	accounts := make(map[string]config.SIPBridgeAccount)
	devices := make(map[string]string)
	for _, account := range cfg.Accounts {
		account.Username, account.DeviceID = strings.TrimSpace(account.Username), strings.TrimSpace(account.DeviceID)
		account.Bearer = strings.ToLower(strings.TrimSpace(account.Bearer))
		if !sipUser.MatchString(account.Username) || account.DeviceID == "" || strings.TrimSpace(account.Password) == "" ||
			accounts[account.Username].Username != "" || devices[account.DeviceID] != "" {
			return nil, errors.New("SIP 账号需要有效用户名、密码和唯一的绑定设备")
		}
		switch account.Bearer {
		case "", "auto", "ims", db.SMSBearerCellular:
		default:
			return nil, fmt.Errorf("SIP 账号 %s 的短信承载无效", account.Username)
		}
		devices[account.DeviceID], accounts[account.Username] = account.Username, account
	}

	address := strings.TrimSpace(cfg.ListenAddr)
	if address == "" {
		address = "127.0.0.1:5060"
	}
	listener, err := net.ListenPacket("udp", address)
	if err != nil {
		return nil, err
	}
	success := false
	defer func() {
		if !success {
			_ = listener.Close()
		}
	}()
	local := listener.LocalAddr().(*net.UDPAddr)
	host := strings.TrimSpace(cfg.AdvertiseHost)
	if host == "" && !local.IP.IsUnspecified() {
		host = local.IP.String()
	}
	if ip, err := netip.ParseAddr(host); err == nil {
		if ip.IsUnspecified() || ip.IsMulticast() || ip.Zone() != "" {
			return nil, errors.New("SIP 发布地址无效")
		}
	} else if !sipHost.MatchString(host) {
		return nil, errors.New("SIP 需要有效的 advertise_host")
	}
	ua, err := sipgo.NewUA(sipgo.WithUserAgent("VoHive SMS"), sipgo.WithUserAgentHostname(host))
	if err != nil {
		return nil, err
	}
	defer func() {
		if !success {
			_ = ua.Close()
		}
	}()
	srv, err := sipgo.NewServer(ua)
	if err != nil {
		return nil, err
	}
	client, err := sipgo.NewClient(ua, sipgo.WithClientHostname(host), sipgo.WithClientPort(local.Port), sipgo.WithClientConnectionAddr(local.String()))
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer func() {
		if !success {
			cancel()
		}
	}()
	b := &Bridge{
		ctx: ctx, cancel: cancel, done: make(chan struct{}), ua: ua, client: client,
		listener: listener, advertise: host, callbacks: callbacks, accounts: accounts,
		contacts: make(map[string]contact), digestUses: make(map[string]digestUse),
	}
	if _, err := rand.Read(b.nonceKey[:]); err != nil {
		return nil, err
	}
	for username := range accounts {
		b.allAccounts = append(b.allAccounts, username)
	}
	sort.Strings(b.allAccounts)
	if err := db.RecoverSIPBridgeQueue(); err != nil {
		return nil, err
	}
	db.ConfigureSIPBridgeAccounts(devices)
	srv.OnRegister(b.onRegister)
	srv.OnMessage(b.onMessage)
	srv.OnOptions(func(req *sip.Request, tx sip.ServerTransaction) { respond(req, tx, 200, "OK") })
	go func() {
		var wg sync.WaitGroup
		wg.Go(func() { b.runQueue(db.SIPToClient) })
		wg.Go(func() { b.runQueue(db.SIPToModem) })
		if err := srv.ServeUDP(listener); err != nil && ctx.Err() == nil {
			logger.Warn("SIP 短信监听退出", "err", err)
		}
		cancel()
		wg.Wait()
		close(b.done)
	}()
	success = true
	return b, nil
}

func (b *Bridge) Close(ctx context.Context) error {
	b.cancel()
	db.ConfigureSIPBridgeAccounts(nil)
	_ = b.listener.Close()
	err := b.ua.Close()
	select {
	case <-b.done:
		return err
	case <-ctx.Done():
		return errors.Join(err, ctx.Err())
	}
}

func respond(req *sip.Request, tx sip.ServerTransaction, status int, reason string) {
	_ = tx.Respond(sip.NewResponseFromRequest(req, status, reason, nil))
}

func (b *Bridge) onRegister(req *sip.Request, tx sip.ServerTransaction) {
	account, ok := b.authenticate(req, tx)
	if !ok {
		return
	}
	to := req.To()
	if to == nil || to.Address.User != account.Username {
		respond(req, tx, 403, "Forbidden")
		return
	}
	h := req.Contact()
	if h == nil {
		res := sip.NewResponseFromRequest(req, 200, "OK", nil)
		if current, ok := b.currentContact(account.Username); ok {
			ch := &sip.ContactHeader{Address: current.uri}
			ch.Params.Add("expires", strconv.Itoa(max(0, int(time.Until(current.expires)/time.Second))))
			res.AppendHeader(ch)
		}
		_ = tx.Respond(res)
		return
	}
	expires := "600"
	if hdr := req.GetHeader("Expires"); hdr != nil {
		expires = hdr.Value()
	}
	if value, exists := h.Params.Get("expires"); exists {
		expires = value
	}
	seconds, err := strconv.Atoi(strings.TrimSpace(expires))
	source, sourceErr := netip.ParseAddrPort(req.Source())
	if err != nil || sourceErr != nil || seconds < 0 ||
		(seconds > 0 && (h.Address.Wildcard || h.Address.Scheme != "sip" || h.Address.Host == "")) {
		respond(req, tx, 400, "Bad Contact")
		return
	}
	seconds = min(seconds, 3600)
	callID, cseq := req.CallID().Value(), req.CSeq().SeqNo
	b.mu.Lock()
	current, exists := b.contacts[account.Username]
	if exists && current.callID == callID && current.cseq > cseq && time.Now().Before(current.expires) {
		b.mu.Unlock()
		respond(req, tx, 500, "Out-of-order Registration")
		return
	}
	if seconds == 0 {
		delete(b.contacts, account.Username)
	} else {
		b.contacts[account.Username] = contact{uri: *h.Address.Clone(), source: source.String(),
			expires: time.Now().Add(time.Duration(seconds) * time.Second), callID: callID, cseq: cseq}
	}
	b.mu.Unlock()
	res := sip.NewResponseFromRequest(req, 200, "OK", nil)
	if seconds > 0 {
		ch := h.Clone()
		ch.Params.Add("expires", strconv.Itoa(seconds))
		res.AppendHeader(ch)
	}
	_ = tx.Respond(res)
}

func (b *Bridge) onMessage(req *sip.Request, tx sip.ServerTransaction) {
	account, ok := b.authenticate(req, tx)
	if !ok {
		return
	}
	h := req.GetHeader("Content-Type")
	if h == nil {
		respond(req, tx, 415, "Unsupported Media Type")
		return
	}
	media, params, err := mime.ParseMediaType(h.Value())
	if err != nil || media != "text/plain" ||
		(params["charset"] != "" && !strings.EqualFold(params["charset"], "utf-8") && !strings.EqualFold(params["charset"], "us-ascii")) {
		respond(req, tx, 415, "Unsupported Media Type")
		return
	}
	body := req.Body()
	if len(body) > 4096 || !utf8.Valid(body) || strings.TrimSpace(string(body)) == "" {
		respond(req, tx, 400, "Invalid SMS Text")
		return
	}
	phone := req.Recipient.User
	if !phoneNumber.MatchString(phone) {
		respond(req, tx, 400, "Invalid SMS Recipient")
		return
	}
	iccid := b.callbacks.CurrentICCID(account.DeviceID)
	if iccid == "" {
		respond(req, tx, 503, "SIM Unavailable")
		return
	}
	key := fmt.Sprintf("%s\x00%s\x00%d", account.Username, req.CallID().Value(), req.CSeq().SeqNo)
	id := fmt.Sprintf("sip-%x", sha256.Sum256([]byte(key)))
	ctx, cancel := context.WithTimeout(b.ctx, 3*time.Second)
	defer cancel()
	err = db.EnqueueSIPBridgeSubmission(ctx, db.SIPBridgeSMS{
		ID: id, Account: account.Username, DeviceID: account.DeviceID, ICCID: iccid, Peer: phone, Content: string(body),
	})
	if errors.Is(err, db.ErrSIPBridgeQueueFull) {
		response := sip.NewResponseFromRequest(req, 503, "SMS Queue Full", nil)
		response.AppendHeader(sip.NewHeader("Retry-After", "30"))
		_ = tx.Respond(response)
		return
	}
	if err != nil {
		respond(req, tx, 503, "SMS Queue Unavailable")
		return
	}
	res := sip.NewResponseFromRequest(req, 202, "Accepted", nil)
	res.AppendHeader(sip.NewHeader("X-VoHive-SMS-ID", id))
	_ = tx.Respond(res)
}

func (b *Bridge) currentContact(account string) (contact, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	c, ok := b.contacts[account]
	return c, ok && time.Now().Before(c.expires)
}

type AccountStatus struct {
	Username   string     `json:"username"`
	DeviceID   string     `json:"device_id"`
	Registered bool       `json:"registered"`
	ExpiresAt  *time.Time `json:"expires_at,omitempty"`
}

func (b *Bridge) Status() []AccountStatus {
	out := make([]AccountStatus, 0, len(b.allAccounts))
	for _, username := range b.allAccounts {
		account := b.accounts[username]
		c, registered := b.currentContact(username)
		status := AccountStatus{Username: username, DeviceID: account.DeviceID, Registered: registered}
		if registered {
			status.ExpiresAt = &c.expires
		}
		out = append(out, status)
	}
	return out
}
