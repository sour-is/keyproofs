package app_wkd

import (
	"context"
	"crypto/sha1"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/fsnotify/fsnotify"
	"github.com/go-chi/chi"
	"github.com/rs/zerolog/log"
	"github.com/sour-is/crypto/openpgp"
	"github.com/tv42/zbase32"

	"github.com/sour-is/keyproofs/pkg/graceful"
	"github.com/sour-is/keyproofs/pkg/opgp/entity"
)

type wkdApp struct {
	path   string
	domain string
}

func New(ctx context.Context, path, domain string) (*wkdApp, error) {
	log := log.Ctx(ctx)
	log.Debug().Str("domain", domain).Str("path", path).Msg("NewWKDApp")

	path = filepath.Clean(path)
	app := &wkdApp{path: path, domain: domain}
	err := app.CheckFiles(ctx)
	if err != nil {
		return nil, err
	}

	watch, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	for _, typ := range []string{"keys"} {
		err = watch.Add(filepath.Join(path, typ))
		if err != nil {
			return nil, err
		}
	}

	log.Debug().Msg("startup wkd watcher")
	wg := graceful.WaitGroup(ctx)
	wg.Go(func() error {
		for {
			select {
			case <-ctx.Done():
				log.Debug().Msg("shutdown wkd watcher")
				return nil
			case op := <-watch.Events:
				log.Print(op)
				switch op.Op {
				case fsnotify.Create:
					path = filepath.Dir(op.Name)
					kind := filepath.Base(path)
					name := filepath.Base(op.Name)
					if err := app.createLinks(kind, name); err != nil {
						log.Err(err).Send()
					}
				case fsnotify.Remove, fsnotify.Rename:
					path = filepath.Dir(op.Name)
					kind := filepath.Base(path)
					name := filepath.Base(op.Name)
					if err := app.removeLinks(kind, name); err != nil {
						log.Error().Err(err).Send()
					}
				default:
				}
			case err := <-watch.Errors:
				log.Err(err).Send()
			}
		}
	})

	return app, nil
}

func (app *wkdApp) CheckFiles(ctx context.Context) error {
	log := log.Ctx(ctx)

	for _, name := range []string{".links", "keys"} {
		log.Debug().Msgf("mkdir: %s", filepath.Join(app.path, name))
		err := os.MkdirAll(filepath.Join(app.path, name), 0700)
		if err != nil {
			return err
		}
	}

	return filepath.Walk(app.path, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		log.Debug().Msg(info.Name())
		if path == app.path {
			return nil
		}
		if info.IsDir() {
			switch info.Name() {
			case "keys":
				return nil
			}
			return filepath.SkipDir

		}

		path = filepath.Dir(path)
		kind := filepath.Base(path)
		name := info.Name()

		log.Debug().Msgf("link: %s %s %s", app.path, kind, name)

		return app.createLinks(kind, name)
	})
}

func (app *wkdApp) getRedirect(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := log.Ctx(ctx)

	log.Print(r.Host)

	hash := chi.URLParam(r, "hash")

	if strings.ContainsRune(hash, '@') {
		hash, domain := hashHuman(hash)
		log.Debug().Str("hash", hash).Str("domain", domain).Msg("redirect")
		if host, adv := getWKDDomain(ctx, domain); adv {
			log.Debug().Str("host", host).Str("domain", domain).Bool("adv", adv).Msg("redirect")
			http.Redirect(w, r, fmt.Sprintf("https://%s/.well-known/openpgpkey/hu/%s/%s", host, domain, hash), http.StatusTemporaryRedirect)
		} else {
			log.Debug().Str("host", host).Str("domain", domain).Bool("adv", adv).Msg("redirect")
			http.Redirect(w, r, fmt.Sprintf("https://%s/.well-known/openpgpkey/hu/%s", domain, hash), http.StatusTemporaryRedirect)
		}

		return
	}

	writeText(w, http.StatusBadRequest, "Bad Request")
}

func (app *wkdApp) get(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := log.Ctx(ctx)

	log.Print(r.Host)

	hash := chi.URLParam(r, "hash")
	domain := chi.URLParam(r, "domain")
	if domain == "" {
		domain = app.domain
	}

	if strings.ContainsRune(hash, '@') {
		hash, domain = hashHuman(hash)
	}

	fname := filepath.Join(app.path, ".links", strings.Join([]string{"keys", domain, hash}, "-"))
	log.Debug().Msgf("path: %s", fname)

	f, err := os.Open(fname)
	if err != nil {
		writeText(w, 500, err.Error())
		return
	}

	_, err = io.Copy(w, f)
	if err != nil {
		writeText(w, 500, err.Error())
		return
	}
}

func (app *wkdApp) Routes(r *chi.Mux) {
	r.MethodFunc("GET", "/wkd/{hash}", app.getRedirect)
	r.MethodFunc("GET", "/key/{hash}", app.get)
	r.MethodFunc("POST", "/pks/add", app.postKey)
	r.MethodFunc("GET", "/.well-known/openpgpkey/hu/{hash}", app.get)
	r.MethodFunc("GET", "/.well-known/openpgpkey/hu/{domain}/{hash}", app.get)
}

func (app *wkdApp) createLinks(kind, name string) error {
	if !strings.ContainsRune(name, '@') {
		return nil
	}

	src := filepath.Join("..", kind, name)
	name = strings.ToLower(name)

	hash, domain := hashHuman(name)
	link := filepath.Join(app.path, ".links", strings.Join([]string{kind, domain, hash}, "-"))
	err := app.replaceLink(src, link)
	if err != nil {
		return err
	}

	return err
}
func hashHuman(name string) (string, string) {
	name = strings.ToLower(name)
	parts := strings.SplitN(name, "@", 2)
	hash := sha1.Sum([]byte(parts[0]))
	lp := zbase32.EncodeToString(hash[:])

	return lp, parts[1]
}

func (app *wkdApp) removeLinks(kind, name string) error {
	if !strings.ContainsRune(name, '@') {
		return nil
	}
	name = strings.ToLower(name)

	hash, domain := hashHuman(name)
	link := filepath.Join(app.path, ".links", strings.Join([]string{kind, domain, hash}, "-"))
	err := os.Remove(link)
	if err != nil {
		return err
	}

	return err
}

func (app *wkdApp) replaceLink(src, link string) error {
	if dst, err := os.Readlink(link); err != nil {
		if os.IsNotExist(err) {
			err = os.Symlink(src, link)
			if err != nil {
				return err
			}
		}
	} else {
		if dst != src {
			err = os.Remove(link)
			if err != nil {
				return err
			}
			err = os.Symlink(src, link)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func getWKDDomain(ctx context.Context, domain string) (string, bool) {
	adv := "openpgpkey." + domain
	_, err := net.DefaultResolver.LookupCNAME(ctx, adv)
	if err == nil {
		return adv, true
	}
	return domain, false
}

func (app *wkdApp) postKey(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := log.Ctx(ctx)

	body, err := ioutil.ReadAll(r.Body)
	r.Body.Close()
	if err != nil {
		log.Err(err).Send()
		writeText(w, http.StatusBadRequest, "ERR BODY")

		return
	}

	q, err := url.ParseQuery(string(body))
	if err != nil {
		log.Err(err).Send()
		writeText(w, http.StatusBadRequest, "ERR PARSE")

		return
	}

	lis, err := openpgp.ReadArmoredKeyRing(strings.NewReader(q.Get("keytext")))
	if err != nil {
		log.Err(err).Send()
		writeText(w, http.StatusBadRequest, "ERR READ KEY")

		return
	}

	e, err := entity.GetOne(lis)
	if err != nil {
		log.Err(err).Send()
		writeText(w, http.StatusBadRequest, "ERR ENTITY")

		return
	}

	fname := filepath.Join(app.path, "keys", e.Primary.Address)

	f, err := os.Open(fname)
	if os.IsNotExist(err) {
		out, err := os.Create(fname)
		if err != nil {
			log.Err(err).Send()
			writeText(w, http.StatusInternalServerError, "ERR CREATE")

			return
		}

		err = e.Serialize(out)
		if err != nil {
			log.Err(err).Send()
			writeText(w, http.StatusInternalServerError, "ERR WRITE")
			return
		}

		w.Header().Set("X-HKP-Status", "Created key")
		writeText(w, http.StatusOK, "OK CREATED")
		return
	}

	current, err := openpgp.ReadKeyRing(f)
	if err != nil {
		log.Err(err).Send()
		writeText(w, http.StatusInternalServerError, "ERR READ")

		return
	}
	f.Close()

	compare, err := entity.GetOne(current)
	if err != nil {
		log.Err(err).Send()
		writeText(w, http.StatusInternalServerError, "ERR PARSE")

		return
	}

	if e.Fingerprint != compare.Fingerprint {
		w.Header().Set("X-HKP-Status", "Mismatch fingerprint")
		writeText(w, http.StatusBadRequest, "ERR FINGERPRINT")
		return
	}
	if e.SelfSignature == nil || compare.SelfSignature == nil {
		w.Header().Set("X-HKP-Status", "Missing signature")
		writeText(w, http.StatusBadRequest, "ERR SIGNATURE")
		return
	}

	log.Debug().Msgf("%v < %v", e.SelfSignature.CreationTime, compare.SelfSignature.CreationTime)

	if !compare.SelfSignature.CreationTime.Before(e.SelfSignature.CreationTime) {
		w.Header().Set("X-HKP-Status", "out of date")
		writeText(w, http.StatusBadRequest, "ERR OUT OF DATE")

		return
	}

	out, err := os.Create(fname)
	if err != nil {
		log.Err(err).Send()
		writeText(w, http.StatusInternalServerError, "ERR CREATE")

		return
	}

	err = e.Serialize(out)
	if err != nil {
		log.Err(err).Send()
		writeText(w, http.StatusInternalServerError, "ERR WRITE")

		return
	}

	w.Header().Set("X-HKP-Status", "Updated key")
	writeText(w, http.StatusOK, "OK UPDATED")
}

// WriteText writes plain text
func writeText(w http.ResponseWriter, code int, o string) {
	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(code)
	_, _ = w.Write([]byte(o))
}
