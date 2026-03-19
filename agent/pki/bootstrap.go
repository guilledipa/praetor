package pki

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"io/ioutil"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	masterpb "github.com/guilledipa/praetor/proto/gen/master"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

type BootstrapConfig struct {
	ClientCertPath    string
	ClientKeyPath     string
	MasterRootCA      string
	MasterGRPCAddress string
	NodeID            string
	BootstrapToken    string
	Logger            *slog.Logger
}

func RunBootstrapWorkflow(cfg BootstrapConfig) error {
	logger := cfg.Logger

	if _, err := os.Stat(cfg.ClientCertPath); err == nil {
		logger.Debug("Client certificate exists, skipping bootstrap")
		return nil
	}
	logger.Info("Client certificate missing. Enrolling with Master...")

	// 1. Trust Master CA
	caCert, err := os.ReadFile(cfg.MasterRootCA)
	if err != nil {
		return fmt.Errorf("failed to read master root CA: %w", err)
	}
	caCertPool := x509.NewCertPool()
	caCertPool.AppendCertsFromPEM(caCert)

	tlsConfig := &tls.Config{RootCAs: caCertPool}
	creds := credentials.NewTLS(tlsConfig)

	bootstrapAddr := strings.Split(cfg.MasterGRPCAddress, ":")[0] + ":50052"
	conn, err := grpc.Dial(bootstrapAddr, grpc.WithTransportCredentials(creds))
	if err != nil {
		return fmt.Errorf("failed to dial bootstrap server: %w", err)
	}
	defer conn.Close()

	client := masterpb.NewCertificateAuthorityClient(conn)

	// 2. Generate Key Pair
	priv, err := rsa.GenerateKey(rand.Reader, 4096)
	if err != nil {
		return err
	}

	subj := pkix.Name{CommonName: cfg.NodeID}
	template := x509.CertificateRequest{
		Subject:            subj,
		SignatureAlgorithm: x509.SHA256WithRSA,
	}
	csrBytes, err := x509.CreateCertificateRequest(rand.Reader, &template, priv)
	if err != nil {
		return err
	}

	csrPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: csrBytes})

	// 3. Send CSR
	req := &masterpb.SignCSRRequest{
		NodeId:         cfg.NodeID,
		BootstrapToken: cfg.BootstrapToken,
		CsrPem:         string(csrPEM),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	resp, err := client.SignCSR(ctx, req)
	if err != nil {
		return fmt.Errorf("failed to sign CSR via gRPC: %w", err)
	}

	// 4. Write back cert & key
	privBytes, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		return err
	}
	privPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privBytes})

	os.MkdirAll(filepath.Dir(cfg.ClientKeyPath), 0755)

	if err := os.WriteFile(cfg.ClientKeyPath, privPEM, 0600); err != nil {
		return err
	}
	if err := os.WriteFile(cfg.ClientCertPath, []byte(resp.GetCertificatePem()), 0644); err != nil {
		return err
	}

	logger.Info("Enrollment successful! Certificate provisioned locally.")
	return nil
}

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

func LoadPrivateKey(path string) (ed25519.PrivateKey, error) {
	pemBytes, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}
	block, _ := pem.Decode(pemBytes)
	if block == nil || block.Type != "PRIVATE KEY" {
		return nil, fmt.Errorf("failed to decode PEM block containing private key")
	}
	priv, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, err
	}
	return priv.(ed25519.PrivateKey), nil
}

func LoadPublicKey(path string) (ed25519.PublicKey, error) {
	pemBytes, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}
	block, _ := pem.Decode(pemBytes)
	if block == nil || block.Type != "PUBLIC KEY" {
		return nil, fmt.Errorf("failed to decode PEM block containing public key")
	}
	pub, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, err
	}
	return pub.(ed25519.PublicKey), nil
}
