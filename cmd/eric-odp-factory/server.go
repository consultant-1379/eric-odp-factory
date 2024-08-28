package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"eric-odp-factory/internal/config"
	"eric-odp-factory/internal/dirwatcher"
	"eric-odp-factory/internal/factory"
	"eric-odp-factory/internal/rest"
	"eric-odp-factory/internal/tokenservice"
	"eric-odp-factory/internal/userlookup"

	"github.com/prometheus/client_golang/prometheus/promhttp"

	"k8s.io/client-go/kubernetes"
	k8srest "k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

const (
	readTimeout  = 10 * time.Second
	writeTimeout = 10 * time.Second
	idleTimeout  = 30 * time.Second
)

var (
	appConfig        *config.Config
	isReady          int32
	exitSignal       chan os.Signal
	testK8sInterface kubernetes.Interface
	healthHTTPD      *http.Server
	metricsHTTPD     *http.Server
)

func readinessHealthCheck(w http.ResponseWriter, _ *http.Request) {
	if atomic.LoadInt32(&isReady) == 1 {
		_, _ = fmt.Fprintf(w, "OK")

		return
	}

	slog.Error("Service is not ready")

	w.WriteHeader(http.StatusServiceUnavailable)
}

func getExitSignalsChannel() chan os.Signal {
	channel := make(chan os.Signal, 1)
	signal.Notify(channel,
		syscall.SIGTERM,
		syscall.SIGINT,
		syscall.SIGQUIT,
		syscall.SIGHUP,
	)

	return channel
}

func initHealthCheck() {
	addr := fmt.Sprintf(":%d", appConfig.HealthCheckPort)
	mux := http.NewServeMux()

	mux.HandleFunc("/health/liveness", readinessHealthCheck)
	mux.HandleFunc("/health/readiness", readinessHealthCheck)

	slog.Info("Adding /debug/ handler")
	mux.HandleFunc("/debug/", handleDebug)

	healthHTTPD = &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  readTimeout,
		WriteTimeout: writeTimeout,
		IdleTimeout:  idleTimeout,
	}
	slog.Info("Starting healthcheck httpd service", "addr", addr)
	if err := healthHTTPD.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("healthcheck httpd service error %v", err)
	}

	slog.Info("healthcheck httpd service shutdown")
}

func initMetricsProvider() {
	addr := fmt.Sprintf(":%d", appConfig.MetricsPort)
	mux := http.NewServeMux()

	mux.Handle("/metrics", promhttp.Handler())

	metricsHTTPD = &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  readTimeout,
		WriteTimeout: writeTimeout,
		IdleTimeout:  idleTimeout,
	}

	slog.Info("Starting metrics httpd service", "addr", addr)
	if err := metricsHTTPD.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("metrics httpd service error %v", err)
	}

	slog.Info("metrics httpd service shutdown")
}

func getClient(odpConfig *config.Config) (kubernetes.Interface, error) {
	if testK8sInterface != nil {
		return testK8sInterface, nil
	}

	var cfg *k8srest.Config
	var err error
	if odpConfig.Kubeconfig != "" {
		// If we're running a test with KUBECONFIG set (out of cluster)
		slog.Info("getClient", "Kubeconfig", odpConfig.Kubeconfig)
		cfg, err = clientcmd.BuildConfigFromFlags("", odpConfig.Kubeconfig)
	} else {
		// Normal case (inside cluster)
		cfg, err = k8srest.InClusterConfig()
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get client config: %w", err)
	}

	// Set QPS and Burst
	// QPS indicates the maximum QPS to the master from this client.
	// If it's zero, the created RESTClient will use DefaultQPS: 5
	// Maximum burst for throttle.
	// If it's zero, the created RESTClient will use DefaultBurst: 10.

	// We need 1 request per create and 1 request for delete
	// so we can support ~ QPS / 2 rate of ODPs
	cfg.QPS = float32(odpConfig.K8sQPS)
	cfg.Burst = odpConfig.K8sQPS * 2 //nolint:gomnd // This isn't a magic number as it's based on a parameter

	slog.Info("getClient cfg", "QPS", cfg.QPS, "Burst", cfg.Burst)

	// creates the clientset
	var clientSetErr error
	clientset, clientSetErr := kubernetes.NewForConfig(cfg)
	if clientSetErr != nil {
		return nil, fmt.Errorf("failed to get client: %w", clientSetErr)
	}

	slog.Info(
		"getClient",
		"RateLimiter QPS",
		clientset.DiscoveryClient.RESTClient().GetRateLimiter().QPS(),
	)

	return clientset, nil
}

func main() {
	if err := dirwatcher.Start(); err != nil {
		log.Fatalf("failed to start dirwatcher: %v", err)
	}

	appConfig = config.GetConfig()
	initLogging()

	slog.Info("Starting service", "appConfig", appConfig)

	ctx, cancel := context.WithCancel(context.Background())
	exitSignal = getExitSignalsChannel()

	atomic.StoreInt32(&isReady, 0)

	wg := sync.WaitGroup{}

	go initHealthCheck()
	go initMetricsProvider()

	var clientSet kubernetes.Interface
	var err error
	if clientSet, err = getClient(appConfig); err != nil {
		log.Fatalf("getClient failed: %v", err)
	}

	userLookup, err := userlookup.NewUserLookup(appConfig)
	if err != nil {
		log.Fatal(err)
	}

	var tokenServiceClient tokenservice.Client
	if appConfig.TokenServiceURL != "" {
		tokenServiceClient = tokenservice.NewTokenServiceClient(appConfig)
	}
	odpFactory := factory.NewOdpFactory(ctx, appConfig, clientSet, userLookup, tokenServiceClient)
	restAPI := rest.NewRestApi(appConfig, odpFactory)

	atomic.StoreInt32(&isReady, 1)
	slog.Info("Startup completed")

	// Wait for exit signal
	<-exitSignal
	slog.Info("Received exit signal")

	// Shutdown
	cancel()

	healthHTTPD.Shutdown(context.Background())  //nolint:errcheck // We're shutting down so don't care about result
	metricsHTTPD.Shutdown(context.Background()) //nolint:errcheck // We're shutting down so don't care about result

	restAPI.Stop()
	odpFactory.Stop()
	dirwatcher.Stop()

	slog.Info("Waiting for the services to terminate")
	wg.Wait()
	slog.Info("Terminated service")
}
