package main

import (
	"context"
	"crypto/ed25519"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io/ioutil"
	"log/slog"
	"net"
	"os"
	"time"

	"github.com/guilledipa/praetor/master/catalog"
	pb "github.com/guilledipa/praetor/proto/gen/master"
	"github.com/kelseyhightower/envconfig"
	"github.com/nats-io/nats.go"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/reflection"
	"gopkg.in/yaml.v3"
)

const (
	port = ":50051"
)

var logger *slog.Logger

type MasterConfig struct {
	NatsURL        string        `envconfig:"NATS_URL" default:"nats://localhost:4222"`
	NatsClientCert string        `envconfig:"NATS_CLIENT_CERT" default:"../nats/certs/client.crt"`
	NatsClientKey  string        `envconfig:"NATS_CLIENT_KEY" default:"../nats/certs/client.key"`
	NatsRootCA     string        `envconfig:"NATS_ROOT_CA" default:"../nats/certs/ca.crt"`
	TriggerInterval time.Duration `envconfig:"TRIGGER_INTERVAL" default:"15s"`
	TargetNodeID   string        `envconfig:"TARGET_NODE_ID" default:"agent1"`
}

// server is used to implement master.ConfigurationMasterServer.
type server struct {
	pb.UnimplementedConfigurationMasterServer
	signingKey ed25519.PrivateKey
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

// GetCatalog implements master.ConfigurationMasterServer
func (s *server) GetCatalog(ctx context.Context, in *pb.GetCatalogRequest) (*pb.GetCatalogResponse, error) {
	nodeID := in.GetNodeId()
	receivedFacts := in.GetFacts()
	reqLogger := logger.With("node_id", nodeID)
	reqLogger.Info("Received GetCatalog request", "facts", receivedFacts)

	// Read catalog from YAML file
	yamlFile, err := ioutil.ReadFile("catalog.yaml")
	if err != nil {
		reqLogger.Error("Error reading catalog.yaml", "error", err)
		return nil, fmt.Errorf("failed to read catalog: %w", err)
	}

	// Unmarshal YAML into a generic map
	var catalogContainer map[string]any
	err = yaml.Unmarshal(yamlFile, &catalogContainer)
	if err != nil {
		reqLogger.Error("Error unmarshalling catalog YAML", "error", err)
		return nil, fmt.Errorf("failed to parse catalog YAML: %w", err)
	}

	spec, ok := catalogContainer["spec"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("catalog YAML missing spec or not a map")
	}
	resources, ok := spec["resources"].([]any)
	if !ok {
		return nil, fmt.Errorf("catalog YAML missing spec.resources or not a list")
	}

	rawResources := make([]json.RawMessage, 0, len(resources))
	for _, res := range resources {
		raw, err := json.Marshal(res) // Re-marshal each resource to get json.RawMessage
		if err != nil {
			return nil, fmt.Errorf("failed to marshal resource to JSON: %w", err)
		}
		rawResources = append(rawResources, raw)
	}

	// Hydrate the resources
	hydratedResources, err := catalog.HydrateCatalog(rawResources, receivedFacts)
	if err != nil {
		reqLogger.Error("Error hydrating catalog", "error", err)
		return nil, fmt.Errorf("failed to hydrate catalog: %w", err)
	}

	// Create the final catalog structure with hydrated resources
	finalCatalog := struct {
		APIVersion string                 `json:"apiVersion"`
		Kind       string                 `json:"kind"`
		Metadata   map[string]any `json:"metadata"`
		Spec       struct {
			Resources []any `json:"resources"`
		} `json:"spec"`
	}{
		APIVersion: catalogContainer["apiVersion"].(string),
		Kind:       catalogContainer["kind"].(string),
		Metadata:   catalogContainer["metadata"].(map[string]any),
		Spec: struct {
			Resources []any `json:"resources"`
		}{Resources: hydratedResources},
	}

	// Convert Go struct to JSON
	jsonBytes, err := json.Marshal(finalCatalog)
	if err != nil {
		reqLogger.Error("Error marshalling to JSON", "error", err)
		return nil, fmt.Errorf("failed to marshal catalog to JSON: %w", err)
	}
	catalogContent := string(jsonBytes)

	// Sign the JSON content
	signature := ed25519.Sign(s.signingKey, []byte(catalogContent))
	reqLogger.Info("Catalog signed", "algorithm", "ed25519")

	return &pb.GetCatalogResponse{
		Catalog: &pb.Catalog{
			Content: catalogContent,
			Format:  "json",
		},
		Signature:          signature,
		SignatureAlgorithm: "ed25519",
	}, nil
}

func setupNATS(cfg MasterConfig) (*nats.Conn, nats.JetStreamContext, error) {
	cert, err := tls.LoadX509KeyPair(cfg.NatsClientCert, cfg.NatsClientKey)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to load NATS client key pair: %w", err)
	}

	caCert, err := os.ReadFile(cfg.NatsRootCA)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read NATS root CA file: %w", err)
	}
	caCertPool := x509.NewCertPool()
	caCertPool.AppendCertsFromPEM(caCert)

	natsTLSConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
		RootCAs:      caCertPool,
		MinVersion:   tls.VersionTLS12,
	}

	opts := []nats.Option{
		nats.Secure(natsTLSConfig),
		nats.Name("Praetor Master"),
	}
	nc, err := nats.Connect(cfg.NatsURL, opts...)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to connect to NATS: %w", err)
	}

	js, err := nc.JetStream()
	if err != nil {
		nc.Close()
		return nil, nil, fmt.Errorf("failed to get JetStream context: %w", err)
	}

	// Ensure the AGENT_TRIGGERS stream exists
	_, err = js.AddStream(&nats.StreamConfig{
		Name:     "AGENT_TRIGGERS",
		Subjects: []string{"agent.trigger.>"},
	})
	if err != nil {
		if err != nats.ErrStreamNameAlreadyInUse {
			nc.Close()
			return nil, nil, fmt.Errorf("failed to add AGENT_TRIGGERS stream: %w", err)
		}
	}
	return nc, js, nil
}

func startTriggerPublisher(js nats.JetStreamContext, cfg MasterConfig) {
	ticker := time.NewTicker(cfg.TriggerInterval)
	defer ticker.Stop()

	subject := fmt.Sprintf("agent.trigger.getCatalog.%s", cfg.TargetNodeID)

	for {
		select {
		case <-ticker.C:
			logger.Info("Publishing catalog trigger", "subject", subject)
			_, err := js.Publish(subject, nil)
			if err != nil {
				logger.Error("Failed to publish trigger message", "subject", subject, "error", err)
			} else {
				logger.Info("Successfully published trigger", "subject", subject)
			}
		}
	}
}

func main() {
	// Initialize logger
	logLevel := slog.LevelInfo
	if os.Getenv("LOG_LEVEL") == "debug" {
		logLevel = slog.LevelDebug
	}
	logger = slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: logLevel}))
	slog.SetDefault(logger)

	logger.Info("Master server starting...")

	var masterCfg MasterConfig
	if err := envconfig.Process("MASTER", &masterCfg); err != nil {
		logger.Error("Failed to process master config", "error", err)
		os.Exit(1)
	}

	// Setup NATS connection and JetStream
	nc, js, err := setupNATS(masterCfg)
	if err != nil {
		logger.Error("Failed to setup NATS", "error", err)
		os.Exit(1)
	}
	defer nc.Close()
	logger.Info("Connected to NATS server and got JetStream context")

	// Start the periodic trigger publisher
	go startTriggerPublisher(js, masterCfg)

	// Load server certificate and key for gRPC
	serverCert, err := tls.LoadX509KeyPair("certs/server.crt", "certs/server.key")
	if err != nil {
		logger.Error("failed to load server certificate and key", "error", err)
		os.Exit(1)
	}

	// Load CA certificate for gRPC
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

	// Load signing key
	signingKey, err := loadPrivateKey("certs/master_signing.key")
	if err != nil {
		logger.Error("failed to load master signing key", "error", err)
		os.Exit(1)
	}
	logger.Info("Master signing key loaded.")

	// Create TLS credentials for gRPC
	grpcTLSConfig := &tls.Config{
		Certificates: []tls.Certificate{serverCert},
		ClientAuth:   tls.RequireAndVerifyClientCert,
		ClientCAs:    caCertPool,
	}
	creds := credentials.NewTLS(grpcTLSConfig)

	// Create gRPC server
	s := grpc.NewServer(grpc.Creds(creds))
	pb.RegisterConfigurationMasterServer(s, &server{signingKey: signingKey})
	reflection.Register(s)

	lis, err := net.Listen("tcp", port)
	if err != nil {
		logger.Error("failed to listen", "port", port, "error", err)
		os.Exit(1)
	}

	logger.Info("Master server listening on gRPC", "port", port, "tls", "mTLS")
	if err := s.Serve(lis); err != nil {
		logger.Error("failed to serve gRPC", "error", err)
		os.Exit(1)
	}
}