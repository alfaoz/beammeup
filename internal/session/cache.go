package session

import "sync"

type PasswordCache struct {
	mu sync.RWMutex
	m  map[string]string
}

func NewPasswordCache() *PasswordCache {
	return &PasswordCache{m: map[string]string{}}
}

func (c *PasswordCache) Get(shipName string) (string, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	v, ok := c.m[shipName]
	return v, ok
}

func (c *PasswordCache) Set(shipName, password string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.m[shipName] = password
}

func (c *PasswordCache) Forget(shipName string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.m, shipName)
}

func (c *PasswordCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.m = map[string]string{}
}
