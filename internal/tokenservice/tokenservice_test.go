package tokenservice

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"eric-odp-factory/internal/config"
	"eric-odp-factory/internal/dirwatcher"
	"eric-odp-factory/internal/testcommon"
)

const (
	expectedTokenName    = "name"
	expectedTokenDataKey = "name1"
	expectedTokenDataVal = "val1"
	hdrContentType       = "Content-Type"
)

var testCtx = context.WithValue(context.TODO(), "ctxid", "test") //nolint:revive,staticcheck // Ignore for test code

func TestCreateOkay(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		srvCheckRequest(t, r, http.MethodPost, "/odp-token")
		srvWriteResponse(w, true)
	}))
	defer server.Close()

	cfg := config.Config{TokenServiceURL: server.URL + "/odp-token"}

	tsc := NewTokenServiceClient(&cfg)
	odpToken, err := tsc.CreateToken(testCtx, "user1", []string{"sso"})
	t.Logf("odpToken: %v", odpToken)
	if err != nil {
		t.Errorf("unexpected error from CreateToken: %v", err)
	} else if odpToken.Name != expectedTokenName || odpToken.Data[expectedTokenDataKey] != expectedTokenDataVal {
		t.Errorf("unexpected token from CreateToken: %v", *odpToken)
	}
}

// Test where Content-Type includes trailing info, e.g. "application/json; charset=UTF-8".
func TestGetOkayContentType(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		srvCheckRequest(t, r, http.MethodGet, "/odp-token/name")
		srvWriteResponseContentType(w, true, applicationJSON+"; charset=UTF-8")
	}))
	defer server.Close()

	cfg := config.Config{TokenServiceURL: server.URL + "/odp-token"}

	tsc := NewTokenServiceClient(&cfg)
	odpToken, err := tsc.GetToken(testCtx, "name")
	if err != nil {
		t.Errorf("unexpected error from GetToken: %v", err)
	} else if odpToken.Name != expectedTokenName || odpToken.Data[expectedTokenDataKey] != expectedTokenDataVal {
		t.Errorf("unexpected token from GetToken: %v", *odpToken)
	}
}

func TestTLS(t *testing.T) {
	certDir := t.TempDir()
	caCert, caKey := testcommon.CreateCertPair(t, "tokenserviceca", true, nil, nil, certDir)
	testcommon.CreateCertPair(t, "tokenservice", false, caCert, caKey, certDir)

	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		srvCheckRequest(t, r, http.MethodPost, "/odp-token")
		srvWriteResponse(w, true)
	}))

	serverTLSCert, _ := tls.LoadX509KeyPair(certDir+"/tokenservice/tls.crt", certDir+"/tokenservice/tls.key")
	certPool := x509.NewCertPool()
	caCertPEM, err := os.ReadFile(certDir + "/tokenserviceca/tls.crt")
	if err != nil {
		panic(err)
	}
	certPool.AppendCertsFromPEM(caCertPEM)
	tlsConfig := &tls.Config{
		ClientAuth:   tls.RequireAndVerifyClientCert,
		ClientCAs:    certPool,
		Certificates: []tls.Certificate{serverTLSCert},
	}
	server.TLS = tlsConfig
	server.StartTLS()
	t.Logf("server URL: %s", server.URL)

	testcommon.CreateCertPair(t, "trustedclient", false, caCert, caKey, certDir)

	dirwatcher.Start()

	cfg := config.Config{
		TokenServiceURL:      server.URL + "/odp-token",
		TokenServiceCAFile:   certDir + "/tokenserviceca/tls.crt",
		TokenServiceCertFile: certDir + "/trustedclient/tls.crt",
		TokenServiceKeyFile:  certDir + "/trustedclient/tls.key",
	}
	tsc := NewTokenServiceClient(&cfg)
	odpToken, err := tsc.CreateToken(testCtx, "user1", []string{"sso"})
	if err != nil {
		t.Errorf("unexpected error from CreateToken: %v", err)
	} else if odpToken.Name != expectedTokenName || odpToken.Data[expectedTokenDataKey] != expectedTokenDataVal {
		t.Errorf("unexpected token from CreateToken: %v", *odpToken)
	}

	dirwatcher.Stop()
}

func TestGetOkay(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		srvCheckRequest(t, r, http.MethodGet, "/odp-token/name")
		srvWriteResponse(w, true)
	}))
	defer server.Close()

	cfg := config.Config{TokenServiceURL: server.URL + "/odp-token"}

	tsc := NewTokenServiceClient(&cfg)
	odpToken, err := tsc.GetToken(testCtx, "name")
	if err != nil {
		t.Errorf("unexpected error from GetToken: %v", err)
	} else if odpToken.Name != expectedTokenName || odpToken.Data[expectedTokenDataKey] != expectedTokenDataVal {
		t.Errorf("unexpected token from GetToken: %v", *odpToken)
	}
}

func TestDeleteOkay(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		srvCheckRequest(t, r, http.MethodDelete, "/odp-token/name")
		srvWriteResponse(w, false)
	}))
	defer server.Close()

	cfg := config.Config{TokenServiceURL: server.URL + "/odp-token"}

	tsc := NewTokenServiceClient(&cfg)
	err := tsc.DeleteToken(testCtx, "name")
	if err != nil {
		t.Errorf("unexpected error from GetToken: %v", err)
	}
}

func TestFailStatusCode(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	cfg := config.Config{TokenServiceURL: server.URL + "/odp-token"}
	tsc := NewTokenServiceClient(&cfg)
	verifyError(t, tsc)
}

func TestTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		time.Sleep(time.Millisecond * 100)
	}))
	defer server.Close()

	cfg := config.Config{TokenServiceURL: server.URL + "/odp-token"}
	tsc := NewTokenServiceClient(&cfg)
	tsc.httpClient.Timeout = time.Millisecond
	verifyError(t, tsc)
}

func TestFailInvalidURL(t *testing.T) {
	cfg := config.Config{TokenServiceURL: "blah blah blah"}
	tsc := NewTokenServiceClient(&cfg)
	odpToken, err := tsc.GetToken(testCtx, "name")
	if err == nil {
		t.Errorf("expected error from GetToken: %v", odpToken)
	}
}

func TestFailBodyRead(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Add(hdrContentType, applicationJSON)
		w.Header().Add("Content-Length", "500")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	cfg := config.Config{TokenServiceURL: server.URL + "/odp-token"}
	tsc := NewTokenServiceClient(&cfg)
	odpToken, err := tsc.GetToken(testCtx, "name")
	if err == nil {
		t.Errorf("expected error from GetToken: %v", odpToken)
	}
}

func TestFailContentType(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		srvCheckRequest(t, r, http.MethodGet, "/odp-token/name")
		srvWriteResponseContentType(w, true, "plain/text")
	}))
	defer server.Close()

	cfg := config.Config{TokenServiceURL: server.URL + "/odp-token"}
	tsc := NewTokenServiceClient(&cfg)
	odpToken, err := tsc.GetToken(testCtx, "name")
	if err == nil {
		t.Errorf("expected error from GetToken: %v", odpToken)
	}
}

func TestFailInvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		srvCheckRequest(t, r, http.MethodGet, "/odp-token/name")
		w.Header().Add(hdrContentType, applicationJSON)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`"tokenname":"name","tokendata":{"name1":"val1"}}`))
	}))
	defer server.Close()

	cfg := config.Config{TokenServiceURL: server.URL + "/odp-token"}
	tsc := NewTokenServiceClient(&cfg)
	odpToken, err := tsc.GetToken(testCtx, "name")
	if err == nil {
		t.Errorf("expected error from GetToken: %v", odpToken)
	}
}

func verifyError(t *testing.T, tsc *ClientImpl) {
	odpToken, err := tsc.CreateToken(testCtx, "user1", []string{"sso"})
	if err == nil {
		t.Errorf("expected error from CreateToken: %v", odpToken)
	}

	odpToken, err = tsc.GetToken(testCtx, "name")
	if err == nil {
		t.Errorf("expected error from GetToken: %v", odpToken)
	}

	err = tsc.DeleteToken(testCtx, "name")
	if err == nil {
		t.Errorf("expected error from GetToken: %v", odpToken)
	}
}

func srvCheckRequest(t *testing.T, r *http.Request, expectedMethod, expectedPath string) {
	if r.Method != expectedMethod {
		t.Errorf("Expected to method %s, got: %s", expectedMethod, r.Method)
	}
	if r.URL.Path != expectedPath {
		t.Errorf("Expected to request %s, got: %s", expectedPath, r.URL.Path)
	}
	if r.Header.Get("Accept") != applicationJSON {
		t.Errorf("Expected Accept: application/json header, got: %s", r.Header.Get("Accept"))
	}
}

func srvWriteResponse(w http.ResponseWriter, writeToken bool) {
	srvWriteResponseContentType(w, writeToken, applicationJSON)
}

func srvWriteResponseContentType(w http.ResponseWriter, writeToken bool, contentType string) {
	if writeToken {
		w.Header().Add(hdrContentType, contentType)
	}

	w.WriteHeader(http.StatusOK)

	if writeToken {
		w.Write([]byte(`{"tokenname":"name","tokendata":{"name1":"val1"}}`))
	}
}
