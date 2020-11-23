package promise

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/rs/zerolog/log"

	"go.uber.org/ratelimit"
)

type Q interface {
	Key() interface{}
	Context() context.Context
	Resolve(interface{})
	Reject(error)

	Tasker
}
type ResultQ interface {
	Key() interface{}
	Context() context.Context
	Result() interface{}

	Tasker
}
type Fn func(Q)
type AfterFn func(ResultQ)
type Key interface {
	Key() interface{}
}

func typ(v interface{}) string {
	return fmt.Sprintf("%T", v)
}

type qTask struct {
	key Key

	fn  Fn
	ctx context.Context

	cancel func()
	done   chan struct{}

	result interface{}
	err    error

	Tasker
}

func (t *qTask) Key() interface{}         { return t.key }
func (t *qTask) Context() context.Context { return t.ctx }
func (t *qTask) Resolve(r interface{})    { t.result = r; t.finish() }
func (t *qTask) Reject(err error)         { t.err = err; t.finish() }

// After runs on successful completion of the task.
func (t *qTask) After(fn AfterFn) {
	log := log.Ctx(t.Context())

	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Panic().Msgf("%v", r)
			}
		}()

		<-t.Await()
		if err := t.Err(); err != nil {
			return
		}
		fn(t)
	}()
}
func (t *qTask) Await() <-chan struct{} { return t.done }
func (t *qTask) Cancel()                { t.err = fmt.Errorf("task cancelled"); t.finish() }

func (t *qTask) Result() interface{} { return t.result }
func (t *qTask) Err() error          { return t.err }

func (t *qTask) finish() {
	if t.done == nil {
		return
	}

	t.cancel()
	close(t.done)
	t.done = nil
}

type Option interface {
	Apply(*qTask)
}
type OptionFn func(*qTask)

func (fn OptionFn) Apply(t *qTask) { fn(t) }

type Tasker interface {
	Run(Key, Fn, ...Option) *qTask
}

type Runner struct {
	defaultOpts []Option
	queue       map[interface{}]*qTask
	mu          sync.RWMutex
	ctx         context.Context
	cancel      func()
	pause       chan struct{}
	limiter     ratelimit.Limiter
}

type Timeout time.Duration

func (d Timeout) Apply(task *qTask) {
	task.ctx, task.cancel = context.WithTimeout(task.ctx, time.Duration(d))
}

func (tr *Runner) Run(key Key, fn Fn, opts ...Option) *qTask {
	log := log.Ctx(tr.ctx)

	tr.mu.RLock()
	log.Trace().Interface(typ(key), key.Key()).Msg("task to run")

	if task, ok := tr.queue[key.Key()]; ok {
		tr.mu.RUnlock()
		log.Trace().Interface(typ(key), key.Key()).Msg("task found running")

		return task
	}
	tr.mu.RUnlock()

	task := &qTask{
		key:    key,
		fn:     fn,
		cancel: func() {},
		ctx:    tr.ctx,
		done:   make(chan struct{}),
		Tasker: tr,
	}

	for _, opt := range tr.defaultOpts {
		opt.Apply(task)
	}

	for _, opt := range opts {
		opt.Apply(task)
	}

	tr.mu.Lock()
	tr.queue[key.Key()] = task
	tr.mu.Unlock()

	tr.limiter.Take()

	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Panic().Msgf("%v", r)
			}

			if err := task.Err(); err == nil {
				log.Trace().Interface(typ(key), key.Key()).Msg("task complete")
			} else {
				log.Debug().Interface(typ(key), key.Key()).Err(err).Msg("task Failed")
			}

			tr.mu.Lock()
			delete(tr.queue, task.Key())
			tr.mu.Unlock()
		}()

		log.Trace().Interface(typ(key), key.Key()).Msg("task Running")

		task.fn(task)
	}()

	return task
}

func NewRunner(ctx context.Context, defaultOpts ...Option) *Runner {
	ctx, cancel := context.WithCancel(ctx)

	tr := &Runner{
		defaultOpts: defaultOpts,
		queue:       make(map[interface{}]*qTask),
		ctx:         ctx,
		cancel:      cancel,
		pause:       make(chan struct{}),
		limiter:     ratelimit.New(10),
	}

	return tr
}

func (tr *Runner) List() []*qTask {
	tr.mu.RLock()
	defer tr.mu.RUnlock()

	lis := make([]*qTask, 0, len(tr.queue))

	for _, task := range tr.queue {
		lis = append(lis, task)
	}

	return lis
}

func (tr *Runner) Stop() {
	tr.cancel()
}

func (tr *Runner) Len() int {
	tr.mu.RLock()
	defer tr.mu.RUnlock()

	return len(tr.queue)
}
