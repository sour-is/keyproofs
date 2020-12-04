package opgp

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
	"github.com/sour-is/keyproofs/pkg/opgp/entity"
	"github.com/tv42/zbase32"
	"golang.org/x/crypto/openpgp/armor"
)

func GetKey(ctx context.Context, id string) (entity *entity.Entity, err error) {
	if isFingerprint(id) {
		addr := "https://keys.openpgp.org/vks/v1/by-fingerprint/" + strings.ToUpper(id)
		return getEntityHTTP(ctx, addr, true)
	} else if email, err := mail.ParseAddress(id); err == nil {
		addr, advAddr := getWKDPubKeyAddr(email)
		req, err := getEntityHTTP(ctx, addr, false)
		if err == nil {
			return req, err
		}

		req, err = getEntityHTTP(ctx, advAddr, false)
		if err == nil {
			return req, err
		}

		addr = "https://keys.openpgp.org/vks/v1/by-email/" + url.QueryEscape(id)
		return getEntityHTTP(ctx, addr, true)
	} else {
		return entity, fmt.Errorf("Parse address: %w", err)
	}
}

func getEntityHTTP(ctx context.Context, url string, useArmored bool) (entity *entity.Entity, err error) {
	log := log.Ctx(ctx)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return entity, err
	}
	cl := http.Client{}
	resp, err := cl.Do(req)
	if err != nil {
		return entity, fmt.Errorf("Requesting key: %w\nRemote URL: %v", err, url)
	}
	log.Debug().
		Bool("useArmored", useArmored).
		Str("status", resp.Status).
		Str("url", url).
		Msg("getEntityHTTP")

	if resp.StatusCode != 200 {
		return entity, fmt.Errorf("bad response from remote: %s\nRemote URL: %v", resp.Status, url)
	}

	defer resp.Body.Close()

	if resp.Header.Get("Content-Type") == "application/pgp-keys" {
		useArmored = true
	}

	return ReadKey(resp.Body, useArmored)
}

func ReadKey(r io.Reader, useArmored bool) (e *entity.Entity, err error) {
	var buf bytes.Buffer

	var w io.Writer = &buf
	e = &entity.Entity{}

	defer func() {
		if e != nil {
			e.ArmorText = buf.String()
		}
	}()

	if !useArmored {
		var aw io.WriteCloser
		aw, err = armor.Encode(&buf, "PGP PUBLIC KEY BLOCK", nil)
		if err != nil {
			return e, fmt.Errorf("Read key: %w", err)
		}
		defer aw.Close()

		w = aw
	}

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

	e, err = entity.GetOne(lis)
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

func getWKDPubKeyAddr(email *mail.Address) (string, string) {
	parts := strings.SplitN(email.Address, "@", 2)
	hash := sha1.Sum([]byte(parts[0]))
	lp := zbase32.EncodeToString(hash[:])

	return fmt.Sprintf("https://%s/.well-known/openpgpkey/hu/%s", parts[1], lp),
		fmt.Sprintf("https://openpgpkey.%s/.well-known/openpgpkey/hu/%s/%s", parts[1], parts[1], lp)
}
