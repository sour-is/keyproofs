package graceful

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
	"go.uber.org/multierr"
)

func WithInterupt(ctx context.Context) context.Context {
	log := log.Ctx(ctx)
	ctx, cancel := context.WithCancel(ctx)

	// Listen for Interrupt signals
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)

	go func() {
		defer signal.Stop(c)

		for {
			select {
			case <-c:
				cancel()
				log.Warn().Msg("Shutting down! interrupt received")
				return
			case <-ctx.Done():
				log.Warn().Msg("Shutting down! context cancelled")
				return
			}
		}
	}()

	return ctx
}

type contextKey struct{ string }

var wgKey = contextKey{"waitgroup"}

type wgContext struct {
	wg  sync.WaitGroup
	err error
	ctx context.Context
}

func (wg *wgContext) String() string {
	return fmt.Sprintf("WaitGroup[%v %v]", wg.err, wg.ctx)
}

type WG interface {
	Wait(time.Duration) error
	Go(func() error)
}

func WithWaitGroup(ctx context.Context) (context.Context, WG) {
	if wg := WaitGroup(ctx); wg != nil {
		return ctx, wg
	}
	wg := &wgContext{ctx: ctx}
	return context.WithValue(ctx, wgKey, wg), wg
}

func WaitGroup(ctx context.Context) *wgContext {
	if wg, ok := ctx.Value(wgKey).(*wgContext); ok {
		return wg
	}
	return nil
}

func (wg *wgContext) Go(fn func() error) {
	if wg == nil {
		panic("nil wait group")
	}

	wg.Add(1)
	go func() {
		err := fn()
		wg.err = multierr.Append(wg.err, err)
		wg.Done()
	}()
}

func (wg *wgContext) Add(n int) {
	wg.wg.Add(n)
}

func (wg *wgContext) Done() {
	wg.wg.Done()
}

func (wg *wgContext) Wait(gracetime time.Duration) error {
	if wg == nil {
		return nil
	}

	log := log.Ctx(wg.ctx)

	ch := make(chan struct{})
	go func() {
		wg.wg.Wait()
		close(ch)
	}()

	<-wg.ctx.Done()
	wg.err = multierr.Append(wg.err, wg.ctx.Err())

	log.Debug().Msg("shutdown begin")
	timer := time.NewTimer(gracetime)

	select {
	case <-ch:
	case <-timer.C:
		wg.err = multierr.Append(wg.err, ErrExpiredGrace)
	}
	log.Debug().Msg("shutdown complete")

	return wg.err
}

var ErrExpiredGrace = errors.New("grace time expired")
