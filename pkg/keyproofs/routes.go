package keyproofs

import (
	"context"
	"encoding/base64"
	"fmt"
	"net"
	"net/http"
	"net/mail"
	"strconv"
	"strings"
	"text/template"
	"time"

	"github.com/go-chi/chi"
	zlog "github.com/rs/zerolog/log"
	"github.com/skip2/go-qrcode"
	"gosrc.io/xmpp"

	"github.com/sour-is/keyproofs/pkg/cache"
	"github.com/sour-is/keyproofs/pkg/config"
	"github.com/sour-is/keyproofs/pkg/promise"
)

var expireAfter = 20 * time.Minute

func New(ctx context.Context, c cache.Cacher) (*identity, error) {
	log := zlog.Ctx(ctx)

	var ok bool
	var xmppConfig *xmpp.Config
	if xmppConfig, ok = config.FromContext(ctx).Get("xmpp-config").(*xmpp.Config); !ok {
		log.Error().Msg("no xmpp-config")

		return nil, fmt.Errorf("no xmpp config")
	}

	conn, err := NewXMPP(ctx, xmppConfig)
	if err != nil {
		return nil, err
	}

	tasker := promise.NewRunner(ctx, promise.Timeout(30*time.Second), promise.WithCache(c, expireAfter))
	i := &identity{
		cache:  c,
		tasker: tasker,
		conn:   conn,
	}

	return i, nil
}

// 1x1 gif pixel
var pixl = "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNkYAAAAAYAAjCB0C8AAAAASUVORK5CYII="
var keypng, _ = base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAABAAAAAQCAYAAAAf8/9hAAABKUlEQVQ4jZ2SvUoDURCFUy/Y2Fv4BoKIiFgLSWbmCWw0e3cmNgGfwacQsbCxUEFEEIVkxsQulaK1kheIiFVW1mJXiZv904FbXb5zzvzUaiWlPqyYwIkyvRjjqwmeaauxUcbFMKOvTKEJRVPv05hCY9wrhHt+fckEJ79gxg9rweJN8qdSkESZjlLOkQm+Xe9szlubFkxwYoznuQIm9DgrQJEyjZXpPU5Eo6L+H7IEUmJFAnBQJmAMp5nw0IFnjFoiEGrQXJuBLx14JtgtiR5qAO2c4aFLAffGeGiMT8b0rAEe96WxnBlbGbbia/vZ+2CwjXO5g0pN/TZ1NNXgoQPPHO2aJLsViu4E+xdVnXsOOtPOMbxeDY6jw/6/nL+r6+qryjQyhqs/OSf1Bf+pJC1wKqO/AAAAAElFTkSuQmCC")

var defaultStyle = &Style{
	Avatar:     pixl,
	Cover:      pixl,
	Background: pixl,
	Palette:    getPalette("#93CCEA"),
}

type identity struct {
	cache  cache.Cacher
	tasker promise.Tasker
	conn   *connection
}

func (s *identity) Routes(r *chi.Mux) {
	r.Use(secHeaders)
	r.MethodFunc("GET", "/id/{id}", s.get)
	r.MethodFunc("GET", "/dns/{domain}", s.getDNS)
	r.MethodFunc("GET", "/vcard/{jid}", s.getVCard)
	r.MethodFunc("GET", "/qr", s.getQR)
	r.MethodFunc("GET", "/favicon.ico", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		w.WriteHeader(200)
		_, _ = w.Write(keypng)
	})
}

func fmtKey(key promise.Key) string {
	return fmt.Sprintf("%T", key.Key())
}

func (s *identity) get(w http.ResponseWriter, r *http.Request) {
	log := zlog.Ctx(r.Context())
	cfg := config.FromContext(r.Context())

	id := chi.URLParam(r, "id")
	log.Debug().Str("get ", id).Send()

	// Setup timeout for page refresh
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	// Run tasks to resolve entity, style, and proofs.
	task := s.tasker.Run(EntityKey(id), func(q promise.Q) {
		ctx := q.Context()
		log := zlog.Ctx(ctx).With().Interface(fmtKey(q), q.Key()).Logger()

		key := q.Key().(EntityKey)

		entity, err := getOpenPGPkey(ctx, string(key))
		if err != nil {
			q.Reject(err)
			return
		}

		log.Debug().Msg("Resolving Entity")
		q.Resolve(entity)
	})

	task.After(func(q promise.ResultQ) {
		entity := q.Result().(*Entity)

		zlog.Ctx(q.Context()).
			Info().
			Str("email", entity.Primary.Address).
			Interface(fmtKey(q), q.Key()).
			Msg("Do Style ")

		q.Run(StyleKey(entity.Primary.Address), func(q promise.Q) {
			ctx := q.Context()
			log := zlog.Ctx(ctx).With().Interface(fmtKey(q), q.Key()).Logger()

			key := q.Key().(StyleKey)

			log.Debug().Msg("start task")
			style, err := s.getStyle(ctx, string(key))
			if err != nil {
				q.Reject(err)
				return
			}

			log.Debug().Msg("Resolving Style")
			q.Resolve(style)
		})

	})

	task.After(func(q promise.ResultQ) {
		entity := q.Result().(*Entity)
		log := zlog.Ctx(ctx).
			With().
			Interface(fmtKey(q), q.Key()).
			Logger()

		log.Info().
			Int("num", len(entity.Proofs)).
			Msg("Scheduling Proofs")

		for i := range entity.Proofs {
			q.Run(ProofKey(entity.Proofs[i]), func(q promise.Q) {
				ctx := q.Context()
				log := zlog.Ctx(ctx).
					With().
					Interface(fmtKey(q), q.Key()).
					Logger()

				key := q.Key().(ProofKey)
				proof := NewProof(ctx, string(key), entity.Fingerprint)
				defer log.Debug().Interface("status", proof.Proof().Status).Msg("Resolving Proof")

				if err := proof.Resolve(ctx); err != nil && err != ErrNoFingerprint {
					log.Err(err).Send()
				}

				q.Resolve(proof.Proof())
			})
		}
	})

	page := page{Style: defaultStyle}
	page.AppName = fmt.Sprintf("%s v%s", cfg.GetString("app-name"), cfg.GetString("app-version"))


	// Wait for either entity to resolve or timeout
	select {
	case <-task.Await():
		log.Print("Tasks Competed")
		if err := task.Err(); err != nil {
			page.Err = err
			page.IsComplete = true
			break
		}
		page.Entity = task.Result().(*Entity)

	case <-ctx.Done():
		log.Print("Deadline Timeout")
		if e, ok := s.cache.Get(EntityKey(id)); ok {
			page.Entity = e.Value().(*Entity)
		}
	}

	// Build page based on available information.
	if page.Entity != nil {
		var gotStyle, gotProofs bool

		if s, ok := s.cache.Get(StyleKey(page.Entity.Primary.Address)); ok {
			page.Style = s.Value().(*Style)
			gotStyle = true
		}

		gotProofs = true
		if len(page.Entity.Proofs) > 0 {
			page.HasProofs = true
			proofs := make(Proofs, len(page.Entity.Proofs))
			for i := range page.Entity.Proofs {
				p := page.Entity.Proofs[i]

				if s, ok := s.cache.Get(ProofKey(p)); ok {
					log.Debug().Str("uri", p).Msg("Proof from cache")
					proofs[p] = s.Value().(*Proof)
				} else {
					log.Debug().Str("uri", p).Msg("Missing proof")
					proofs[p] = NewProof(ctx, p, page.Entity.Fingerprint).Proof()
					gotProofs = false
				}
			}
			page.Proofs = &proofs
		}

		page.IsComplete = gotStyle && gotProofs
	}

	// Template and display.
	t, err := template.New("identity").Parse(pageTPL)
	if err != nil {
		WriteText(w, 500, err.Error())
		return
	}
	err = t.Execute(w, page)
	if err != nil {
		WriteText(w, 500, err.Error())
		return
	}
}

func (s *identity) getDNS(w http.ResponseWriter, r *http.Request) {
	domain := chi.URLParam(r, "domain")

	res, err := net.DefaultResolver.LookupTXT(r.Context(), domain)
	if err != nil {
		WriteText(w, 400, err.Error())
		return
	}

	WriteText(w, 200, strings.Join(res, "\n"))
}

func (s *identity) getQR(w http.ResponseWriter, r *http.Request) {
	log := zlog.Ctx(r.Context())

	content := r.URL.Query().Get("c")
	size := 64

	sz, _ := strconv.Atoi(r.URL.Query().Get("s"))

	if sz > -10 && sz < 0 {
		size = sz
	} else if sz > 64 && sz < 4096 {
		size = sz
	} else if sz > 4096 {
		size = 4096
	}

	quality := qrcode.Medium
	switch r.URL.Query().Get("r") {
	case "L":
		quality = qrcode.Low
	case "Q":
		quality = qrcode.High
	case "H":
		quality = qrcode.Highest
	}

	log.Debug().Str("content", content).Int("size", size).Interface("quality", quality).Int("s", sz).Msg("QRCode")

	png, err := qrcode.Encode(content, quality, size)
	if err != nil {
		WriteText(w, 400, err.Error())
		return
	}

	w.Header().Add("Content-Type", "image/png")
	w.WriteHeader(200)

	_, _ = w.Write(png)
}

func (s *identity) getVCard(w http.ResponseWriter, r *http.Request) {
	jid := chi.URLParam(r, "jid")
	if _, err := mail.ParseAddress(jid); err != nil {
		fmt.Fprint(w, err)
		w.WriteHeader(400)
	}

	vcard, err := s.conn.GetXMPPVCard(r.Context(), jid)
	if err != nil {
		fmt.Fprint(w, err)
		w.WriteHeader(500)
	}

	w.Header().Set("Content-Type", "text/xml")
	w.WriteHeader(200)
	fmt.Fprint(w, vcard)
}

func secHeaders(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-XSS-Protection", "1; mode=block")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Content-Type-Options", "nosniff")

		h.ServeHTTP(w, r)
	})
}

// WriteText writes plain text
func WriteText(w http.ResponseWriter, code int, o string) {
	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(code)
	_, _ = w.Write([]byte(o))
}
