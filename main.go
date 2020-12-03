package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/go-chi/chi"
	"github.com/go-chi/chi/middleware"
	lru "github.com/hashicorp/golang-lru"
	_ "github.com/joho/godotenv/autoload"
	"github.com/rs/cors"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"gosrc.io/xmpp"

	"github.com/sour-is/keyproofs/pkg/cache"
	"github.com/sour-is/keyproofs/pkg/config"
	"github.com/sour-is/keyproofs/pkg/graceful"
	"github.com/sour-is/keyproofs/pkg/keyproofs"
)

var (
	// AppVersion Application Version Number
	AppVersion string

	// AppBuild Application Build Hash
	BuildHash string

	// AppDate Application Build Date
	BuildDate string
)

func main() {
	log := zerolog.New(zerolog.NewConsoleWriter()).
		With().
		Timestamp().
		Caller().
		Logger()

	ctx := context.Background()
	ctx = log.WithContext(ctx)
	ctx = graceful.WithInterupt(ctx)
	ctx, _ = graceful.WithWaitGroup(ctx)

	cfg := config.New()
	cfg.Set("app-name", "KeyProofs")
	cfg.Set("app-version", AppVersion)
	cfg.Set("build-hash", BuildHash)
	cfg.Set("build-date", BuildDate)
	ctx = cfg.Apply(ctx)

	if err := run(ctx); err != nil {
		log.Error().Stack().Err(err).Msg("Application Failed")
		os.Exit(1)
	}
}

func run(ctx context.Context) error {
	log := log.Ctx(ctx)
	wg := graceful.WaitGroup(ctx)

	// derive baseURL from listener options
	listen := env("HTTP_LISTEN", ":9061")
	host, _ := os.Hostname()
	if strings.HasPrefix(listen, ":") {
		host += listen
	}
	baseURL := fmt.Sprintf("http://%s", host)

	// Set config values
	cfg := config.FromContext(ctx)
	cfg.Set("base-url", env("BASE_URL", baseURL))
	cfg.Set("dns-url", env("DNS_URL", baseURL))
	cfg.Set("xmpp-url", env("XMPP_URL", baseURL))

	cfg.Set("reddit.api-key", os.Getenv("REDDIT_APIKEY"))
	cfg.Set("reddit.secret", os.Getenv("REDDIT_SECRET"))

	cfg.Set("xmpp-config", &xmpp.Config{
		Jid:        os.Getenv("XMPP_USERNAME"),
		Credential: xmpp.Password(os.Getenv("XMPP_PASSWORD")),
	})

	mux := chi.NewRouter()
	mux.Use(
		cfg.ApplyHTTP,
		func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				r = r.WithContext(log.WithContext(r.Context()))
				next.ServeHTTP(w, r)
			})
		},
		secHeaders,
		cors.New(cors.Options{
			AllowCredentials: true,
			AllowedMethods:   strings.Fields(env("CORS_METHODS", "GET")),
			AllowedOrigins:   strings.Fields(env("CORS_ORIGIN", "*")),
		}).Handler,
		middleware.RequestID,
		middleware.RealIP,
		middleware.RequestLogger(&middleware.DefaultLogFormatter{Logger: accessLog(log.Info)}),
		middleware.Recoverer,
	)

	if env("DISABLE_KEYPROOF", "false") == "false" {
		// Create cache for promise engine
		arc, _ := lru.NewARC(4096)
		c := cache.New(arc)
		keyproofs.NewKeyProofApp(ctx, c).Routes(mux)
	}

	if env("DISABLE_DNS", "false") == "false" {
		keyproofs.NewDNSApp(ctx).Routes(mux)
	}

	if env("DISABLE_AVATAR", "false") == "false" {
		avatarApp, err := keyproofs.NewAvatarApp(ctx, env("AVATAR_PATH", "pub"))
		if err != nil {
			return err
		}

		avatarApp.Routes(mux)
	}

	if env("DISABLE_WKD", "false") == "false" {
		avatarApp, err := keyproofs.NewWKDApp(ctx, env("WKD_PATH", "pub"), env("WKD_DOMAIN", "pub"))
		if err != nil {
			return err
		}

		avatarApp.Routes(mux)
	}

	if env("DISABLE_VCARD", "false") == "false" {
		vcardApp, err := keyproofs.NewVCardApp(ctx)
		if err != nil {
			return err
		}
		vcardApp.Routes(mux)
	}

	log.Info().
		Str("app", cfg.GetString("app-name")).
		Str("version", cfg.GetString("app-version")).
		Str("build-hash", cfg.GetString("build-hash")).
		Str("build-date", cfg.GetString("build-date")).
		Str("listen", listen).
		Int("user", os.Geteuid()).
		Int("group", os.Getgid()).
		Msg("startup")

	err := New(&http.Server{
		Addr:         listen,
		WriteTimeout: 15 * time.Second,
		ReadTimeout:  15 * time.Second,
		Handler:      mux,
	}).Run(ctx)
	if err != nil {
		return err
	}

	return wg.Wait(5 * time.Second)
}

type Server struct {
	srv *http.Server
}

func New(s *http.Server) *Server {
	return &Server{srv: s}
}
func (s *Server) Run(ctx context.Context) error {
	log := log.Ctx(ctx)
	wg := graceful.WaitGroup(ctx)

	wg.Go(func() error {
		<-ctx.Done()
		log.Info().Msg("Shutdown HTTP")

		ctx := context.Background()
		ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		err := s.srv.Shutdown(ctx)
		if err != nil && err != http.ErrServerClosed {
			return err
		}

		log.Info().Msg("Stopped  HTTP")
		return nil
	})

	err := s.srv.ListenAndServe()
	if err != nil && err != http.ErrServerClosed {
		return err
	}

	return nil
}

func env(name, defaultValue string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}

	return defaultValue
}

func secHeaders(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-XSS-Protection", "1; mode=block")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Content-Security-Policy", "font-src https://pagecdn.io")

		h.ServeHTTP(w, r)
	})
}

type accessLog func() *zerolog.Event

func (a accessLog) Print(v ...interface{}) {
	a().Msg(fmt.Sprint(v...))
}
