package app_keyproofs

import (
	"context"
	"encoding/base64"
	"fmt"
	"html/template"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi"
	zlog "github.com/rs/zerolog/log"
	"github.com/russross/blackfriday"
	"github.com/skip2/go-qrcode"

	"github.com/sour-is/keyproofs/pkg/cache"
	"github.com/sour-is/keyproofs/pkg/config"
	"github.com/sour-is/keyproofs/pkg/opgp"
	"github.com/sour-is/keyproofs/pkg/opgp/entity"
	"github.com/sour-is/keyproofs/pkg/promise"
	"github.com/sour-is/keyproofs/pkg/style"
)

var expireAfter = 20 * time.Minute
var runnerTimeout = 30 * time.Second

// 1x1 gif pixel
var pixl = "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNkYAAAAAYAAjCB0C8AAAAASUVORK5CYII="
var keypng, _ = base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAABAAAAAQCAYAAAAf8/9hAAABKUlEQVQ4jZ2SvUoDURCFUy/Y2Fv4BoKIiFgLSWbmCWw0e3cmNgGfwacQsbCxUEFEEIVkxsQulaK1kheIiFVW1mJXiZv904FbXb5zzvzUaiWlPqyYwIkyvRjjqwmeaauxUcbFMKOvTKEJRVPv05hCY9wrhHt+fckEJ79gxg9rweJN8qdSkESZjlLOkQm+Xe9szlubFkxwYoznuQIm9DgrQJEyjZXpPU5Eo6L+H7IEUmJFAnBQJmAMp5nw0IFnjFoiEGrQXJuBLx14JtgtiR5qAO2c4aFLAffGeGiMT8b0rAEe96WxnBlbGbbia/vZ+2CwjXO5g0pN/TZ1NNXgoQPPHO2aJLsViu4E+xdVnXsOOtPOMbxeDY6jw/6/nL+r6+qryjQyhqs/OSf1Bf+pJC1wKqO/AAAAAElFTkSuQmCC")

var defaultStyle = &style.Style{
	Avatar:     pixl,
	Cover:      pixl,
	Background: pixl,
	Palette:    style.GetPalette("#93CCEA"),
}

type keyproofApp struct {
	cache  cache.Cacher
	tasker promise.Tasker
}

func NewKeyProofApp(ctx context.Context, c cache.Cacher) *keyproofApp {
	return &keyproofApp{
		cache: c,
		tasker: promise.NewRunner(
			ctx,
			promise.Timeout(runnerTimeout),
			promise.WithCache(c, expireAfter),
		),
	}
}
func (app *keyproofApp) Routes(r *chi.Mux) {
	r.MethodFunc("GET", "/", app.getHome)
	r.MethodFunc("GET", "/id/{id}", app.getProofs)
	r.MethodFunc("GET", "/qr", app.getQR)
	r.MethodFunc("GET", "/favicon.ico", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		w.WriteHeader(200)
		_, _ = w.Write(keypng)
	})
}
func (app *keyproofApp) getProofs(w http.ResponseWriter, r *http.Request) {
	log := zlog.Ctx(r.Context())
	cfg := config.FromContext(r.Context())

	id := chi.URLParam(r, "id")
	log.Debug().Str("get ", id).Send()

	// Setup timeout for page refresh
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	// Run tasks to resolve entity, style, and proofs.
	task := app.tasker.Run(entity.Key(id), func(q promise.Q) {
		ctx := q.Context()
		log := zlog.Ctx(ctx).With().Interface(fmtKey(q), q.Key()).Logger()

		key := q.Key().(entity.Key)

		e, err := opgp.GetKey(ctx, string(key))
		if err != nil {
			q.Reject(err)
			return
		}

		log.Debug().Msg("Resolving Entity")
		q.Resolve(e)
	})

	task.After(func(q promise.ResultQ) {
		entity := q.Result().(*entity.Entity)

		zlog.Ctx(q.Context()).
			Info().
			Str("email", entity.Primary.Address).
			Interface(fmtKey(q), q.Key()).
			Msg("Do Style ")

		q.Run(style.Key(entity.Primary.Address), func(q promise.Q) {
			ctx := q.Context()
			log := zlog.Ctx(ctx).With().Interface(fmtKey(q), q.Key()).Logger()

			key := q.Key().(style.Key)

			log.Debug().Msg("start task")
			style, err := style.GetStyle(ctx, string(key))
			if err != nil {
				q.Reject(err)
				return
			}

			log.Debug().Msg("Resolving Style")
			q.Resolve(style)
		})
	})

	task.After(func(q promise.ResultQ) {
		entity := q.Result().(*entity.Entity)
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
	page.AppBuild = fmt.Sprintf("%s %s", cfg.GetString("build-date"), cfg.GetString("build-hash"))

	// Wait for either entity to resolve or timeout
	select {
	case <-task.Await():
		log.Print("Tasks Competed")
		if err := task.Err(); err != nil {
			page.Err = err
			page.IsComplete = true
			break
		}
		page.Entity = task.Result().(*entity.Entity)

	case <-ctx.Done():
		log.Print("Deadline Timeout")
		if e, ok := app.cache.Get(entity.Key(id)); ok {
			page.Entity = e.Value().(*entity.Entity)
		}
	}

	// Build page based on available information.
	if page.Entity != nil {
		var gotStyle, gotProofs bool

		if s, ok := app.cache.Get(style.Key(page.Entity.Primary.Address)); ok {
			page.Style = s.Value().(*style.Style)
			gotStyle = true
		}

		gotProofs = true
		if len(page.Entity.Proofs) > 0 {
			page.HasProofs = true
			proofs := make(Proofs, len(page.Entity.Proofs))
			for i := range page.Entity.Proofs {
				p := page.Entity.Proofs[i]

				if s, ok := app.cache.Get(ProofKey(p)); ok {
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
	var err error
	t := template.New("page")
	t, err = t.Parse(pageTPL)
	if err != nil {
		writeText(w, 500, err.Error())
		return
	}

	t, err = t.Parse(proofTPL)
	if err != nil {
		writeText(w, 500, err.Error())
		return
	}

	err = t.Execute(w, page)
	if err != nil {
		writeText(w, 500, err.Error())
		return
	}
}
func (app *keyproofApp) getHome(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	cfg := config.FromContext(ctx)

	baseURL := cfg.GetString("base-url")
	if id := r.URL.Query().Get("id"); id != "" {
		http.Redirect(w, r, fmt.Sprintf("%s/id/%s", baseURL, id), http.StatusFound)
		return
	}

	page := page{Style: defaultStyle, IsComplete: true, Markdown: homeMKDN}
	page.AppName = fmt.Sprintf("%s v%s", cfg.GetString("app-name"), cfg.GetString("app-version"))

	// Template and display.
	var err error
	t := template.New("page")
	t = t.Funcs(template.FuncMap{"markDown": markDowner})
	t, err = t.Parse(pageTPL)
	if err != nil {
		writeText(w, 500, err.Error())
		return
	}

	t, err = t.Parse(homeTPL)
	if err != nil {
		writeText(w, 500, err.Error())
		return
	}

	err = t.Execute(w, page)
	if err != nil {
		writeText(w, 500, err.Error())
		return
	}
}
func (app *keyproofApp) getQR(w http.ResponseWriter, r *http.Request) {
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
		writeText(w, 400, err.Error())
		return
	}

	w.Header().Add("Content-Type", "image/png")
	w.WriteHeader(200)

	_, _ = w.Write(png)
}

func markDowner(args ...interface{}) template.HTML {
	s := blackfriday.MarkdownCommon([]byte(fmt.Sprintf("%s", args...)))
	return template.HTML(s)
}

// WriteText writes plain text
func writeText(w http.ResponseWriter, code int, o string) {
	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(code)
	_, _ = w.Write([]byte(o))
}

func fmtKey(key promise.Key) string {
	return fmt.Sprintf("%T", key.Key())
}
