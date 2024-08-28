package rest

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
)

type TestHandler struct {
	t *testing.T
}

func (th *TestHandler) ServeHTTP(_ http.ResponseWriter, _ *http.Request) {
	th.t.Log("ServerHTTP called")
}

func TestMetrics(t *testing.T) {
	handler := TestHandler{t: t}

	handlerWrapper := setupMetrics(&handler)

	request := httptest.NewRequest(http.MethodGet, "/odp/", strings.NewReader(""))
	response := httptest.NewRecorder()
	handlerWrapper.ServeHTTP(response, request)

	metricFamilies, _ := prometheus.DefaultGatherer.Gather()
	foundCount := 0
	for _, metricFamily := range metricFamilies {
		if *metricFamily.Name == "rest_total" ||
			*metricFamily.Name == "rest_inflight" ||
			*metricFamily.Name == "rest_duration_seconds" {
			foundCount++
		}
	}
	expectedCount := 3
	if foundCount != expectedCount {
		t.Errorf("expectedCount = %d, foundCount = %d", expectedCount, foundCount)
	}
}
