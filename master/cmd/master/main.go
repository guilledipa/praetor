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
	"net/http"
	"os"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/guilledipa/praetor/pkg/pki"
	"github.com/guilledipa/praetor/master/broker"
	"github.com/guilledipa/praetor/master/classifier"
	"github.com/guilledipa/praetor/master/server"
	"github.com/guilledipa/praetor/pkg/secrets"
	"github.com/guilledipa/praetor/pkg/secrets/local"
	"github.com/guilledipa/praetor/pkg/secrets/vault"
	"github.com/guilledipa/praetor/pkg/storage"
	"github.com/guilledipa/praetor/pkg/storage/natsjs"
	"github.com/guilledipa/praetor/pkg/telemetry"
	natsgo "github.com/nats-io/nats.go"
	pb "github.com/guilledipa/praetor/proto/gen/master"
	"github.com/spf13/viper"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/reflection"
)

type MasterConfig struct {
	NatsURL         string        `mapstructure:"nats_url"`
	NatsClientCert  string        `mapstructure:"nats_client_cert"`
	NatsClientKey   string        `mapstructure:"nats_client_key"`
	NatsRootCA      string        `mapstructure:"nats_root_ca"`
	TriggerInterval time.Duration `mapstructure:"trigger_interval"`
	TargetNodeID    string        `mapstructure:"target_node_id"`
	BootstrapToken  string        `mapstructure:"bootstrap_token"`
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

func setupStorage(cfg MasterConfig) (storage.Provider, *natsgo.Conn, error) {
	cert, err := tls.LoadX509KeyPair(cfg.NatsClientCert, cfg.NatsClientKey)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to load client key pair: %w", err)
	}

	caCert, err := os.ReadFile(cfg.NatsRootCA)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read root CA file: %w", err)
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
		return nil, nil, fmt.Errorf("storage nats connection failed: %w", err)
	}

	provider, err := natsjs.NewProvider(context.Background(), nc)
	if err != nil {
		return nil, nil, err
	}
	return provider, nc, nil
}

func main() {
	logLevel := slog.LevelInfo
	if os.Getenv("LOG_LEVEL") == "debug" {
		logLevel = slog.LevelDebug
	}
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: logLevel}))
	slog.SetDefault(logger)

	logger.Info("Master server starting...")

	tp, err := telemetry.InitProvider(context.Background(), logger, "praetor-master")
	if err != nil {
		logger.Error("Failed to initialize OpenTelemetry", "error", err)
	} else if tp != nil {
		defer func() {
			if err := tp.Shutdown(context.Background()); err != nil {
				logger.Error("Error shutting down tracer provider", "error", err)
			}
		}()
	}

	if err := pki.AutoProvisionPKI(logger); err != nil {
		logger.Error("Failed to auto-provision TLS PKI", "error", err)
		os.Exit(1)
	}

	viper.SetEnvPrefix("PRAETOR_MASTER")
	viper.AutomaticEnv()
	viper.SetConfigFile("/etc/praetor/master.yaml")

	if err := viper.ReadInConfig(); err != nil {
		logger.Warn("Failed to read config file, falling back to env vars", "error", err)
	}

	viper.SetDefault("nats_url", "nats://localhost:4222")
	viper.SetDefault("nats_client_cert", "../nats/certs/client.crt")
	viper.SetDefault("nats_client_key", "../nats/certs/client.key")
	viper.SetDefault("nats_root_ca", "../nats/certs/ca.crt")
	viper.SetDefault("trigger_interval", "15s")
	viper.SetDefault("target_node_id", "agent1")
	viper.SetDefault("bootstrap_token", "praetor-secret-token")
	viper.SetDefault("secrets_backend", "local") // 'local' or 'vault'

	var cfg MasterConfig
	if err := viper.Unmarshal(&cfg); err != nil {
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
	bootServer := grpc.NewServer(grpc.Creds(bootCreds), grpc.StatsHandler(otelgrpc.NewServerHandler()))
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

	s := grpc.NewServer(grpc.Creds(creds), grpc.StatsHandler(otelgrpc.NewServerHandler()))
	var secretProv secrets.Provider
	var secretsErr error
	if viper.GetString("secrets_backend") == "vault" {
		logger.Info("Initializing Vault Secrets Provider")
		secretProv, secretsErr = vault.NewProvider()
	} else {
		logger.Info("Initializing Local File Secrets Provider (secrets.yaml)")
		secretProv, secretsErr = local.NewProvider("secrets.yaml")
	}

	if secretsErr != nil {
		logger.Warn("Failed to initialize selected secrets provider", "error", secretsErr)
	}

	storageProv, ncStorage, err := setupStorage(cfg)
	if err != nil {
		logger.Error("Failed to bootstrap JS Storage persistence mapping natively", "error", err)
		os.Exit(1)
	}

	cls := classifier.NewFileClassifier("classification.yaml", "roles")
	pb.RegisterConfigurationMasterServer(s, server.NewServer(signingKey, secretProv, storageProv, cls, logger))
	reflection.Register(s)

	// Standup Operator Server
	opServer := grpc.NewServer(grpc.Creds(creds), grpc.StatsHandler(otelgrpc.NewServerHandler()))
	pb.RegisterOperatorServer(opServer, server.NewOperatorServer(storageProv, ncStorage, logger))
	reflection.Register(opServer)

	opLis, err := net.Listen("tcp", ":50053")
	if err != nil {
		logger.Error("failed to listen on operator port", "error", err)
		os.Exit(1)
	}
	go func() {
		logger.Info("Operator API listening on gRPC", "port", ":50053", "tls", "mTLS")
		if err := opServer.Serve(opLis); err != nil {
			logger.Error("failed to serve operator gRPC", "error", err)
		}
	}()

	lis, err := net.Listen("tcp", ":50051")
	if err != nil {
		logger.Error("failed to listen", "error", err)
		os.Exit(1)
	}

	go func() {
		http.Handle("/metrics", promhttp.Handler())
		logger.Info("Metrics server listening on", "port", ":8080")
		if err := http.ListenAndServe(":8080", nil); err != nil {
			logger.Error("Metrics server failed", "error", err)
		}
	}()

	logger.Info("Master server listening on gRPC", "port", ":50051", "tls", "mTLS")
	if err := s.Serve(lis); err != nil {
		logger.Error("failed to serve gRPC", "error", err)
		os.Exit(1)
	}
}