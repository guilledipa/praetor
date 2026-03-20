package pki

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"log/slog"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"time"
)

// AutoProvisionPKI checks if certificates exist in their respective directories,
// and if not, generates a complete cluster PKI mirroring the legacy shell scripts.
func AutoProvisionPKI(logger *slog.Logger) error {
	natsDir := "nats/certs"
	masterDir := "master/certs"
	agentDir := "agent/certs"

	if err := os.MkdirAll(natsDir, 0755); err != nil {
		return err
	}
	if err := os.MkdirAll(masterDir, 0755); err != nil {
		return err
	}
	if err := os.MkdirAll(agentDir, 0755); err != nil {
		return err
	}

	// Fast check: If the CA exists, we assume PKI is provisioned.
	if _, err := os.Stat(filepath.Join(masterDir, "ca.crt")); err == nil {
		logger.Info("PKI already provisioned on disk. Skipping Auto-TLS.")
		return nil
	}

	logger.Info("No Certificates found. Executing Auto-TLS PKI Provisioning...")

	// 1. Generate Unified CA
	caPrivKey, err := rsa.GenerateKey(rand.Reader, 4096)
	if err != nil {
		return fmt.Errorf("generate CA key: %w", err)
	}

	caTemplate := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			CommonName: "PraetorUnifiedCA",
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().AddDate(10, 0, 0),
		IsCA:                  true,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
	}

	caDerBytes, err := x509.CreateCertificate(rand.Reader, &caTemplate, &caTemplate, &caPrivKey.PublicKey, caPrivKey)
	if err != nil {
		return fmt.Errorf("create CA cert: %w", err)
	}
	if err := writeCert(filepath.Join(masterDir, "ca.crt"), caDerBytes); err != nil {
		return err
	}
	if err := writeKey(filepath.Join(masterDir, "ca.key"), caPrivKey); err != nil {
		return err
	}
	
	// NATS CA Copy
	if err := writeCert(filepath.Join(natsDir, "ca.crt"), caDerBytes); err != nil {
		return err
	}
	// Agent CA Copy
	if err := writeCert(filepath.Join(agentDir, "master-ca.crt"), caDerBytes); err != nil {
		return err
	}

	// 2. Generate NATS Server Cert
	if err := generateSignedCert(
		"localhost", []string{"localhost"}, []net.IP{net.ParseIP("127.0.0.1")},
		caTemplate, caPrivKey,
		filepath.Join(natsDir, "server.crt"), filepath.Join(natsDir, "server.key"),
	); err != nil {
		return fmt.Errorf("nats server cert: %w", err)
	}

	// 3. Generate NATS Client Cert (for Master)
	if err := generateSignedCert(
		"PraetorNATSClient", nil, nil,
		caTemplate, caPrivKey,
		filepath.Join(natsDir, "client.crt"), filepath.Join(natsDir, "client.key"),
	); err != nil {
		return fmt.Errorf("nats client cert: %w", err)
	}

	// 4. Generate Master Server Cert
	if err := generateSignedCert(
		"localhost", []string{"localhost"}, []net.IP{net.ParseIP("127.0.0.1")},
		caTemplate, caPrivKey,
		filepath.Join(masterDir, "server.crt"), filepath.Join(masterDir, "server.key"),
	); err != nil {
		return fmt.Errorf("master server cert: %w", err)
	}

	// 5. Generate Agent Client Cert (Pre-provisioned)
	if err := generateSignedCert(
		"PraetorPreprovisionedAgent", nil, nil,
		caTemplate, caPrivKey,
		filepath.Join(agentDir, "client.crt"), filepath.Join(agentDir, "client.key"),
	); err != nil {
		return fmt.Errorf("agent client cert: %w", err)
	}

	// 6. Generate Master Signing Ed25519 Keys
	pubKey, privKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return fmt.Errorf("master ed25519 key: %w", err)
	}
	
	privBytes, err := x509.MarshalPKCS8PrivateKey(privKey)
	if err != nil {
		return err
	}
	
	privFile, err := os.Create(filepath.Join(masterDir, "master_signing.key"))
	if err != nil {
		return err
	}
	defer privFile.Close()
	pem.Encode(privFile, &pem.Block{Type: "PRIVATE KEY", Bytes: privBytes})

	pubBytes, err := x509.MarshalPKIXPublicKey(pubKey)
	if err != nil {
		return err
	}
	pubFile, err := os.Create(filepath.Join(masterDir, "master_signing.pub"))
	if err != nil {
		return err
	}
	defer pubFile.Close()
	pem.Encode(pubFile, &pem.Block{Type: "PUBLIC KEY", Bytes: pubBytes})

	logger.Info("✅ Auto-TLS PKI Provisioning Complete.")
	return nil
}

func generateSignedCert(cn string, dnsNames []string, ips []net.IP, caTemplate x509.Certificate, caPrivKey *rsa.PrivateKey, certPath, keyPath string) error {
	privKey, err := rsa.GenerateKey(rand.Reader, 4096)
	if err != nil {
		return err
	}

	template := x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()),
		Subject: pkix.Name{
			CommonName: cn,
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().AddDate(10, 0, 0),
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		DNSNames:              dnsNames,
		IPAddresses:           ips,
		BasicConstraintsValid: true,
	}

	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &caTemplate, &privKey.PublicKey, caPrivKey)
	if err != nil {
		return err
	}

	if err := writeCert(certPath, derBytes); err != nil {
		return err
	}
	if err := writeKey(keyPath, privKey); err != nil {
		return err
	}
	return nil
}

func writeCert(path string, derBytes []byte) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return pem.Encode(f, &pem.Block{Type: "CERTIFICATE", Bytes: derBytes})
}

func writeKey(path string, privKey *rsa.PrivateKey) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return pem.Encode(f, &pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(privKey)})
}

// SetupTLS is a generic helper to load certificates from disk
func SetupTLS(clientCert, clientKey, rootCA string) (*tls.Config, error) {
	cert, err := tls.LoadX509KeyPair(clientCert, clientKey)
	if err != nil {
		return nil, fmt.Errorf("failed to load client key pair: %w", err)
	}

	caCert, err := os.ReadFile(rootCA)
	if err != nil {
		return nil, fmt.Errorf("failed to read root CA file: %w", err)
	}
	caCertPool := x509.NewCertPool()
	caCertPool.AppendCertsFromPEM(caCert)

	return &tls.Config{
		Certificates: []tls.Certificate{cert},
		RootCAs:      caCertPool,
		MinVersion:   tls.VersionTLS12,
	}, nil
}
