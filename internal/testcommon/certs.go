package testcommon

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"os"
	"testing"
	"time"
)

func CreateCertPair(
	t *testing.T,
	cn string,
	isCA bool,
	caCert *x509.Certificate,
	caKey *rsa.PrivateKey,
	certDir string,
) (*x509.Certificate, *rsa.PrivateKey) {
	var keyUsage x509.KeyUsage
	var extKeyUsage []x509.ExtKeyUsage

	if isCA {
		keyUsage = x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign
		extKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth}
	} else {
		keyUsage = x509.KeyUsageDigitalSignature
		extKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth}
	}

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
		IsCA:                  isCA,
		ExtKeyUsage:           extKeyUsage,
		KeyUsage:              keyUsage,
	}

	if caCert != nil {
		cert.IPAddresses = []net.IP{net.IPv4(127, 0, 0, 1), net.IPv6loopback} //nolint:gomnd // Test code
		cert.DNSNames = append(cert.DNSNames, "localhost")
	}

	privateKey, err := rsa.GenerateKey(rand.Reader, 2048) //nolint:gomnd // Test code
	if err != nil {
		panic(err)
	}

	var parentCert *x509.Certificate
	var parentKey *rsa.PrivateKey
	if caCert == nil {
		parentCert = cert
		parentKey = privateKey
	} else {
		parentCert = caCert
		parentKey = caKey
	}

	certBytes, err := x509.CreateCertificate(
		rand.Reader,
		cert,
		parentCert,
		privateKey.Public(),
		parentKey,
	)
	if err != nil {
		panic(err)
	}

	certPEM := new(bytes.Buffer)
	pem.Encode(certPEM, &pem.Block{ //nolint:errcheck // Test code
		Type:  "CERTIFICATE",
		Bytes: certBytes,
	})

	certPrivKeyPEM := new(bytes.Buffer)
	pem.Encode(certPrivKeyPEM, &pem.Block{ //nolint:errcheck // Test code
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(privateKey),
	})

	thisCertDir := certDir + "/" + cn
	if mkdirErr := os.Mkdir(thisCertDir, 0o777); mkdirErr != nil { //nolint:gomnd // Test code
		t.Errorf("Cannot create thisCertDir %s: %v", thisCertDir, mkdirErr)
		time.Sleep(time.Hour)
	}

	err = os.WriteFile(thisCertDir+"/tls.crt", certPEM.Bytes(), 0o600) //nolint:gomnd // Test code
	if err != nil {
		t.Errorf("Cannot create cert %v", err)
		time.Sleep(time.Hour)
	}
	err = os.WriteFile(thisCertDir+"/tls.key", certPrivKeyPEM.Bytes(), 0o600) //nolint:gomnd // Test code
	if err != nil {
		panic(err)
	}

	return cert, privateKey
}
