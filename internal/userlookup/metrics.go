package userlookup

import (
	"github.com/prometheus/client_golang/prometheus"
)

var (
	metricsRegistered = false
	metricRequests    *prometheus.HistogramVec
	metricErrors      *prometheus.CounterVec

	defaultBuckets = []float64{0.005, 0.010, 0.050, 0.100, 0.500, 1, 5}
)

func setupMetrics() {
	if !metricsRegistered {
		metricsRegistered = true

		// metricRequests is partitioned by the LDAP method and handler. It uses custom
		// buckets based on the expected request duration.
		metricRequests = prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "ldap_request",
				Help:    "A histogram of latencies for requests.",
				Buckets: defaultBuckets,
			},
			[]string{"method"},
		)
		prometheus.MustRegister(metricRequests)

		metricErrors = prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "ldap_errors_total",
				Help: "A counter for LDAP errors",
			},
			[]string{"op"},
		)
		prometheus.MustRegister(metricErrors)
	}
}

func recordLdapRequest(op string, duration float64) {
	metricRequests.WithLabelValues(op).Observe(duration)
}

func recordLdapError(op string) {
	metricErrors.WithLabelValues(op).Inc()
}
