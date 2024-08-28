package rest

import (
	"crypto/tls"
	"crypto/x509"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"eric-odp-factory/internal/config"
	"eric-odp-factory/internal/dirwatcher"
	"eric-odp-factory/internal/factory"
	"eric-odp-factory/internal/testcommon"
)

type TestOdpFactory struct {
	odp *factory.OnDemandPod
}

func (t *TestOdpFactory) GetOnDemandPod(_ *factory.OnDemandPodParam) *factory.OnDemandPod {
	return t.odp
}

func (t *TestOdpFactory) Stop() {
}

func TestOdpRestOkay(t *testing.T) {
	odp := factory.OnDemandPod{
		Username:    "testuser",
		Application: "testapp",
		TokenTypes:  []string{"testtoken"},
		PodName:     "testapp-testuser",
		ResultCode:  factory.Creating,
	}

	cfg := config.Config{
		RestTLSEnabled: false,
		RestListenAddr: "localhost:9876",
	}
	odpFactory := &TestOdpFactory{odp: &odp}

	//nolint:revive,stylecheck // Suppress var restApiImpl should be restAPIImpl
	restApiImpl := NewRestApi(&cfg, odpFactory)

	requestString := "{ \"username\": \"testuser\", \"application\": \"testapp\", \"tokentypes\": [ \"testtoken\"] }"

	request := httptest.NewRequest(http.MethodGet, "/odp/", strings.NewReader(requestString))
	response := httptest.NewRecorder()
	restApiImpl.ServeHTTP(response, request)

	// Verify we get the expected content-type
	expectedContentType := "application/json"
	gotContentType := response.Header().Get("Content-Type")
	if gotContentType != expectedContentType {
		t.Errorf("expected content-type = %s, got %s", expectedContentType, gotContentType)
	}

	// Verify we get the expected content
	res := response.Result()
	defer res.Body.Close()
	data, err := io.ReadAll(res.Body)
	if err != nil {
		t.Errorf("expected error to be nil got %v", err)
	}
	//nolint:lll // Ignore this line
	expectedContent := "{\"podname\":\"testapp-testuser\",\"resultcode\":-1,\"tokendata\":null,\"podips\":null,\"error\":\"\"}"
	gotContent := string(data)
	if gotContent != expectedContent {
		t.Errorf("expected contenr = %s got %s", expectedContent, gotContent)
	}

	restApiImpl.Stop()
}

func TestOdpRestMalformedRequest(t *testing.T) {
	odp := factory.OnDemandPod{
		Username:    "testuser",
		Application: "testapp",
		TokenTypes:  []string{"testtoken"},
		PodName:     "testapp-testuser",
		ResultCode:  factory.Creating,
	}

	cfg := config.Config{
		RestTLSEnabled: false,
		RestListenAddr: "localhost:9876",
	}
	odpFactory := &TestOdpFactory{odp: &odp}

	//nolint:revive,stylecheck // Suppress var restApiImpl should be restAPIImpl
	restApiImpl := NewRestApi(&cfg, odpFactory)

	// Missing : after username
	requestString := "{ \"username\": \"testuser\", \"application\": \"testapp\", \"tokentype\": \"testtoken\" }"

	request := httptest.NewRequest(http.MethodGet, "/odp/", strings.NewReader(requestString))
	response := httptest.NewRecorder()
	restApiImpl.ServeHTTP(response, request)

	res := response.Result()
	defer res.Body.Close()

	expected := 500
	if expected != res.StatusCode {
		t.Errorf("TestOdpRestMalformedRequest expected = %d got %d", expected, res.StatusCode)
	}

	restApiImpl.Stop()
}

func TestOdpRestTLS(t *testing.T) {
	odp := factory.OnDemandPod{
		Username:    "testuser",
		Application: "testapp",
		TokenTypes:  []string{"testtoken"},
		PodName:     "testapp-testuser",
		ResultCode:  factory.Creating,
	}

	certDir := t.TempDir()
	caCert, caKey := testcommon.CreateCertPair(t, "factoryca", true, nil, nil, certDir)
	testcommon.CreateCertPair(t, "factory", false, caCert, caKey, certDir)
	cfg := config.Config{
		RestTLSEnabled: true,
		RestCAFile:     certDir + "/factoryca/tls.crt",
		RestCertFile:   certDir + "/factory/tls.crt",
		RestKeyFile:    certDir + "/factory/tls.key",
		RestListenAddr: "localhost:9876",
	}

	odpFactory := &TestOdpFactory{odp: &odp}

	dirwatcher.Start()
	//nolint:revive,stylecheck // Suppress var restApiImpl should be restAPIImpl
	restApiImpl := NewRestApi(&cfg, odpFactory)

	// Send request where client has a trusted cert, no error expected
	testcommon.CreateCertPair(t, "trustedclient", false, caCert, caKey, certDir)
	// time.Sleep(time.Hour)
	clientRequest(t, "trustedclient", certDir, false)

	// Send request where client has a trusted cert, error expected
	otherCaCert, otherCaKey := testcommon.CreateCertPair(t, "otherca", true, nil, nil, certDir)
	testcommon.CreateCertPair(t, "untrustedclient", false, otherCaCert, otherCaKey, certDir)
	clientRequest(t, "untrustedclient", certDir, true)

	restApiImpl.Stop()
	dirwatcher.Stop()
}

func clientRequest(t *testing.T, certname string, certdir string, expectErr bool) {
	var err error

	clientCertDir := certdir + "/" + certname
	clientTLSCert, err := tls.LoadX509KeyPair(
		clientCertDir+"/tls.crt",
		clientCertDir+"/tls.key",
	)
	if err != nil {
		panic(err)
	}
	certPool, err := x509.SystemCertPool()
	if err != nil {
		panic(err)
	}
	if caCertPEM, errReadPEM := os.ReadFile(certdir + "/factoryca/tls.crt"); errReadPEM != nil {
		panic(errReadPEM)
	} else if ok := certPool.AppendCertsFromPEM(caCertPEM); !ok {
		panic("invalid cert in CA PEM")
	}
	tlsConfig := &tls.Config{
		RootCAs:      certPool,
		Certificates: []tls.Certificate{clientTLSCert},
	}
	tr := &http.Transport{
		TLSClientConfig: tlsConfig,
	}
	client := &http.Client{Transport: tr}

	requestString := "{ \"username\": \"testuser\", \"application\": \"testapp\", \"tokentypes\": [ \"testtoken\" ] }"

	t.Logf("clientRequest sending request for cert %s", certname)

	//nolint:noctx // Not requured for test
	response, err := client.Post("https://localhost:9876/odp/", "application/json", strings.NewReader(requestString))
	if response != nil {
		response.Body.Close()
	}
	if (expectErr && err == nil) || (!expectErr && err != nil) {
		t.Errorf("unexpected response to POST expectedError = %t, err=%v", expectErr, err)

		return
	}
}
