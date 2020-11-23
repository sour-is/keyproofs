package keyproofs

import (
	"bytes"
	"context"
	"crypto/sha1"
	"fmt"
	"io"
	"net/http"
	"net/mail"
	"net/url"
	"strings"

	"github.com/rs/zerolog/log"
	"github.com/sour-is/crypto/openpgp"
	"github.com/tv42/zbase32"
	"golang.org/x/crypto/openpgp/armor"
)

func getOpenPGPkey(ctx context.Context, id string) (entity *Entity, err error) {
	if isFingerprint(id) {
		addr := "https://keys.openpgp.org/vks/v1/by-fingerprint/" + strings.ToUpper(id)
		return getEntityHTTP(ctx, addr, true)
	} else if email, err := mail.ParseAddress(id); err == nil {
		addr := getWKDPubKeyAddr(email)
		req, err := getEntityHTTP(ctx, addr, false)
		if err == nil {
			return req, err
		}

		addr = "https://keys.openpgp.org/vks/v1/by-email/" + url.QueryEscape(id)
		return getEntityHTTP(ctx, addr, true)
	} else {
		return entity, fmt.Errorf("Parse address: %w", err)
	}
}

func getEntityHTTP(ctx context.Context, url string, useArmored bool) (entity *Entity, err error) {
	log := log.Ctx(ctx)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return entity, err
	}
	cl := http.Client{}
	resp, err := cl.Do(req)
	log.Debug().
		Bool("useArmored", useArmored).
		Str("status", resp.Status).
		Str("url", url).
		Msg("getEntityHTTP")

	if err != nil {
		return entity, fmt.Errorf("Requesting key: %w\nRemote URL: %v", err, url)
	}

	if resp.StatusCode != 200 {
		return entity, fmt.Errorf("bad response from remote: %s\nRemote URL: %v", resp.Status, url)
	}

	defer resp.Body.Close()

	if resp.Header.Get("Content-Type") == "application/pgp-keys" {
		useArmored = true
	}

	return ReadKey(resp.Body, useArmored)
}

type EntityKey string

func (k EntityKey) Key() interface{} {
	return k
}

type Entity struct {
	Primary     *mail.Address
	Emails      []*mail.Address
	Fingerprint string
	Proofs      []string
	ArmorText   string
}

func getEntity(lis openpgp.EntityList) (*Entity, error) {
	entity := &Entity{}
	var err error

	for _, e := range lis {
		if e == nil {
			continue
		}
		if e.PrimaryKey == nil {
			continue
		}

		entity.Fingerprint = fmt.Sprintf("%X", e.PrimaryKey.Fingerprint)

		for name, ident := range e.Identities {
			// Pick first identity
			if entity.Primary == nil {
				entity.Primary, err = mail.ParseAddress(name)
				if err != nil {
					return entity, err
				}
			}
			// If one is marked primary use that
			if ident.SelfSignature != nil && ident.SelfSignature.IsPrimaryId != nil && *ident.SelfSignature.IsPrimaryId {
				entity.Primary, err = mail.ParseAddress(name)
				if err != nil {
					return entity, err
				}

			} else {
				var email *mail.Address
				if email, err = mail.ParseAddress(name); err != nil {
					return entity, err
				}
				if email.Address != entity.Primary.Address {
					entity.Emails = append(entity.Emails, email)
				}
			}

			// If identity is self signed read notation data.
			if ident.SelfSignature != nil && ident.SelfSignature.NotationData != nil {
				// Get proofs and append to list.
				if proofs, ok := ident.SelfSignature.NotationData["proof@metacode.biz"]; ok {
					entity.Proofs = append(entity.Proofs, proofs...)
				}
			}
		}
		break
	}

	if entity.Primary == nil {
		entity.Primary, _ = mail.ParseAddress("nobody@nodomain.xyz")
	}

	return entity, err
}

func ReadKey(r io.Reader, useArmored bool) (e *Entity, err error) {
	var buf bytes.Buffer

	var w io.Writer = &buf

	e = &Entity{}

	if !useArmored {
		var aw io.WriteCloser
		aw, err = armor.Encode(&buf, "PGP PUBLIC KEY BLOCK", nil)
		if err != nil {
			return e, fmt.Errorf("Read key: %w", err)
		}
		defer aw.Close()

		w = aw
	}
	defer func() {
		if e != nil {
			e.ArmorText = buf.String()
		}
	}()

	r = io.TeeReader(r, w)

	var lis openpgp.EntityList

	if useArmored {
		lis, err = openpgp.ReadArmoredKeyRing(r)
	} else {
		lis, err = openpgp.ReadKeyRing(r)
	}
	if err != nil {
		return e, fmt.Errorf("Read key: %w", err)
	}

	e, err = getEntity(lis)
	if err != nil {
		return e, fmt.Errorf("Parse key: %w", err)
	}

	return
}

func isFingerprint(s string) bool {
	for _, r := range s {
		switch r {
		case '0', '1', '2', '3', '4', '5', '6', '7', '8', '9', 'a', 'b', 'c', 'd', 'e', 'f', 'A', 'B', 'C', 'D', 'E', 'F':
		default:
			return false
		}
	}

	return true
}

func getWKDPubKeyAddr(email *mail.Address) string {
	parts := strings.SplitN(email.Address, "@", 2)

	hash := sha1.Sum([]byte(parts[0]))
	lp := zbase32.EncodeToString(hash[:])

	return fmt.Sprintf("https://%s/.well-known/openpgpkey/hu/%s", parts[1], lp)
}
