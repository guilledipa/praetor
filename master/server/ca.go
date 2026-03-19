package server

import (
	"context"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"log/slog"
	"math/big"
	"time"

	pb "github.com/guilledipa/praetor/proto/gen/master"
)

type CAServer struct {
	pb.UnimplementedCertificateAuthorityServer
	CaCert         *x509.Certificate
	CaPrivKey      interface{}
	BootstrapToken string
	Logger         *slog.Logger
}

func NewCAServer(caCert *x509.Certificate, caPrivKey interface{}, token string, logger *slog.Logger) *CAServer {
	return &CAServer{
		CaCert:         caCert,
		CaPrivKey:      caPrivKey,
		BootstrapToken: token,
		Logger:         logger,
	}
}

func (s *CAServer) SignCSR(ctx context.Context, in *pb.SignCSRRequest) (*pb.SignCSRResponse, error) {
	if s.BootstrapToken != "" && in.GetBootstrapToken() != s.BootstrapToken {
		return nil, fmt.Errorf("invalid bootstrap token")
	}

	s.Logger.Info("Received CSR signing request", "node_id", in.GetNodeId())

	// Parse CSR
	block, _ := pem.Decode([]byte(in.GetCsrPem()))
	if block == nil || block.Type != "CERTIFICATE REQUEST" {
		return nil, fmt.Errorf("failed to decode PEM block containing CSR")
	}

	csr, err := x509.ParseCertificateRequest(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse CSR: %w", err)
	}
	if err := csr.CheckSignature(); err != nil {
		return nil, fmt.Errorf("invalid CSR signature: %w", err)
	}

	// Create Certificate
	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serialNumber, err := rand.Int(rand.Reader, serialNumberLimit)
	if err != nil {
		return nil, fmt.Errorf("failed to generate serial number: %w", err)
	}

	template := x509.Certificate{
		SerialNumber:          serialNumber,
		Subject:               csr.Subject,
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(365 * 24 * time.Hour), // 1 year
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
	}

	certBytes, err := x509.CreateCertificate(rand.Reader, &template, s.CaCert, csr.PublicKey, s.CaPrivKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create certificate: %w", err)
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certBytes})

	s.Logger.Info("Successfully signed CSR", "node_id", in.GetNodeId(), "serial", serialNumber.String())

	return &pb.SignCSRResponse{
		CertificatePem: string(certPEM),
	}, nil
}
