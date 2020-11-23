package config

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"sync"
)

type cfg struct {
	sync.RWMutex
	m map[string]interface{}
}

var key struct{}

func New() *cfg {
	return &cfg{m: make(map[string]interface{})}
}

func FromContext(ctx context.Context) *cfg {
	if v, ok := ctx.Value(key).(*cfg); ok {
		return v
	}

	return nil
}

func (c *cfg) Apply(ctx context.Context) context.Context {
	if inctx := FromContext(ctx); inctx != nil {
		inctx.setAll(c.m)
	}

	return context.WithValue(ctx, key, c)
}

func (c *cfg) setAll(m map[string]interface{}) {
	if c == nil {
		return
	}

	c.Lock()
	defer c.Unlock()

	c.m = m
}

func (c *cfg) GetString(name string) string {
	if v := c.Get(name); v != nil {
		if s, ok := v.(string); ok {
			return s
		} else {
			return fmt.Sprint(s)
		}
	}

	return ""
}

func (c *cfg) Set(name string, value interface{}) {
	if c == nil {
		return
	}

	c.Lock()
	defer c.Unlock()

	c.m[name] = value
}

func (c *cfg) Get(name string) interface{} {
	if c == nil {
		return nil
	}

	c.RLock()
	defer c.RUnlock()

	return c.m[name]
}

func (c *cfg) String() string {
	if c == nil {
		return "<nil>"
	}

	c.RLock()
	defer c.RUnlock()

	var b bytes.Buffer
	for k, v := range c.m {
		fmt.Fprintf(&b, "%s = %v\n", k, v)
	}
	return b.String()
}

func (c *cfg) ApplyHTTP(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(c.Apply(r.Context()))
		h.ServeHTTP(w, r)
	})
}
