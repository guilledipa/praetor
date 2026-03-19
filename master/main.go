package main

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io/ioutil"
	"log/slog"
	"math/big"
	"net"
	"os"
	"sync"
	"time"

	"github.com/guilledipa/praetor/master/catalog"
	"github.com/guilledipa/praetor/pkg/broker"
	"github.com/guilledipa/praetor/pkg/broker/nats"
	"github.com/guilledipa/praetor/pkg/secrets"
	"github.com/guilledipa/praetor/pkg/secrets/local"
	pb "github.com/guilledipa/praetor/proto/gen/master"
	"github.com/kelseyhightower/envconfig"
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
	NatsRootCA      string        `envconfig:"NATS_ROOT_CA" default:"../nats/certs/ca.crt"`
	TriggerInterval time.Duration `envconfig:"TRIGGER_INTERVAL" default:"15s"`
	TargetNodeID    string        `envconfig:"TARGET_NODE_ID" default:"agent1"`
	BootstrapToken  string        `envconfig:"BOOTSTRAP_TOKEN" default:"praetor-secret-token"`
}

// server is used to implement master.ConfigurationMasterServer.
type server struct {
	pb.UnimplementedConfigurationMasterServer
	signingKey ed25519.PrivateKey
	secretProv secrets.Provider
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

	// 3. Hydrate catalog with secrets and facts
	hydratedResources, err := catalog.HydrateCatalog(rawResources, receivedFacts, s.secretProv)
	if err != nil {
		logger.Error("Failed to hydrate catalog", "error", err)
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

// ReportState implements master.ConfigurationMasterServer
func (s *server) ReportState(ctx context.Context, in *pb.ReportStateRequest) (*pb.ReportStateResponse, error) {
	nodeID := in.GetNodeId()
	isCompliant := in.GetIsCompliant()
	
	logger.Info("Received state report", "node_id", nodeID, "is_compliant", isCompliant, "resources_checked", len(in.GetResources()))
	for _, req := range in.GetResources() {
		logger.Debug("Resource run", "node_id", nodeID, "type", req.GetType(), "id", req.GetId(), "compliant", req.GetCompliant(), "message", req.GetMessage())
	}
	
	return &pb.ReportStateResponse{Acknowledged: true}, nil
}

type caServer struct {
	pb.UnimplementedCertificateAuthorityServer
	caCert         *x509.Certificate
	caPrivKey      interface{}
	bootstrapToken string
}

func (s *caServer) SignCSR(ctx context.Context, in *pb.SignCSRRequest) (*pb.SignCSRResponse, error) {
	if s.bootstrapToken != "" && in.GetBootstrapToken() != s.bootstrapToken {
		return nil, fmt.Errorf("invalid bootstrap token")
	}

	logger.Info("Received CSR signing request", "node_id", in.GetNodeId())

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

	certBytes, err := x509.CreateCertificate(rand.Reader, &template, s.caCert, csr.PublicKey, s.caPrivKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create certificate: %w", err)
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certBytes})
	
	logger.Info("Successfully signed CSR", "node_id", in.GetNodeId(), "serial", serialNumber.String())

	return &pb.SignCSRResponse{
		CertificatePem: string(certPEM),
	}, nil
}

var activeAgents sync.Map

func handleAgentRegistration(msg broker.Message) {
	agentID := string(msg.Data())
	if agentID != "" {
		activeAgents.Store(agentID, time.Now())
		logger.Info("Agent registered dynamically", "node_id", agentID)
	}
}

func setupBroker(cfg MasterConfig) (broker.Broker, error) {
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

	b := nats.NewBroker()
	err = b.Connect("Praetor Master", cfg.NatsURL, tlsConfig)
	if err != nil {
		return nil, err
	}

	// Subscribe to agent registrations
	err = b.Subscribe("agent.register", handleAgentRegistration)
	if err != nil {
		logger.Error("Failed to subscribe to agent.register", "error", err)
	}

	// Ensure the AGENT_TRIGGERS stream exists
	err = b.EnsureStream("AGENT_TRIGGERS", []string{"agent.trigger.>"})
	if err != nil {
		b.Close()
		return nil, fmt.Errorf("failed to add AGENT_TRIGGERS stream: %w", err)
	}
	return b, nil
}

func startTriggerPublisher(b broker.Broker, cfg MasterConfig) {
	ticker := time.NewTicker(cfg.TriggerInterval)
	defer ticker.Stop()

	// Seed with configured target if you want it to still trigger agent1 statically safely
	if cfg.TargetNodeID != "" {
		activeAgents.Store(cfg.TargetNodeID, time.Now())
	}

	for {
		select {
		case <-ticker.C:
			count := 0
			activeAgents.Range(func(key, value interface{}) bool {
				agentID := key.(string)
				subject := fmt.Sprintf("agent.trigger.getCatalog.%s", agentID)
				logger.Debug("Publishing catalog trigger", "subject", subject)
				err := b.PublishStream(subject, nil)
				if err != nil {
					logger.Error("Failed to publish trigger message", "subject", subject, "error", err)
				}
				count++
				return true
			})
			logger.Info("Published triggers completed", "agents_triggered", count)
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

	// Setup message broker
	b, err := setupBroker(masterCfg)
	if err != nil {
		logger.Error("Failed to setup message broker", "error", err)
		os.Exit(1)
	}
	defer b.Close()
	logger.Info("Connected to message broker successfully")

	// Start the periodic trigger publisher
	go startTriggerPublisher(b, masterCfg)

	// Load server certificate and key for gRPC
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

	// Bootstrap server setup
	bootCreds := credentials.NewServerTLSFromCert(&serverCert)
	bootServer := grpc.NewServer(grpc.Creds(bootCreds))
	pb.RegisterCertificateAuthorityServer(bootServer, &caServer{
		caCert:         caX509Cert,
		caPrivKey:      caTLSCert.PrivateKey,
		bootstrapToken: masterCfg.BootstrapToken,
	})
	
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

	// Load CA certificate for gRPC mTLS
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
	// Load Secrets Provider
	secretProv, err := local.NewProvider("secrets.yaml")
	if err != nil {
		logger.Warn("Failed to load secrets.yaml, providing blank secret fallback", "error", err)
	}
	pb.RegisterConfigurationMasterServer(s, &server{signingKey: signingKey, secretProv: secretProv})
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