package server

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"log/slog"
	"math/big"
	"os"
	"testing"
	"time"

	pb "github.com/guilledipa/praetor/proto/gen/master"
)

func generateTestCA(t *testing.T) (*x509.Certificate, *rsa.PrivateKey) {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("Failed to generate RSA key: %v", err)
	}

	template := x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "Test CA"},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(1 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}

	certBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	if err != nil {
		t.Fatalf("Failed to create certificate: %v", err)
	}

	cert, _ := x509.ParseCertificate(certBytes)
	return cert, priv
}

func TestSignCSR(t *testing.T) {
	caCert, caPriv := generateTestCA(t)
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	caServer := NewCAServer(caCert, caPriv, "test-token", logger)

	priv, _ := rsa.GenerateKey(rand.Reader, 2048)
	template := x509.CertificateRequest{
		Subject:            pkix.Name{CommonName: "node-123"},
		SignatureAlgorithm: x509.SHA256WithRSA,
	}
	csrBytes, _ := x509.CreateCertificateRequest(rand.Reader, &template, priv)
	csrPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: csrBytes})

	req := &pb.SignCSRRequest{
		NodeId:         "node-123",
		BootstrapToken: "test-token",
		CsrPem:         string(csrPEM),
	}

	resp, err := caServer.SignCSR(context.Background(), req)
	if err != nil {
		t.Fatalf("SignCSR failed: %v", err)
	}

	if resp.CertificatePem == "" {
		t.Fatalf("Expected certificate PEM, got empty string")
	}

	req.BootstrapToken = "invalid-token"
	_, err = caServer.SignCSR(context.Background(), req)
	if err == nil {
		t.Fatalf("Expected error with invalid token")
	}
}
