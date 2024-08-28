package rest

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	metricsRegistered = false
	inFlightGauge     prometheus.Gauge
	counter           *prometheus.CounterVec
	duration          *prometheus.HistogramVec

	defaultBuckets = []float64{0.005, 0.010, 0.050, 0.100, 0.500, 1, 5}
)

func setupMetrics(handler http.Handler) http.Handler {
	if !metricsRegistered {
		metricsRegistered = true

		counter = prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "rest_total",
				Help: "A counter for requests",
			},
			[]string{"code"},
		)
		prometheus.MustRegister(counter)

		inFlightGauge = prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "rest_inflight",
			Help: "A gauge of requests currently being served",
		})
		prometheus.MustRegister(inFlightGauge)

		// duration is partitioned by the HTTP method and handler. It uses custom
		// buckets based on the expected request duration.
		duration = prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "rest_duration_seconds",
				Help:    "A histogram of latencies for requests.",
				Buckets: defaultBuckets,
			},
			[]string{"handler", "method"},
		)
		prometheus.MustRegister(duration)
	}

	return promhttp.InstrumentHandlerInFlight(
		inFlightGauge,
		promhttp.InstrumentHandlerDuration(
			duration.MustCurryWith(prometheus.Labels{"handler": "odp"}),
			promhttp.InstrumentHandlerCounter(counter, handler),
		),
	)
}
