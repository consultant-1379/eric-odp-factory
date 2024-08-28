package rest

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"io"
	"log"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"eric-odp-factory/internal/certwatcher"
	"eric-odp-factory/internal/config"
	"eric-odp-factory/internal/factory"
)

type OnDemandPodRequest struct {
	Username    string            `json:"username"`
	Application string            `json:"application"`
	TokenTypes  []string          `json:"tokentypes"`
	Data        map[string]string `json:"data"`
	InstanceID  string            `json:"instanceid"`
	Profile     string            `json:"profile"`
}

type OnDemandPodReply struct {
	PodName    string            `json:"podname"`
	ResultCode int               `json:"resultcode"`
	TokenData  map[string]string `json:"tokendata"`
	PodIPs     []string          `json:"podips"`
	Error      string            `json:"error"`
}

type Instance struct {
	httpServer   *http.Server
	odpFactory   factory.OdpFactory
	httpdRunning sync.WaitGroup
}

type Interface interface {
	Stop()
}

const (
	readHeaderTimeoutSec = 5
	megaByte             = 1048576
)

func NewRestApi( //nolint:revive,stylecheck // RestApi easier to read
	cfg *config.Config,
	odpFactory factory.OdpFactory,
) *Instance {
	slog.Info("NewRestApi", "RestTLSEnabled", cfg.RestTLSEnabled)

	raInst := Instance{odpFactory: odpFactory}
	// Create a new request multiplexer
	// Takes incoming requests and dispatch them to the matching handlers
	mux := http.NewServeMux()

	// Register the routes and handlers
	mux.Handle("/odp/", setupMetrics(&raInst))

	httpServer := http.Server{
		Addr:              cfg.RestListenAddr,
		Handler:           mux,
		ReadHeaderTimeout: readHeaderTimeoutSec * time.Second,
	}

	if cfg.RestTLSEnabled {
		fullpath, _ := filepath.Abs(cfg.RestCertFile)
		certDir := filepath.Dir(fullpath)
		certFileName := filepath.Base(cfg.RestCertFile)
		certKeyName := filepath.Base(cfg.RestKeyFile)
		certWatcher := certwatcher.NewWatcher(certDir, certFileName, certKeyName)

		caCert, err := os.ReadFile(cfg.RestCAFile)
		if err != nil {
			log.Fatal(err)
		}
		caCertPool := x509.NewCertPool()
		caCertPool.AppendCertsFromPEM(caCert)

		httpServer.TLSConfig = &tls.Config{
			MinVersion:     tls.VersionTLS12,
			GetCertificate: certWatcher.GetCertificate,
			ClientAuth:     tls.RequireAndVerifyClientCert,
			ClientCAs:      caCertPool,
		}
	}

	//
	raInst.httpdRunning.Add(1)

	// Run HTTP server.
	go func() {
		defer raInst.httpdRunning.Done()

		var err error
		if cfg.RestTLSEnabled {
			err = httpServer.ListenAndServeTLS("", "")
		} else {
			err = httpServer.ListenAndServe()
		}
		if !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("HTTP Server error %v", err)
		}
		slog.Info("httpServer stopped")
	}()

	raInst.httpServer = &httpServer

	return &raInst
}

func (r *Instance) ServeHTTP(reply http.ResponseWriter, request *http.Request) {
	slog.Info("ServeHTTP Entered")

	contentType := request.Header.Get("Content-Type")
	if contentType != "" {
		mediaType := strings.ToLower(strings.TrimSpace(strings.Split(contentType, ";")[0]))
		if mediaType != "application/json" {
			http.Error(reply, "Content-Type header is not application/json", http.StatusInternalServerError)

			return
		}
	}

	requestData, err := io.ReadAll(http.MaxBytesReader(reply, request.Body, megaByte))
	if err != nil {
		slog.Error("Failed to read request body", "err", err)
		http.Error(reply, "Fail to read request body", http.StatusInternalServerError)

		return
	}

	var odpRequest OnDemandPodRequest
	decoder := json.NewDecoder(bytes.NewBuffer(requestData))
	// Fail if the request includes any unknown fields.
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(&odpRequest); err != nil {
		slog.Error("ServeHTTP failed to decode request", "err", err, "requestData", requestData)
		internalServerError(reply, request)

		return
	}

	odpParam := factory.OnDemandPodParam{
		Username:    odpRequest.Username,
		Application: odpRequest.Application,
		TokenTypes:  odpRequest.TokenTypes,
		RequestData: odpRequest.Data,
		InstanceID:  odpRequest.InstanceID,
		Profile:     odpRequest.Profile,
	}

	odp := r.odpFactory.GetOnDemandPod(&odpParam)
	slog.Debug("handleOdpGet", "odp", odp)

	result := OnDemandPodReply{
		PodName:    odp.PodName,
		ResultCode: odp.ResultCode,
		TokenData:  odp.TokenData,
		PodIPs:     odp.PodIPs,
	}
	if !odp.ErrorTime.IsZero() {
		result.Error = odp.Error.Error()
	}

	resultBytes, err := json.Marshal(result)
	if err != nil {
		internalServerError(reply, request)

		return
	}

	reply.Header().Set("Content-Type", "application/json")
	reply.WriteHeader(http.StatusOK)
	//nolint:errcheck // Suppress errcheck cause there's nothing to do if we get an error
	reply.Write(resultBytes)
}

func (r *Instance) Stop() {
	//nolint:errcheck // Suppress errcheck cause there's nothing to do if we get an error
	r.httpServer.Shutdown(context.Background())
	slog.Info("Stop waiting while httpdRunning")
	r.httpdRunning.Wait()
	slog.Info("Stop returning")
}

func internalServerError(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusInternalServerError)
	//nolint:errcheck // We can't do anything with the error
	w.Write([]byte("500 Internal Server Error"))
}
