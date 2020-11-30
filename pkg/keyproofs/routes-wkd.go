package keyproofs

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/fsnotify/fsnotify"
	"github.com/go-chi/chi"
	"github.com/rs/zerolog/log"
	"github.com/sour-is/keyproofs/pkg/graceful"
)

type wkdApp struct {
	path   string
	domain string
}

func NewWKDApp(ctx context.Context, path, domain string) (*wkdApp, error) {
	log := log.Ctx(ctx)

	path = filepath.Clean(path)
	app := &wkdApp{path: path}
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
						fmt.Println(err)
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
				fmt.Println(err)
			}
		}
	})

	return app, nil
}

func (app *wkdApp) CheckFiles(ctx context.Context) error {
	log := log.Ctx(ctx)

	for _, name := range []string{".links", "wkd"} {
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
		if info.IsDir() {
			if info.Name() == ".links" {
				return filepath.SkipDir
			}
			return nil
		}

		path = filepath.Dir(path)
		kind := filepath.Base(path)
		name := info.Name()

		log.Debug().Msgf("link: %s %s %s", app.path, kind, name)

		return app.createLinks(kind, name)
	})
}

func (app *wkdApp) get(w http.ResponseWriter, r *http.Request) {
	log := log.Ctx(r.Context())

	log.Print(r.Host)

	kind := chi.URLParam(r, "kind")
	hash := chi.URLParam(r, "hash")

	if strings.ContainsRune(hash, '@') {
		avatarHost, _, err := styleSRV(r.Context(), hash)
		if err != nil {
			writeText(w, 500, err.Error())
			return
		}
		hash = hashSHA256(strings.ToLower(hash))
		http.Redirect(w, r, fmt.Sprintf("https://%s/%s/%s?%s", avatarHost, kind, hash, r.URL.RawQuery), 301)
		return
	}

	fname := filepath.Join(app.path, ".links", strings.Join([]string{kind, hash}, "-"))
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
	r.MethodFunc("GET", "/.well-known/openpgpkey/hu/{hash}", app.get)
	r.MethodFunc("GET", "/.well-known/openpgpkey/hu/{domain}/{hash}", app.get)
}

func (app *wkdApp) createLinks(kind, name string) error {
	if !strings.ContainsRune(name, '@') {
		return nil
	}

	src := filepath.Join("..", kind, name)
	name = strings.ToLower(name)

	hash := hashMD5(name)
	link := filepath.Join(app.path, ".links", strings.Join([]string{kind, hash}, "-"))
	err := app.replaceLink(src, link)
	if err != nil {
		return err
	}

	return err
}

func (app *wkdApp) removeLinks(kind, name string) error {
	if !strings.ContainsRune(name, '@') {
		return nil
	}
	name = strings.ToLower(name)

	hash := hashMD5(name)
	link := filepath.Join(app.path, ".links", strings.Join([]string{kind, hash}, "-"))
	err := os.Remove(link)
	if err != nil {
		return err
	}

	hash = hashSHA256(name)
	link = filepath.Join(app.path, ".links", strings.Join([]string{kind, hash}, "-"))
	err = os.Remove(link)

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
