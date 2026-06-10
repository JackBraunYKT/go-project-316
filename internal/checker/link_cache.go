package checker

import (
	"sync"
)

type LinkCache struct {
	mu    sync.Mutex
	cache map[string]BrokenLink
}

// NewLinkCache создает кеш результатов проверки ссылок.
func NewLinkCache() *LinkCache {
	return &LinkCache{
		cache: make(map[string]BrokenLink),
	}
}

// Get возвращает результат проверки ссылки из кеша.
func (c *LinkCache) Get(key string) (BrokenLink, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	result, ok := c.cache[key]
	return result, ok
}

// Set сохраняет результат проверки ссылки в кеше.
func (c *LinkCache) Set(key string, value BrokenLink) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cache[key] = value
}

// Contains сообщает, есть ли результат проверки ссылки в кеше.
func (c *LinkCache) Contains(key string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	_, ok := c.cache[key]
	return ok
}
