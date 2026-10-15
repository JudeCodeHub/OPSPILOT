// Package metrics owns the Prometheus registry of a service.
//
// It registers the Go runtime and process collectors (go_memstats_*,
// go_goroutines, process_resident_memory_bytes, ...), which is how memory
// leaks and goroutine leaks get noticed, plus the standard HTTP server and
// upstream-client metrics used by the httpx package.
//
// Label cardinality rule: label values must come from a small, fixed set.
// Never use IDs, emails or raw URL paths as labels; use the route pattern.
package metrics

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Registry is a service's metric registry plus the shared HTTP metrics.
type Registry struct {
	reg *prometheus.Registry

	// Factory creates service-specific metrics that are registered automatically.
	Factory promauto.Factory

	// Server-side HTTP metrics, recorded by httpx.Metrics middleware.
	HTTPRequests *prometheus.CounterVec   // labels: method, route, status
	HTTPDuration *prometheus.HistogramVec // labels: method, route, status
	HTTPInFlight *prometheus.GaugeVec     // labels: method, route

	// Client-side metrics for service-to-service calls, recorded by httpx.Client.
	UpstreamRequests *prometheus.CounterVec   // labels: target, status
	UpstreamDuration *prometheus.HistogramVec // labels: target
}

// DurationBuckets are the histogram buckets for request latencies: 5ms … 10s.
var DurationBuckets = []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10}

// New creates the registry for the named service.
func New(service string) *Registry {
	reg := prometheus.NewRegistry()
	reg.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
	)

	factory := promauto.With(reg)
	// "service" is constant per process, so it is a const label.
	svc := prometheus.Labels{"service": service}

	return &Registry{
		reg:     reg,
		Factory: factory,
		HTTPRequests: factory.NewCounterVec(prometheus.CounterOpts{
			Name:        "http_requests_total",
			Help:        "HTTP requests handled, by route pattern and status code.",
			ConstLabels: svc,
		}, []string{"method", "route", "status"}),
		HTTPDuration: factory.NewHistogramVec(prometheus.HistogramOpts{
			Name:        "http_request_duration_seconds",
			Help:        "HTTP request latency, by route pattern and status code.",
			ConstLabels: svc,
			Buckets:     DurationBuckets,
		}, []string{"method", "route", "status"}),
		HTTPInFlight: factory.NewGaugeVec(prometheus.GaugeOpts{
			Name:        "http_requests_in_flight",
			Help:        "HTTP requests currently being served.",
			ConstLabels: svc,
		}, []string{"method", "route"}),
		UpstreamRequests: factory.NewCounterVec(prometheus.CounterOpts{
			Name:        "upstream_requests_total",
			Help:        "Requests to other services, by target and status (HTTP code, timeout or error).",
			ConstLabels: svc,
		}, []string{"target", "status"}),
		UpstreamDuration: factory.NewHistogramVec(prometheus.HistogramOpts{
			Name:        "upstream_request_duration_seconds",
			Help:        "Latency of requests to other services.",
			ConstLabels: svc,
			Buckets:     DurationBuckets,
		}, []string{"target"}),
	}
}

// Handler serves the registry in the Prometheus text format (GET /metrics).
func (r *Registry) Handler() http.Handler {
	return promhttp.HandlerFor(r.reg, promhttp.HandlerOpts{Registry: r.reg})
}

// MustRegister registers extra collectors (e.g. GaugeFuncs built outside Factory).
func (r *Registry) MustRegister(cs ...prometheus.Collector) {
	r.reg.MustRegister(cs...)
}
