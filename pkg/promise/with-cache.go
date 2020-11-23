package promise

import (
	"time"

	"github.com/rs/zerolog/log"
	"github.com/sour-is/keyproofs/pkg/cache"
)

func WithCache(c cache.Cacher, expireAfter time.Duration) OptionFn {
	return func(task *qTask) {
		innerFn := task.fn
		task.fn = func(q Q) {
			log := log.Ctx(q.Context())

			cacheKey, ok := q.Key().(cache.Key)
			if !ok {
				log.Trace().Interface(typ(q), q.Key()).Msg("not a cache key")
				innerFn(q)

				return
			}

			if v, ok := c.Get(cacheKey); ok {
				log.Trace().Interface(typ(cacheKey), cacheKey.Key()).Msg("task result in cache")
				q.Resolve(v.Value())

				return
			}

			log.Trace().Interface(typ(cacheKey), cacheKey.Key()).Msg("task not in cache")
			innerFn(q)

			if err := task.Err(); err != nil {
				log.Err(err)

				return
			}

			// expireAfter = time.Duration(rand.Int63() % int64(5*time.Second))
			result := cache.NewItem(cacheKey, task.Result(), expireAfter)

			log.Trace().Interface(typ(cacheKey), cacheKey.Key()).Msgf("task result to cache")
			c.Add(cacheKey, result)
		}
	}
}
