package shared

import (
	"encoding/json"
	"fmt"
	"net/url"
	"sync"
)

type URLWithDepth struct {
	URL   string
	Depth int
}

type URLQueue struct {
	items []URLWithDepth
	mu    sync.Mutex
}

// NewURLQueue создает очередь URL с корневым адресом.
func NewURLQueue(rootURL string) *URLQueue {
	return &URLQueue{
		items: []URLWithDepth{{URL: rootURL, Depth: 0}},
	}
}

// Enqueue добавляет URL в очередь обхода.
func (q *URLQueue) Enqueue(urls []URLWithDepth) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.items = append(q.items, urls...)
}

// Dequeue извлекает следующий URL из очереди обхода.
func (q *URLQueue) Dequeue() *URLWithDepth {
	q.mu.Lock()
	defer q.mu.Unlock()

	if len(q.items) == 0 {
		return nil
	}

	item := q.items[0]
	q.items = q.items[1:]
	return &item
}

// IsEmpty сообщает, пуста ли очередь обхода.
func (q *URLQueue) IsEmpty() bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.items) == 0
}

type VisitedSet struct {
	urls map[string]bool
	mu   sync.Mutex
}

// NewVisitedSet создает набор посещенных URL.
func NewVisitedSet() *VisitedSet {
	return &VisitedSet{
		urls: make(map[string]bool),
	}
}

// Add добавляет URL в набор посещенных адресов.
func (v *VisitedSet) Add(url string) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.urls[url] = true
}

// Contains сообщает, есть ли URL в наборе посещенных адресов.
func (v *VisitedSet) Contains(url string) bool {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.urls[url]
}

// FormatError форматирует сообщение об ошибке.
func FormatError(format string, args ...interface{}) string {
	return fmt.Sprintf(format, args...)
}

// Min возвращает меньшее из двух целых чисел.
func Min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// NormalizeURL возвращает URL без фрагмента и лишнего корневого слеша.
func NormalizeURL(u *url.URL) string {
	normalized := *u
	normalized.Fragment = ""

	if normalized.Path == "/" {
		normalized.Path = ""
	}

	return normalized.String()
}

// NormalizeURLFromItem нормализует URL из строки элемента очереди.
func NormalizeURLFromItem(urlStr string) string {
	u, err := url.Parse(urlStr)
	if err != nil {
		return urlStr
	}
	return NormalizeURL(u)
}

// ResolveURL разрешает ссылку относительно базового URL.
func ResolveURL(base *url.URL, link string) string {
	if link == "" {
		return ""
	}

	parsed, err := url.Parse(link)
	if err != nil {
		return ""
	}

	if parsed.IsAbs() {
		return link
	}

	resolved := base.ResolveReference(parsed)
	return resolved.String()
}

// IsValidScheme сообщает, использует ли URL допустимую HTTP-схему.
func IsValidScheme(u string) bool {
	parsed, err := url.Parse(u)
	if err != nil {
		return false
	}

	if parsed.Scheme == "" {
		return false
	}

	return parsed.Scheme == "http" || parsed.Scheme == "https"
}

// ParseAndValidateURL разбирает строку URL и возвращает результат разбора.
func ParseAndValidateURL(urlStr string) (*url.URL, error) {
	return url.Parse(urlStr)
}

// MarshalReport кодирует отчет в JSON с опциональными отступами.
func MarshalReport(report interface{}, indent bool) ([]byte, error) {
	if indent {
		return json.MarshalIndent(report, "", "  ")
	}
	return json.Marshal(report)
}
