package httpx

import (
	"net/http"
	"strings"
	"time"
)

// RouteUnmatched is the route label for requests that matched no pattern.
const RouteUnmatched = "unmatched"

// requestState is per-request bookkeeping shared between middleware and the
// router. It is stored as a pointer in the context by the RequestID middleware
// so the router can fill in the matched route after the outer middleware has
// already been entered.
type requestState struct {
	start   time.Time
	route   string
	onRoute func(route string) // set by Metrics; called once the route is known
}

type stateKey struct{}

func stateFrom(r *http.Request) *requestState {
	s, _ := r.Context().Value(stateKey{}).(*requestState)
	return s
}

// Route returns the matched route pattern without the method ("/events/{id}"),
// or RouteUnmatched. Valid once the router has dispatched the request.
func Route(r *http.Request) string {
	if s := stateFrom(r); s != nil && s.route != "" {
		return s.route
	}
	return RouteUnmatched
}

// StartTime returns when the request entered the middleware chain.
func StartTime(r *http.Request) time.Time {
	if s := stateFrom(r); s != nil {
		return s.start
	}
	return time.Now()
}

// Router wraps http.ServeMux (Go's built-in pattern router) and records the
// matched pattern so logs and metrics can use "/events/{id}" instead of the
// raw path, which would explode label cardinality.
type Router struct {
	mux *http.ServeMux
}

// NewRouter returns a router whose unknown routes answer with a JSON 404.
func NewRouter() *Router {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		WriteError(w, r, http.StatusNotFound, CodeNotFound, "The requested resource does not exist.")
	})
	return &Router{mux: mux}
}

// HandleFunc registers h for a pattern such as "GET /events/{id}".
func (rt *Router) HandleFunc(pattern string, h http.HandlerFunc) {
	rt.Handle(pattern, h)
}

// Handle registers h for a pattern such as "GET /events/{id}".
func (rt *Router) Handle(pattern string, h http.Handler) {
	route := routeLabel(pattern)
	rt.mux.Handle(pattern, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s := stateFrom(r); s != nil {
			s.route = route
			if s.onRoute != nil {
				s.onRoute(route)
			}
		}
		h.ServeHTTP(w, r)
	}))
}

// ServeHTTP dispatches to the registered handlers.
func (rt *Router) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	rt.mux.ServeHTTP(w, r)
}

// routeLabel strips the method from a pattern: "GET /events/{id}" -> "/events/{id}".
func routeLabel(pattern string) string {
	if i := strings.IndexByte(pattern, ' '); i >= 0 {
		return strings.TrimSpace(pattern[i+1:])
	}
	return pattern
}
