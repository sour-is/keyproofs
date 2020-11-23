package cache

import (
	"time"
)

type Key interface {
	Key() interface{}
}
type Value interface {
	Stale() bool
	Value() interface{}
}
type item struct {
	key      interface{}
	value    interface{}
	expireOn time.Time
}

func NewItem(key, value interface{}, expires time.Duration) *item {
	return &item{
		key:      key,
		value:    value,
		expireOn: time.Now().Add(expires),
	}
}
func (e *item) Stale() bool {
	if e == nil || e.value == nil {
		return true
	}

	return time.Now().After(e.expireOn)
}
func (s *item) Value() interface{} {
	return s.value
}

type Cacher interface {
	Add(Key, Value)
	Contains(Key) bool
	Get(Key) (Value, bool)
	Remove(Key)
}

// InterfaceCacher external cache interface.
type InterfaceCacher interface {
	Add(interface{}, interface{})
	Get(interface{}) (interface{}, bool)
	Contains(interface{}) bool
	Remove(interface{})
}

type cache struct {
	cache InterfaceCacher
}

func New(c InterfaceCacher) Cacher {
	return &cache{cache: c}
}
func (c *cache) Add(key Key, value Value) {
	c.cache.Add(key.Key(), value)
}
func (c *cache) Get(key Key) (Value, bool) {
	if v, ok := c.cache.Get(key.Key()); ok {
		if value, ok := v.(Value); ok && !value.Stale() {
			return value, true
		}
		c.cache.Remove(key.Key())
	}
	return nil, false
}
func (c *cache) Contains(key Key) bool {
	_, ok := c.Get(key)
	return ok
}
func (c *cache) Remove(key Key) {
	c.cache.Remove(key.Key())
}
