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

	"github.com/sour-is/keyproofs/pkg/cache"
	"github.com/sour-is/keyproofs/pkg/config"
	"github.com/sour-is/keyproofs/pkg/graceful"
	"github.com/sour-is/keyproofs/pkg/httpsrv"

	app_avatar "github.com/sour-is/keyproofs/pkg/app/avatar"
	app_dns "github.com/sour-is/keyproofs/pkg/app/dns"
	app_keyproofs "github.com/sour-is/keyproofs/pkg/app/keyproofs"
	app_wkd "github.com/sour-is/keyproofs/pkg/app/wkd"
)

var (
	// AppName Application Name
	AppName string = "KeyProofs"

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
	cfg.Set("app-name", AppName)
	cfg.Set("app-version", AppVersion)
	cfg.Set("build-hash", BuildHash)
	cfg.Set("build-date", BuildDate)
	ctx = cfg.Apply(ctx)

	log.Info().
		Str("app", AppName).
		Str("version", AppVersion).
		Str("build-hash", BuildHash).
		Str("build-date", BuildDate).
		Msg("startup...")

	if err := run(ctx); err != nil {
		log.Error().Err(err).Msg("Application Failed")
		os.Exit(1)
	}
}

func run(ctx context.Context) error {
	log := log.Ctx(ctx)
	wg := graceful.WaitGroup(ctx)
	cfg := config.FromContext(ctx)

	// derive baseURL from listener options
	listen := env("HTTP_LISTEN", ":9061")
	host, _ := os.Hostname()
	if strings.HasPrefix(listen, ":") {
		host += listen
	}
	baseURL := fmt.Sprintf("http://%s", host)

	// Setup router
	cors := cors.New(cors.Options{
		AllowCredentials: true,
		AllowedMethods:   strings.Fields(env("CORS_METHODS", "GET")),
		AllowedOrigins:   strings.Fields(env("CORS_ORIGIN", "*")),
	})

	logFmt := &middleware.DefaultLogFormatter{Logger: accessLog(log.Info)}

	mux := chi.NewRouter()
	mux.Use(
		middleware.RequestID,
		middleware.RealIP,
		middleware.Recoverer,
		middleware.RequestLogger(logFmt),
		secHeaders,
		cors.Handler,
		addLogger(log),
		cfg.ApplyHTTP,
	)

	if env("DISABLE_KEYPROOF", "false") == "false" {
		// Set config values
		cfg.Set("base-url", env("BASE_URL", baseURL))
		cfg.Set("dns-url", env("DNS_URL", baseURL))
		cfg.Set("xmpp-url", env("XMPP_URL", baseURL))

		cfg.Set("reddit.api-key", os.Getenv("REDDIT_APIKEY"))
		cfg.Set("reddit.secret", os.Getenv("REDDIT_SECRET"))
		cfg.Set("github.secret", os.Getenv("GITHUB_SECRET"))

		// Create cache for promise engine
		arc, _ := lru.NewARC(4096)
		c := cache.New(arc)
		app_keyproofs.NewKeyProofApp(ctx, c).Routes(mux)
	}

	if env("DISABLE_DNS", "false") == "false" {
		app_dns.New(ctx).Routes(mux)
	}

	if env("DISABLE_AVATAR", "false") == "false" {
		app, err := app_avatar.New(ctx, env("AVATAR_PATH", "pub"))
		if err != nil {
			return err
		}

		app.Routes(mux)
	}

	if env("DISABLE_WKD", "false") == "false" {
		app, err := app_wkd.New(ctx, env("WKD_PATH", "pub"), env("WKD_DOMAIN", "sour.is"))
		if err != nil {
			return err
		}

		app.Routes(mux)
	}

	if env("DISABLE_VCARD", "false") == "false" {
		app, err := app_vcard.New(ctx, &xmpp.Config{
			Jid:        os.Getenv("XMPP_USERNAME"),
			Credential: xmpp.Password(os.Getenv("XMPP_PASSWORD")),
		})
		if err != nil {
			return err
		}
		app.Routes(mux)
	}

	log.Info().
		Str("listen", listen).
		Int("user", os.Geteuid()).
		Int("group", os.Getgid()).
		Msg("running")

	err := httpsrv.New(&http.Server{
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

func addLogger(log *zerolog.Logger) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			r = r.WithContext(log.WithContext(r.Context()))
			next.ServeHTTP(w, r)
		})
	}
}
