package certwatcher

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	"eric-odp-factory/internal/dirwatcher"
)

func genCertPair(t *testing.T, certDir, cn string) {
	now := time.Now()
	cert := &x509.Certificate{
		SerialNumber: big.NewInt(now.Unix()),
		Subject: pkix.Name{
			CommonName:         cn,
			Country:            []string{"SE"},
			Organization:       []string{"Ericsson"},
			OrganizationalUnit: []string{"OSS"},
		},
		NotBefore:             now,
		NotAfter:              now.AddDate(0, 0, 1), // Valid for one day
		BasicConstraintsValid: true,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		KeyUsage:              x509.KeyUsageDigitalSignature,
	}

	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		panic(err)
	}

	certBytes, err := x509.CreateCertificate(
		rand.Reader,
		cert,
		cert,
		privateKey.Public(),
		privateKey,
	)
	if err != nil {
		panic(err)
	}

	certPEM := new(bytes.Buffer)
	pem.Encode(certPEM, &pem.Block{
		Type:  "CERTIFICATE",
		Bytes: certBytes,
	})

	certPrivKeyPEM := new(bytes.Buffer)
	pem.Encode(certPrivKeyPEM, &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(privateKey),
	})

	thisCertDir := certDir + "/" + cn
	if mkdirErr := os.Mkdir(thisCertDir, 0o777); mkdirErr != nil {
		t.Errorf("Cannot create thisCertDir %s: %v", thisCertDir, mkdirErr)
		time.Sleep(time.Hour)
	}

	err = os.WriteFile(thisCertDir+"/tls.crt", certPEM.Bytes(), 0o600)
	if err != nil {
		t.Errorf("Cannot create cert %v", err)
		time.Sleep(time.Hour)
	}
	err = os.WriteFile(thisCertDir+"/tls.key", certPrivKeyPEM.Bytes(), 0o600)
	if err != nil {
		panic(err)
	}
}

func setupV1(certDir string, t *testing.T) {
	genCertPair(t, certDir, "v1")
	dotDotData := filepath.Join(certDir, "..data")
	if err := os.Symlink(filepath.Join(certDir, "v1"), dotDotData); err != nil {
		t.Logf("Failed to create symlink for v1/..data: %v", err)
	}
	if err := os.Symlink(filepath.Join(dotDotData, "tls.crt"), filepath.Join(certDir, "tls.crt")); err != nil {
		t.Logf("Failed to create symlink for tls.crt: %v", err)
	}
	if err := os.Symlink(filepath.Join(dotDotData, "tls.key"), filepath.Join(certDir, "tls.key")); err != nil {
		t.Logf("Failed to create symlink for tls.key: %v", err)
	}
}

func TestCertWatcher(t *testing.T) {
	dirwatcher.Start()

	certDir := t.TempDir()

	setupV1(certDir, t)

	watcher := NewWatcher(certDir, "tls.crt", "tls.key")
	cert, _ := watcher.GetCertificate(nil)
	v1X509, _ := x509.ParseCertificate(cert.Certificate[0])

	// Now create a new cert
	genCertPair(t, certDir, "v2")
	// Switch ..data to point at new link
	os.Remove(filepath.Join(certDir, "..data"))
	if err := os.Symlink(filepath.Join(certDir, "v2"), filepath.Join(certDir, "..data")); err != nil {
		t.Logf("Failed to create symlink for v2/..data: %v", err)
	}

	certUpdated := false
	for i := 0; i < 5 && !certUpdated; i++ {
		time.Sleep(time.Millisecond)
		cert, _ := watcher.GetCertificate(nil)
		currX509, _ := x509.ParseCertificate(cert.Certificate[0])
		certUpdated = !reflect.DeepEqual(v1X509, currX509.Subject)
	}
	if !certUpdated {
		t.Errorf("Cert not updated")
	}

	dirwatcher.Stop()
}

func TestCertWatcherNotReady(t *testing.T) {
	dirwatcher.Start()

	certDir := t.TempDir()

	// default wait timeout is 5 seconds, don't want to
	// wait that long in unit test
	certWaitTimeout = 50 * time.Millisecond

	var newWatcherReturned atomic.Bool
	newWatcherReturned.Store(false)

	go func() {
		NewWatcher(certDir, "tls.crt", "tls.key")
		newWatcherReturned.Store(true)
	}()

	time.Sleep(250 * time.Millisecond)

	if newWatcherReturned.Load() {
		t.Errorf("Expected NewWatcher to still be blocked")
	}

	t.Log("Writing cert now")
	setupV1(certDir, t)

	for i := 0; i < 5 && !newWatcherReturned.Load(); i++ {
		time.Sleep(50 * time.Millisecond)
	}
	if !newWatcherReturned.Load() {
		t.Errorf("Expected NewWatcher to have returned")
	}

	dirwatcher.Stop()
}
