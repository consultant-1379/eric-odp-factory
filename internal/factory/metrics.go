package factory

import (
	"github.com/prometheus/client_golang/prometheus"
)

var (
	metricsRegistered = false

	factoryRef *OdpFactoryImpl

	metricK8sRequests *prometheus.HistogramVec
	metricK8sErrors   *prometheus.CounterVec

	metricK8sEventInFlight *prometheus.GaugeVec
	metricK8sEventDuration *prometheus.HistogramVec

	metricCreatesRequested prometheus.Counter
	metricCreateInFlight   prometheus.Gauge
	metricCreate           *prometheus.HistogramVec
	metricOdpFinished      *prometheus.CounterVec

	metricOdpAll      prometheus.GaugeFunc
	metricOdpCreating prometheus.GaugeFunc
	metricOdpReady    prometheus.GaugeFunc
	metricOdpErrored  prometheus.GaugeFunc

	metricOdpCreationLatency prometheus.Histogram
	metricOdpReadyLatency    *prometheus.HistogramVec
	metricOdpE2ELatency      *prometheus.HistogramVec

	defaultBuckets = []float64{0.005, 0.010, 0.050, 0.100, 0.500, 1, 5}
	slowBuckets    = []float64{1, 5, 10, 50, 100, 200, 500}
)

const (
	All     = 1
	Errored = 2
)

func setupMetrics(factoryRefArg *OdpFactoryImpl) {
	factoryRef = factoryRefArg

	if !metricsRegistered {
		metricsRegistered = true

		regK8sMetrics()

		metricCreatesRequested = prometheus.NewCounter(prometheus.CounterOpts{
			Name: "factory_creates_requested",
			Help: "A counter of ODP created requuested",
		})
		prometheus.MustRegister(metricCreatesRequested)

		metricCreateInFlight = prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "factory_create_inflight",
			Help: "A gauge of requests currently being served",
		})
		prometheus.MustRegister(metricCreateInFlight)

		metricCreate = prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "factory_create",
				Help:    "A histogram of latencies for K8S requests.",
				Buckets: defaultBuckets,
			},
			[]string{"stage"},
		)
		prometheus.MustRegister(metricCreate)

		metricOdpFinished = prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "factory_odp_finished_total",
				Help: "A counter for ODP that have finished",
			},
			[]string{"result"},
		)
		prometheus.MustRegister(metricOdpFinished)

		metricOdpAll = prometheus.NewGaugeFunc(
			prometheus.GaugeOpts{Name: "factory_odp_all", Help: "All OPDs"},
			func() float64 { return getOdpCounts(All) },
		)
		prometheus.MustRegister(metricOdpAll)
		metricOdpCreating = prometheus.NewGaugeFunc(
			prometheus.GaugeOpts{Name: "factory_odp_creating", Help: "Creating OPDs"},
			func() float64 { return getOdpCounts(Creating) },
		)
		prometheus.MustRegister(metricOdpCreating)
		metricOdpReady = prometheus.NewGaugeFunc(
			prometheus.GaugeOpts{Name: "factory_odp_ready", Help: "Ready OPDs"},
			func() float64 { return getOdpCounts(Ready) },
		)
		prometheus.MustRegister(metricOdpReady)
		metricOdpErrored = prometheus.NewGaugeFunc(
			prometheus.GaugeOpts{Name: "factory_odp_errored", Help: "Errored OPDs"},
			func() float64 { return getOdpCounts(Errored) },
		)
		prometheus.MustRegister(metricOdpErrored)

		metricOdpCreationLatency = prometheus.NewHistogram(
			prometheus.HistogramOpts{
				Name:    "factory_odp_creation_latency",
				Help:    "A histogram of latencies for ODP creation requests.",
				Buckets: defaultBuckets,
			},
		)
		prometheus.MustRegister(metricOdpCreationLatency)
		metricOdpReadyLatency = prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "factory_odp_ready_latency",
				Help:    "A histogram of latencies for ODP reaching Ready state.",
				Buckets: slowBuckets,
			},
			[]string{"application"},
		)
		prometheus.MustRegister(metricOdpReadyLatency)
		metricOdpE2ELatency = prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "factory_odp_e2e_latency",
				Help:    "A histogram of e2e latencies for ODP.",
				Buckets: slowBuckets,
			},
			[]string{"application"},
		)
		prometheus.MustRegister(metricOdpE2ELatency)
	}
}

func regK8sMetrics() {
	metricK8sRequests = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "k8s_request",
			Help:    "A histogram of latencies for K8S requests.",
			Buckets: defaultBuckets,
		},
		[]string{"type", "method"},
	)
	prometheus.MustRegister(metricK8sRequests)

	metricK8sErrors = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "k8s_errors_total",
			Help: "A counter for K8S errors",
		},
		[]string{"type", "method"},
	)
	prometheus.MustRegister(metricK8sErrors)

	metricK8sEventInFlight = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "k8s_event_inflight",
			Help: "K8S events currently being processed",
		},
		[]string{"type"},
	)
	prometheus.MustRegister(metricK8sEventInFlight)
	metricK8sEventDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "k8s_event_duration_sec",
			Help:    "A histogram of latencies for K8S event handling.",
			Buckets: defaultBuckets,
		},
		[]string{"type"},
	)
	prometheus.MustRegister(metricK8sEventDuration)
}

func recordK8sRequest(typename string, method string, duration float64) {
	metricK8sRequests.WithLabelValues(typename, method).Observe(duration)
}

func recordK8sError(typename string, method string) {
	metricK8sErrors.WithLabelValues(typename, method).Inc()
}

func recordK8sEventDuration(typename string, duration float64) {
	metricK8sEventDuration.WithLabelValues(typename).Observe(duration)
}

func recordK8sEventInFlight(typename string, inc bool) {
	if inc {
		metricK8sEventInFlight.WithLabelValues(typename).Inc()
	} else {
		metricK8sEventInFlight.WithLabelValues(typename).Dec()
	}
}

func recordCreateRequested() {
	metricCreatesRequested.Inc()
}

func recordCreateInFlight(inc bool) {
	if inc {
		metricCreateInFlight.Inc()
	} else {
		metricCreateInFlight.Dec()
	}
}

func recordCreateStage(stage string, duration float64) {
	metricCreate.WithLabelValues(stage).Observe(duration)
}

func recordOdpFinished(result string) {
	metricOdpFinished.WithLabelValues(result).Inc()
}

func recordOdpCreationLatency(duration float64) {
	metricOdpCreationLatency.Observe(duration)
}

func recordOdpReadyLatency(application string, duration float64) {
	metricOdpReadyLatency.WithLabelValues(application).Observe(duration)
}

func recordOdpEeELatency(application string, duration float64) {
	metricOdpE2ELatency.WithLabelValues(application).Observe(duration)
}

func getOdpCounts(match int) float64 {
	factoryRef.odpsMutex.Lock()
	defer factoryRef.odpsMutex.Unlock()
	count := 0
	for _, odp := range factoryRef.odps {
		if match == All ||
			(match == Creating && odp.ResultCode == Creating) ||
			(match == Ready && odp.ResultCode == Ready) ||
			(match == Errored && odp.ResultCode > Ready) {
			count++
		}
	}

	return float64(count)
}
