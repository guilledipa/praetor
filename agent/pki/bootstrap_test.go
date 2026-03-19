package pki

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func generateTestCertAndKey(t *testing.T, dir string, prefix string) (string, string) {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("Failed to generate RSA key: %v", err)
	}

	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serialNumber, _ := rand.Int(rand.Reader, serialNumberLimit)

	template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject:      pkix.Name{CommonName: "Test Cert"},
		NotBefore:    time.Now(),
		NotAfter:     time.Now().Add(1 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}

	certBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	if err != nil {
		t.Fatalf("Failed to create certificate: %v", err)
	}

	certPath := filepath.Join(dir, prefix+".crt")
	keyPath := filepath.Join(dir, prefix+".key")

	certFile, _ := os.Create(certPath)
	defer certFile.Close()
	pem.Encode(certFile, &pem.Block{Type: "CERTIFICATE", Bytes: certBytes})

	keyFile, _ := os.Create(keyPath)
	defer keyFile.Close()
	privBytes, _ := x509.MarshalPKCS8PrivateKey(priv)
	pem.Encode(keyFile, &pem.Block{Type: "PRIVATE KEY", Bytes: privBytes})

	return certPath, keyPath
}

func generateTestEd25519(t *testing.T, dir string, prefix string) (string, string) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("Failed to generate ed25519 key: %v", err)
	}

	pubBytes, _ := x509.MarshalPKIXPublicKey(pub)
	privBytes, _ := x509.MarshalPKCS8PrivateKey(priv)

	pubPath := filepath.Join(dir, prefix+".pub")
	privPath := filepath.Join(dir, prefix+".key")

	pubFile, _ := os.Create(pubPath)
	defer pubFile.Close()
	pem.Encode(pubFile, &pem.Block{Type: "PUBLIC KEY", Bytes: pubBytes})

	privFile, _ := os.Create(privPath)
	defer privFile.Close()
	pem.Encode(privFile, &pem.Block{Type: "PRIVATE KEY", Bytes: privBytes})

	return pubPath, privPath
}

func TestSetupTLS(t *testing.T) {
	dir := t.TempDir()
	certPath, keyPath := generateTestCertAndKey(t, dir, "client")
	caPath, _ := generateTestCertAndKey(t, dir, "ca")

	tlsConfig, err := SetupTLS(certPath, keyPath, caPath)
	if err != nil {
		t.Fatalf("SetupTLS failed: %v", err)
	}

	if len(tlsConfig.Certificates) == 0 {
		t.Error("Expected certificates in TLS config")
	}
	if tlsConfig.RootCAs == nil {
		t.Error("Expected RootCAs in TLS config")
	}
}

func TestLoadKeys(t *testing.T) {
	dir := t.TempDir()
	pubPath, privPath := generateTestEd25519(t, dir, "master_signing")

	_, err := LoadPrivateKey(privPath)
	if err != nil {
		t.Fatalf("LoadPrivateKey failed: %v", err)
	}

	_, err = LoadPublicKey(pubPath)
	if err != nil {
		t.Fatalf("LoadPublicKey failed: %v", err)
	}
}
