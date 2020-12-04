package app_avatar

import (
	"context"
	"crypto/md5"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"hash"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/disintegration/imaging"
	"github.com/fsnotify/fsnotify"
	"github.com/go-chi/chi"
	"github.com/nullrocks/identicon"
	"github.com/rs/zerolog/log"

	"github.com/sour-is/keyproofs/pkg/graceful"
	"github.com/sour-is/keyproofs/pkg/style"
)

var pixl = "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNkYAAAAAYAAjCB0C8AAAAASUVORK5CYII="

type avatar struct {
	path string
}

func New(ctx context.Context, path string) (*avatar, error) {
	log := log.Ctx(ctx)

	path = filepath.Clean(path)
	app := &avatar{path: path}
	err := app.CheckFiles(ctx)
	if err != nil {
		return nil, fmt.Errorf("check files: %w", err)
	}

	watch, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	for _, typ := range []string{"avatar", "bg", "cover"} {
		err = watch.Add(filepath.Join(path, typ))
		if err != nil {
			return nil, fmt.Errorf("adding watch: %w", err)
		}
	}

	log.Debug().Msg("startup avatar watcher")
	wg := graceful.WaitGroup(ctx)
	wg.Go(func() error {
		for {
			select {
			case <-ctx.Done():
				log.Debug().Msg("shutdown avatar watcher")
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

func (app *avatar) CheckFiles(ctx context.Context) error {
	log := log.Ctx(ctx)

	for _, name := range []string{".links", "avatar", "bg", "cover"} {
		log.Debug().Msgf("mkdir: %s", filepath.Join(app.path, name))
		err := os.MkdirAll(filepath.Join(app.path, name), 0700)
		if err != nil {
			return err
		}
	}

	return filepath.Walk(app.path, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return fmt.Errorf("walk failed: %w", err)
		}
		if info.IsDir() {
			switch info.Name() {
			case "avatar", "bg", "cover":
				return nil
			default:
				return filepath.SkipDir
			}
		}

		path = filepath.Dir(path)
		kind := filepath.Base(path)
		name := info.Name()

		log.Debug().Msgf("link: %s %s %s", app.path, kind, name)

		return app.createLinks(kind, name)
	})
}

func (app *avatar) get(w http.ResponseWriter, r *http.Request) {
	log := log.Ctx(r.Context())

	log.Print(r.Host)

	kind := chi.URLParam(r, "kind")
	hash := chi.URLParam(r, "hash")

	sizeW, sizeH, resize := 0, 0, false
	if s, err := strconv.Atoi(r.URL.Query().Get("s")); err == nil && s > 0 {
		sizeW, sizeH, resize = sizeByKind(kind, s)
	}
	log.Debug().Int("width", sizeW).Int("height", sizeH).Bool("resize", resize).Str("kind", kind).Msg("Get Image")

	if strings.ContainsRune(hash, '@') {
		avatarHost, _, err := style.GetSRV(r.Context(), hash)
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

	if !fileExists(fname) {
		switch kind {
		case "avatar":
			ig, err := identicon.New("sour.is", 5, 3)
			if err != nil {
				writeText(w, 500, err.Error())
				return
			}

			ii, err := ig.Draw(hash)
			if err != nil {
				writeText(w, 500, err.Error())
				return
			}

			w.Header().Set("Content-Type", "image/png")
			w.WriteHeader(200)
			err = ii.Png(clamp(128, 512, sizeW), w)
			log.Error().Err(err).Send()

			return
		default:
			sp := strings.SplitN(pixl, ",", 2)
			b, _ := base64.RawStdEncoding.DecodeString(sp[1])
			w.Header().Set("Content-Type", "image/png")
			w.WriteHeader(200)
			if _, err := w.Write(b); err != nil {
				log.Error().Err(err).Send()
			}
			return
		}
	}

	if !resize {
		f, err := os.Open(fname)
		if err != nil {
			writeText(w, 500, err.Error())
			return
		}

		w.Header().Set("Content-Type", "image/png")
		w.WriteHeader(200)

		_, err = io.Copy(w, f)
		if err != nil {
			log.Error().Err(err).Send()
		}
		return
	}

	img, err := imaging.Open(fname, imaging.AutoOrientation(true))
	if err != nil {
		writeText(w, 500, err.Error())
		return
	}

	img = imaging.Fill(img, sizeW, sizeH, imaging.Center, imaging.Lanczos)

	w.Header().Set("Content-Type", "image/png")
	w.WriteHeader(200)
	log.Debug().Msg("writing image")
	err = imaging.Encode(w, img, imaging.PNG)
	if err != nil {
		log.Error().Err(err).Send()
	}
}

func (app *avatar) Routes(r *chi.Mux) {
	r.MethodFunc("GET", "/{kind:avatar|bg|cover}/{hash}", app.get)
}

func hashString(value string, h hash.Hash) string {
	_, _ = h.Write([]byte(value))
	return fmt.Sprintf("%x", h.Sum(nil))
}
func hashMD5(name string) string {
	return hashString(name, md5.New())
}
func hashSHA256(name string) string {
	return hashString(name, sha256.New())
}

func (app *avatar) createLinks(kind, name string) error {
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

	hash = hashSHA256(name)
	link = filepath.Join(app.path, ".links", strings.Join([]string{kind, hash}, "-"))
	err = app.replaceLink(src, link)

	return err
}

func (app *avatar) removeLinks(kind, name string) error {
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

func (app *avatar) replaceLink(src, link string) error {
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

func fileExists(filename string) bool {
	info, err := os.Stat(filename)
	if os.IsNotExist(err) {
		return false
	}
	return !info.IsDir()
}

func sizeByKind(kind string, size int) (sizeW int, sizeH int, resize bool) {
	switch kind {
	case "avatar":
		if size == 0 {
			size = 128
		}
		sizeW = clamp(128, 640, size)
		sizeH = sizeW
		resize = true

		return
	case "cover":
		if size == 0 {
			size = 940
		}

		sizeW = clamp(640, 1300, size)
		sizeH = ratio(sizeW, 2.7)
		resize = true

		return
	default:
		return 0, 0, false
	}
}

func ratio(size int, ratio float64) int {
	return int(float64(size) / ratio)
}
func clamp(min, max, size int) int {
	if size > max {
		return max
	}

	if size < min {
		return min
	}

	return size
}

// WriteText writes plain text
func writeText(w http.ResponseWriter, code int, o string) {
	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(code)
	_, _ = w.Write([]byte(o))
}
