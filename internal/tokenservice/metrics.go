package tokenservice

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
)

func setupMetrics(transport http.RoundTripper) http.RoundTripper {
	if !metricsRegistered {
		metricsRegistered = true

		counter = prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "tokensevice_total",
				Help: "A counter for requests",
			},
			[]string{"method", "code"},
		)
		prometheus.MustRegister(counter)

		inFlightGauge = prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "tokenservice_inflight",
			Help: "A gauge of requests currently being served",
		})
		prometheus.MustRegister(inFlightGauge)

		// duration is partitioned by the HTTP method and handler. It uses custom
		// buckets based on the expected request duration.
		duration = prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "tokenservice_duration_seconds",
				Help:    "A histogram of latencies for requests.",
				Buckets: []float64{.1, .5, 1, 2.5, 5, 10},
			},
			[]string{"method", "code"},
		)
		prometheus.MustRegister(duration)
	}

	return promhttp.InstrumentRoundTripperInFlight(
		inFlightGauge,
		promhttp.InstrumentRoundTripperCounter(
			counter,
			promhttp.InstrumentRoundTripperDuration(duration, transport),
		),
	)
}
