package main

import (
	"context"
	"crypto/ed25519"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"io/ioutil"
	"log/slog"
	"net"
	"os"
	"time"

	"github.com/guilledipa/praetor/master/broker"
	"github.com/guilledipa/praetor/master/classifier"
	"github.com/guilledipa/praetor/master/server"
	"github.com/guilledipa/praetor/pkg/secrets/local"
	"github.com/guilledipa/praetor/pkg/storage"
	"github.com/guilledipa/praetor/pkg/storage/natsjs"
	natsgo "github.com/nats-io/nats.go"
	pb "github.com/guilledipa/praetor/proto/gen/master"
	"github.com/kelseyhightower/envconfig"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/reflection"
)

type MasterConfig struct {
	NatsURL         string        `envconfig:"NATS_URL" default:"nats://localhost:4222"`
	NatsClientCert  string        `envconfig:"NATS_CLIENT_CERT" default:"../nats/certs/client.crt"`
	NatsClientKey   string        `envconfig:"NATS_CLIENT_KEY" default:"../nats/certs/client.key"`
	NatsRootCA      string        `envconfig:"NATS_ROOT_CA" default:"../nats/certs/ca.crt"`
	TriggerInterval time.Duration `envconfig:"TRIGGER_INTERVAL" default:"15s"`
	TargetNodeID    string        `envconfig:"TARGET_NODE_ID" default:"agent1"`
	BootstrapToken  string        `envconfig:"BOOTSTRAP_TOKEN" default:"praetor-secret-token"`
}

func loadPrivateKey(path string) (ed25519.PrivateKey, error) {
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

func setupStorage(cfg MasterConfig) (storage.Provider, error) {
	cert, err := tls.LoadX509KeyPair(cfg.NatsClientCert, cfg.NatsClientKey)
	if err != nil {
		return nil, fmt.Errorf("failed to load client key pair: %w", err)
	}

	caCert, err := os.ReadFile(cfg.NatsRootCA)
	if err != nil {
		return nil, fmt.Errorf("failed to read root CA file: %w", err)
	}
	caCertPool := x509.NewCertPool()
	caCertPool.AppendCertsFromPEM(caCert)

	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
		RootCAs:      caCertPool,
		MinVersion:   tls.VersionTLS12,
	}

	nc, err := natsgo.Connect(cfg.NatsURL, natsgo.Secure(tlsConfig))
	if err != nil {
		return nil, fmt.Errorf("storage nats connection failed: %w", err)
	}

	return natsjs.NewProvider(context.Background(), nc)
}

func main() {
	logLevel := slog.LevelInfo
	if os.Getenv("LOG_LEVEL") == "debug" {
		logLevel = slog.LevelDebug
	}
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: logLevel}))
	slog.SetDefault(logger)

	logger.Info("Master server starting...")

	var cfg MasterConfig
	if err := envconfig.Process("MASTER", &cfg); err != nil {
		logger.Error("Failed to process master config", "error", err)
		os.Exit(1)
	}

	b := broker.NewNatsBroadcaster(broker.Config{
		NatsURL:         cfg.NatsURL,
		NatsClientCert:  cfg.NatsClientCert,
		NatsClientKey:   cfg.NatsClientKey,
		NatsRootCA:      cfg.NatsRootCA,
		TriggerInterval: cfg.TriggerInterval,
		TargetNodeID:    cfg.TargetNodeID,
	}, logger)

	br, err := b.SetupBroker()
	if err != nil {
		logger.Error("Failed to setup message broker", "error", err)
		os.Exit(1)
	}
	defer br.Close()
	logger.Info("Connected to message broker successfully")

	go b.StartTriggerPublisher(br)

	serverCert, err := tls.LoadX509KeyPair("certs/server.crt", "certs/server.key")
	if err != nil {
		logger.Error("failed to load server certificate and key", "error", err)
		os.Exit(1)
	}

	caTLSCert, err := tls.LoadX509KeyPair("certs/ca.crt", "certs/ca.key")
	if err != nil {
		logger.Error("failed to load CA definition for signing", "error", err)
		os.Exit(1)
	}
	caX509Cert, err := x509.ParseCertificate(caTLSCert.Certificate[0])
	if err != nil {
		logger.Error("failed to parse CA x509 cert")
		os.Exit(1)
	}

	bootCreds := credentials.NewServerTLSFromCert(&serverCert)
	bootServer := grpc.NewServer(grpc.Creds(bootCreds))
	pb.RegisterCertificateAuthorityServer(bootServer, server.NewCAServer(caX509Cert, caTLSCert.PrivateKey, cfg.BootstrapToken, logger))

	bootLis, err := net.Listen("tcp", ":50052")
	if err != nil {
		logger.Error("failed to listen bootstrap", "error", err)
		os.Exit(1)
	}
	go func() {
		logger.Info("Bootstrap server listening on gRPC", "port", ":50052")
		if err := bootServer.Serve(bootLis); err != nil {
			logger.Error("failed to serve bootstrap gRPC", "error", err)
		}
	}()

	caCert, err := os.ReadFile("certs/ca.crt")
	if err != nil {
		logger.Error("failed to read CA certificate", "error", err)
		os.Exit(1)
	}

	caCertPool := x509.NewCertPool()
	if !caCertPool.AppendCertsFromPEM(caCert) {
		logger.Error("failed to append CA certificate")
		os.Exit(1)
	}

	signingKey, err := loadPrivateKey("certs/master_signing.key")
	if err != nil {
		logger.Error("failed to load master signing key", "error", err)
		os.Exit(1)
	}

	grpcTLSConfig := &tls.Config{
		Certificates: []tls.Certificate{serverCert},
		ClientAuth:   tls.RequireAndVerifyClientCert,
		ClientCAs:    caCertPool,
	}
	creds := credentials.NewTLS(grpcTLSConfig)

	s := grpc.NewServer(grpc.Creds(creds))
	secretProv, err := local.NewProvider("secrets.yaml")
	if err != nil {
		logger.Warn("Failed to load secrets.yaml", "error", err)
	}

	storageProv, err := setupStorage(cfg)
	if err != nil {
		logger.Error("Failed to bootstrap JS Storage persistence mapping natively", "error", err)
		os.Exit(1)
	}

	cls := classifier.NewFileClassifier("classification.yaml", "roles")
	pb.RegisterConfigurationMasterServer(s, server.NewServer(signingKey, secretProv, storageProv, cls, logger))
	reflection.Register(s)

	lis, err := net.Listen("tcp", ":50051")
	if err != nil {
		logger.Error("failed to listen", "error", err)
		os.Exit(1)
	}

	logger.Info("Master server listening on gRPC", "port", ":50051", "tls", "mTLS")
	if err := s.Serve(lis); err != nil {
		logger.Error("failed to serve gRPC", "error", err)
		os.Exit(1)
	}
}