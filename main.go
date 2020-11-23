package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
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
	log := zerolog.New(zerolog.NewConsoleWriter()).With().Timestamp().Caller().Logger()

	ctx := context.Background()
	ctx = log.WithContext(ctx)
	ctx = WithInterupt(ctx)

	cfg := config.New()
	cfg.Set("app-name", "KeyProofs")
	cfg.Set("app-version", AppVersion)
	cfg.Set("build-hash", BuildHash)
	cfg.Set("build-date", BuildDate)
	ctx = cfg.Apply(ctx)

	if err := run(ctx); err != nil {
		log.Fatal().Stack().Err(err).Send()
		os.Exit(1)
	}
}

func run(ctx context.Context) error {
	log := log.Ctx(ctx)

	// derive baseURL from listener options
	listen := env("HTTP_LISTEN", ":9061")
	host, _ := os.Hostname()
	if strings.HasPrefix(listen, ":") {
		host += listen
	}
	baseURL := fmt.Sprintf("http://%s", host)

	// Create cache for promise engine
	arc, _ := lru.NewARC(4096)
	c := cache.New(arc)

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

	// configure cors middleware
	corsMiddleware := cors.New(cors.Options{
		AllowCredentials: true,
		AllowedMethods:   strings.Fields(env("CORS_METHODS", "GET")),
		AllowedOrigins:   strings.Fields(env("CORS_ORIGIN", "*")),
	}).Handler

	mux := chi.NewRouter()
	mux.Use(
		cfg.ApplyHTTP,
		corsMiddleware,
		middleware.RequestID,
		middleware.RealIP,
		middleware.RequestLogger(&middleware.DefaultLogFormatter{Logger: accessLog(log.Info)}),
		middleware.Recoverer,
	)

	app, err := keyproofs.New(ctx, c)
	if err != nil {
		return err
	}

	app.Routes(mux)

	log.Info().
		Str("app", cfg.GetString("app-name")).
		Str("version", cfg.GetString("app-version")).
		Str("build-hash", cfg.GetString("build-hash")).
		Str("build-date", cfg.GetString("build-date")).
		Str("listen", listen).
		Msg("startup")

	err = New(&http.Server{
		Addr:         listen,
		WriteTimeout: 15 * time.Second,
		ReadTimeout:  15 * time.Second,
		Handler:      mux,
	}).Run(ctx)

	if err != nil {
		return err
	}

	log.Info().Msg("shutdown")
	return nil
}

type Server struct {
	srv *http.Server
}

func New(s *http.Server) *Server {
	return &Server{srv: s}
}
func (s *Server) Run(ctx context.Context) error {
	log := log.Ctx(ctx)

	go func() {
		<-ctx.Done()
		log.Info().Msg("Shutdown HTTP")

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		err := s.srv.Shutdown(ctx)
		if err != nil {
			log.Fatal().Err(err)
			return
		}

		log.Info().Msg("Stopped  HTTP")
	}()

	return s.srv.ListenAndServe()
}

func env(name, defaultValue string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}

	return defaultValue
}

func WithInterupt(ctx context.Context) context.Context {
	log := log.Ctx(ctx)
	ctx, cancel := context.WithCancel(ctx)

	// Listen for Interrupt signals
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	defer signal.Stop(c)

	go func() {
		select {
		case <-c:
			cancel()
			log.Warn().Msg("Shutting down! interrupt received")
			return
		case <-ctx.Done():
			cancel()

			log.Warn().Msg("Shutting down! context cancelled")
			return
		}
	}()

	return ctx
}

type accessLog func() *zerolog.Event

func (a accessLog) Print(v ...interface{}) {
	a().Msg(fmt.Sprint(v...))
}
