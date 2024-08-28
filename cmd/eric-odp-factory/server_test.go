package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"k8s.io/client-go/kubernetes/fake"

	"eric-odp-factory/internal/config"
)

const kubeConfig = `apiVersion: v1
kind: Config
clusters:
- name: dummy
  cluster:
    server: http://localhost:7890/
contexts:
- name: dummy
  context:
    cluster: dummy
    namespace: default
    user: dummmy
current-context: dummy
`

func doHealthCheck(t *testing.T, expectedStatusCode int) {
	request := httptest.NewRequest(http.MethodGet, "/health/liveness", strings.NewReader(""))
	responseRecorder := httptest.NewRecorder()
	readinessHealthCheck(responseRecorder, request)

	response := responseRecorder.Result()
	gotStatusCode := response.StatusCode
	response.Body.Close()
	if gotStatusCode != expectedStatusCode {
		t.Errorf("unexpected statusCode to GET expected=%d got=%d", expectedStatusCode, gotStatusCode)
	}
}

func TestReadinessHealthCheck(t *testing.T) {
	atomic.StoreInt32(&isReady, 0)

	doHealthCheck(t, http.StatusServiceUnavailable)

	atomic.StoreInt32(&isReady, 1)
	doHealthCheck(t, http.StatusOK)
}

func TestGetClient(t *testing.T) {
	configDir := t.TempDir()
	kubeConfigFile := filepath.Join(configDir, "kubeconfig")
	os.WriteFile(kubeConfigFile, []byte(kubeConfig), 0o644)

	testconfig := config.Config{Kubeconfig: kubeConfigFile}
	if _, err := getClient(&testconfig); err != nil {
		t.Errorf("failed to getClient: %v", err)
	}
}

func TestMain(t *testing.T) {
	testK8sInterface = fake.NewSimpleClientset()

	// Getting conflict in Jenkins with port 8002 so move to another port
	os.Setenv("HEALTH_CHECK_PORT", "28002")
	os.Setenv("METRICS_PORT", "28003")

	var mainRunning atomic.Bool
	go func() {
		mainRunning.Store(true)
		main()
		mainRunning.Store(false)
	}()

	t.Log("Waiting for main to set isReady")
	for i := 0; i <= 100 && atomic.LoadInt32(&isReady) == 0; i++ {
		time.Sleep(100 * time.Millisecond)
	}
	if atomic.LoadInt32(&isReady) == 0 {
		t.Error("Service not ready")
	}

	t.Log("Make sure we are fully started")
	time.Sleep(time.Second)

	t.Log("Sending SIGTERM")
	exitSignal <- syscall.SIGTERM

	t.Log("Waiting for mainRunning to become false")
	for i := 0; i <= 100 && mainRunning.Load(); i++ {
		time.Sleep(100 * time.Millisecond)
	}
	if mainRunning.Load() {
		t.Error("Main still running")
	}
}
