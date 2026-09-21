package app

import (
	"crypto/tls"
	"crypto/x509"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeCertPair writes the embedded development pair into a temp directory and
// returns both paths. It is a real, loadable pair, which is all these tests need.
func writeCertPair(t *testing.T) (certFile, keyFile string) {
	t.Helper()

	dir := t.TempDir()
	certFile = filepath.Join(dir, "cert.pem")
	keyFile = filepath.Join(dir, "key.pem")

	if err := os.WriteFile(certFile, embeddedCertFile, 0600); err != nil {
		t.Fatalf("write cert: %v", err)
	}
	if err := os.WriteFile(keyFile, embeddedKeyFile, 0600); err != nil {
		t.Fatalf("write key: %v", err)
	}
	return certFile, keyFile
}

func TestLoadTLSCertFromFiles(t *testing.T) {
	certFile, keyFile := writeCertPair(t)

	for _, devEnv := range []bool{true, false} {
		cert, err := loadTLSCert(certFile, keyFile, devEnv)
		if err != nil {
			t.Fatalf("loadTLSCert(devEnv=%v): %v", devEnv, err)
		}
		if len(cert.Certificate) == 0 {
			t.Errorf("devEnv=%v: no certificate was loaded", devEnv)
		}
	}
}

func TestLoadTLSCertProdRefusesTheEmbeddedPair(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "nope.pem")

	tests := []struct {
		name     string
		certFile string
		keyFile  string
	}{
		{"both missing", missing, missing},
		{"both empty", "", ""},
		{"key missing", "", missing},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := loadTLSCert(tt.certFile, tt.keyFile, false)
			if err == nil {
				t.Fatal("production started with the embedded development certificate")
			}
			if !strings.Contains(err.Error(), DevEnv) {
				t.Errorf("error %q does not explain that the embedded pair is dev only", err)
			}
		})
	}
}

func TestLoadTLSCertProdRefusesAHalfPair(t *testing.T) {
	// The certificate exists, the key does not - the old code loaded the pair
	// anyway and failed somewhere deeper.
	certFile, _ := writeCertPair(t)
	missingKey := filepath.Join(t.TempDir(), "key.pem")

	if _, err := loadTLSCert(certFile, missingKey, false); err == nil {
		t.Error("a certificate without its key was accepted in production")
	}
}

func TestLoadTLSCertDevFallsBackToTheEmbeddedPair(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "nope.pem")

	cert, err := loadTLSCert(missing, missing, true)
	if err != nil {
		t.Fatalf("loadTLSCert: %v", err)
	}

	embedded, err := tls.X509KeyPair(embeddedCertFile, embeddedKeyFile)
	if err != nil {
		t.Fatalf("the embedded pair itself does not load: %v", err)
	}
	if len(cert.Certificate) == 0 || string(cert.Certificate[0]) != string(embedded.Certificate[0]) {
		t.Error("the fallback did not return the embedded certificate")
	}
}

func TestEmbeddedCertificateIsForLocalhostOnly(t *testing.T) {
	// Guards the claim in the README: the embedded pair is good for localhost
	// development and nothing else.
	cert, err := tls.X509KeyPair(embeddedCertFile, embeddedKeyFile)
	if err != nil {
		t.Fatalf("X509KeyPair: %v", err)
	}

	parsed, err := x509.ParseCertificate(cert.Certificate[0])
	if err != nil {
		t.Fatalf("ParseCertificate: %v", err)
	}

	if parsed.Subject.CommonName != "localhost" {
		t.Errorf("common name = %q, want localhost", parsed.Subject.CommonName)
	}
}
