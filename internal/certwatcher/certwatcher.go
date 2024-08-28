package certwatcher

import (
	"crypto/tls"
	"errors"
	"log"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"eric-odp-factory/internal/dirwatcher"
)

const (
	defaultCertWaitTimeout = 5
)

var certWaitTimeout = defaultCertWaitTimeout * time.Second

// CertWatcher the struct with the certificate path.
type CertWatcher struct {
	certMu   sync.RWMutex
	tlsCert  *tls.Certificate
	certDir  string
	certPath string
	keyPath  string
	plainDir bool
}

// NewWatcher : New instance of the certificate watcher.
func NewWatcher(certDir, certFileName, certKeyFileName string) *CertWatcher {
	watcher := &CertWatcher{
		certDir:  certDir,
		certPath: filepath.Join(certDir, certFileName),
		keyPath:  filepath.Join(certDir, certKeyFileName),
	}

	dotDotDataFile := filepath.Join(certDir, "..data")
	if _, err := os.Stat(dotDotDataFile); errors.Is(err, os.ErrNotExist) {
		watcher.plainDir = true
	} else {
		watcher.plainDir = false
	}

	slog.Info(
		"certwatcher Starting",
		"certDir",
		certDir,
		"certFileName",
		certFileName,
		"certKeyFileName",
		certKeyFileName,
		"plainDir",
		watcher.plainDir,
	)

	watcher.waitForCert()

	handlerWrapper := func(path string, op uint32) {
		watcher.dirWatchHandler(path, op)
	}
	if err := dirwatcher.WatchDirectory(certDir, handlerWrapper); err != nil {
		log.Fatalf("Failed to set dirwatcher for %s: %v", certDir, err)
	}

	return watcher
}

func fileExists(fileName string) bool {
	_, err := os.Stat(fileName)

	return err == nil
}

func (watcher *CertWatcher) waitForCert() {
	for {
		if fileExists(watcher.certPath) && fileExists(watcher.keyPath) {
			watcher.updateCert()

			return
		}
		slog.Info(
			"certwatcher Waiting for certificate files to be mounted",
			"certPath",
			watcher.certPath,
			"keyPath",
			watcher.keyPath,
		)
		time.Sleep(certWaitTimeout)
	}
}

func (watcher *CertWatcher) updateCert() {
	newCert, err := tls.LoadX509KeyPair(watcher.certPath, watcher.keyPath)
	if err != nil {
		slog.Error("certwatcher Unable to load x509 key pair", "certDir", watcher.certDir, "err", err)

		return
	}

	watcher.certMu.Lock()
	defer watcher.certMu.Unlock()
	watcher.tlsCert = &newCert
	slog.Info("certwatcher Certificate is updated", "certDir", watcher.certDir)
}

// GetCertificate Call back function for getting certificate from
// the ClientHello message.
// Used when acting as a server.
func (watcher *CertWatcher) GetCertificate(*tls.ClientHelloInfo) (*tls.Certificate, error) {
	slog.Debug("certwatcher GetCertificate called", "certDir", watcher.certDir)
	watcher.certMu.RLock()
	defer watcher.certMu.RUnlock()

	return watcher.tlsCert, nil
}

// GetClientCertificateFunc Call back function for updating client certificate.
// Use in tls.Config.GetClientCertificate when acting as the client.
func (watcher *CertWatcher) GetClientCertificate(*tls.CertificateRequestInfo) (*tls.Certificate, error) {
	slog.Debug("certwatcher GetClientCertificate called", "certDir", watcher.certDir)
	watcher.certMu.RLock()
	defer watcher.certMu.RUnlock()

	return watcher.tlsCert, nil
}

func (watcher *CertWatcher) dirWatchHandler(path string, op uint32) {
	slog.Info("dirWatchHandler", "path", path, "op", op)
	fileName := filepath.Base(path)
	if !watcher.plainDir && fileName == "..data" && ((op & dirwatcher.Create) == dirwatcher.Create) {
		watcher.updateCert()
	} else if watcher.plainDir && path == watcher.certPath &&
		((op & (dirwatcher.Create | dirwatcher.Write)) != 0) {
		watcher.updateCert()
	}
}
