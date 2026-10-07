package router

import (
	"net/http"
	"sort"
	"sync"
)

// Mux is what each module needs to register its routes.
type Mux interface {
	Handle(pattern string, h http.Handler)
	HandleFunc(pattern string, h func(http.ResponseWriter, *http.Request))
}

// Router is a ServeMux that remembers every pattern it was given,
// so tests can compare the real routes with the API documentation.
type Router struct {
	*http.ServeMux
	mu       sync.Mutex
	patterns []string
}

func New() *Router { return &Router{ServeMux: http.NewServeMux()} }

func (r *Router) Handle(pattern string, h http.Handler) {
	r.record(pattern)
	r.ServeMux.Handle(pattern, h)
}

func (r *Router) HandleFunc(pattern string, h func(http.ResponseWriter, *http.Request)) {
	r.record(pattern)
	r.ServeMux.HandleFunc(pattern, h)
}

func (r *Router) record(pattern string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.patterns = append(r.patterns, pattern)
}

// Patterns lists the registered "METHOD /path" patterns, sorted.
func (r *Router) Patterns() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := append([]string(nil), r.patterns...)
	sort.Strings(out)
	return out
}
