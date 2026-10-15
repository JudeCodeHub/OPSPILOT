package booking

import (
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/git4jude/opspilot/services/internal/platform/metrics"
)

// Metrics are booking's business and connection-pool metrics.
type Metrics struct {
	HoldsCreated      prometheus.Counter
	HoldConflicts     prometheus.Counter
	BookingsConfirmed prometheus.Counter
	CheckoutFailures  *prometheus.CounterVec // reason: expired|declined|payment_timeout|payment_unavailable|conflict|internal
	PoolAcquireWait   prometheus.Histogram
}

// NewMetrics registers booking's metrics.
func NewMetrics(m *metrics.Registry) *Metrics {
	f := m.Factory
	bm := &Metrics{
		HoldsCreated: f.NewCounter(prometheus.CounterOpts{
			Name: "booking_holds_created_total", Help: "Seat holds created.",
		}),
		HoldConflicts: f.NewCounter(prometheus.CounterOpts{
			Name: "booking_hold_conflicts_total", Help: "Hold attempts on a seat that was already held or sold.",
		}),
		BookingsConfirmed: f.NewCounter(prometheus.CounterOpts{
			Name: "bookings_confirmed_total", Help: "Successful checkouts.",
		}),
		CheckoutFailures: f.NewCounterVec(prometheus.CounterOpts{
			Name: "booking_checkout_failures_total", Help: "Failed checkouts by reason.",
		}, []string{"reason"}),
		PoolAcquireWait: f.NewHistogram(prometheus.HistogramOpts{
			Name:    "booking_db_pool_acquire_wait_seconds",
			Help:    "Time spent waiting for a free database connection.",
			Buckets: []float64{0.001, 0.005, 0.01, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10},
		}),
	}
	// Pre-create every reason so the series exist (at 0) before the first failure.
	for _, reason := range []string{"expired", "declined", "payment_timeout", "payment_unavailable", "conflict", "internal"} {
		bm.CheckoutFailures.WithLabelValues(reason)
	}
	return bm
}

// RegisterPoolGauges exposes the pool's current and maximum connections.
func RegisterPoolGauges(m *metrics.Registry, pool *pgxpool.Pool) {
	m.Factory.NewGaugeFunc(prometheus.GaugeOpts{
		Name: "booking_db_pool_acquired_conns", Help: "Database connections currently in use.",
	}, func() float64 { return float64(pool.Stat().AcquiredConns()) })
	m.Factory.NewGaugeFunc(prometheus.GaugeOpts{
		Name: "booking_db_pool_max_conns", Help: "Maximum size of the database connection pool.",
	}, func() float64 { return float64(pool.Stat().MaxConns()) })
}
