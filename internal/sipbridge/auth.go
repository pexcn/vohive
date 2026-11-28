package sipbridge

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"strings"
	"time"

	"github.com/emiago/sipgo/sip"
	"github.com/icholy/digest"
	"github.com/iniwex5/vohive/internal/config"
)

const (
	realm           = "vohive-sms"
	digestAlgorithm = "SHA-256"
	nonceLifetime   = 2 * time.Minute
)

func (b *Bridge) signNonce(data []byte, peer string) []byte {
	mac := hmac.New(sha256.New, b.nonceKey[:])
	mac.Write(data)
	mac.Write([]byte(peer))
	return mac.Sum(nil)
}

func (b *Bridge) challenge(req *sip.Request, tx sip.ServerTransaction) {
	data := make([]byte, 20)
	binary.BigEndian.PutUint64(data, uint64(time.Now().Unix()))
	if _, err := rand.Read(data[8:]); err != nil {
		respond(req, tx, 503, "Authentication Unavailable")
		return
	}
	nonce := base64.RawURLEncoding.EncodeToString(append(data, b.signNonce(data, req.Source())...))
	challenge := digest.Challenge{Realm: realm, Nonce: nonce, Algorithm: digestAlgorithm, QOP: []string{"auth"}}
	res := sip.NewResponseFromRequest(req, 401, "Unauthorized", nil)
	res.AppendHeader(sip.NewHeader("WWW-Authenticate", challenge.String()))
	_ = tx.Respond(res)
}

type digestUse struct {
	fingerprint [32]byte
	expires     time.Time
}

func (b *Bridge) authenticate(req *sip.Request, tx sip.ServerTransaction) (config.SIPBridgeAccount, bool) {
	account, ok := b.authorizedAccount(req)
	if !ok {
		b.challenge(req, tx)
	}
	return account, ok
}

func (b *Bridge) authorizedAccount(req *sip.Request) (config.SIPBridgeAccount, bool) {
	empty := config.SIPBridgeAccount{}
	h := req.GetHeader("Authorization")
	if h == nil || req.From() == nil || req.CallID() == nil || req.CSeq() == nil {
		return empty, false
	}
	cr, err := digest.ParseCredentials(h.Value())
	if err != nil || cr.Realm != realm || !strings.EqualFold(cr.Algorithm, digestAlgorithm) ||
		cr.QOP != "auth" || cr.Nc <= 0 || cr.Cnonce == "" || cr.URI != req.Recipient.String() {
		return empty, false
	}
	account, ok := b.accounts[cr.Username]
	if !ok || req.From().Address.User != account.Username {
		return empty, false
	}
	data, err := base64.RawURLEncoding.DecodeString(cr.Nonce)
	if err != nil || len(data) != 20+sha256.Size || !hmac.Equal(data[20:], b.signNonce(data[:20], req.Source())) {
		return empty, false
	}
	now := time.Now()
	issued := time.Unix(int64(binary.BigEndian.Uint64(data)), 0)
	expires := issued.Add(nonceLifetime)
	if now.Before(issued) || !now.Before(expires) {
		return empty, false
	}
	auth, err := digest.Digest(&digest.Challenge{Realm: realm, Nonce: cr.Nonce, Algorithm: digestAlgorithm, QOP: []string{"auth"}},
		digest.Options{Method: string(req.Method), URI: cr.URI, Username: account.Username, Password: account.Password, Count: cr.Nc, Cnonce: cr.Cnonce})
	if err != nil || !hmac.Equal([]byte(auth.Response), []byte(strings.ToLower(cr.Response))) {
		return empty, false
	}
	key := fmt.Sprintf("%s/%s/%d", cr.Nonce, account.Username, cr.Nc)
	// Include the entire request so a nonce count can only repeat the same
	// authenticated datagram, including the body when using qop=auth.
	fingerprint := sha256.Sum256([]byte(req.String()))
	b.mu.Lock()
	defer b.mu.Unlock()
	if prior, exists := b.digestUses[key]; exists {
		return account, prior.fingerprint == fingerprint
	}
	if len(b.digestUses) >= 4096 {
		for key, used := range b.digestUses {
			if !now.Before(used.expires) {
				delete(b.digestUses, key)
			}
		}
		if len(b.digestUses) >= 4096 {
			return empty, false
		}
	}
	b.digestUses[key] = digestUse{fingerprint: fingerprint, expires: expires}
	return account, true
}
