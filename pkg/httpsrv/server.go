package httpsrv

import (
	"context"
	"net/http"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/sour-is/keyproofs/pkg/graceful"
)

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
