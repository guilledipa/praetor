package main

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io/ioutil"
	"log"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/guilledipa/praetor/agent/facts"
	"github.com/guilledipa/praetor/agent/resources"
	masterpb "github.com/guilledipa/praetor/proto/gen/master"
	"github.com/kelseyhightower/envconfig"
	"github.com/nats-io/nats.go"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	_ "github.com/guilledipa/praetor/agent/facts/core"     // Register core facts
	_ "github.com/guilledipa/praetor/agent/resources/exec" // Register exec resource
	_ "github.com/guilledipa/praetor/agent/resources/file" // Register file resource
	_ "github.com/guilledipa/praetor/agent/resources/pkg"  // Register package resource
	_ "github.com/guilledipa/praetor/agent/resources/svc"  // Register service resource
)

var logger *slog.Logger

// Config represents the agent configuration.
type Config struct {
	NatsURL             string `envconfig:"NATS_URL" default:"nats://localhost:4222"`
	NatsClientCert      string `envconfig:"NATS_CLIENT_CERT" default:"certs/client.crt"`
	NatsClientKey       string `envconfig:"NATS_CLIENT_KEY" default:"certs/client.key"`
	NatsRootCA          string `envconfig:"NATS_ROOT_CA" default:"certs/master-ca.crt"`
	MasterGRPCAddress   string `envconfig:"MASTER_GRPC_ADDRESS" default:"localhost:50051"`
	MasterClientCert    string `envconfig:"MASTER_CLIENT_CERT" default:"certs/client.crt"`
	MasterClientKey     string `envconfig:"MASTER_CLIENT_KEY" default:"certs/client.key"`
	MasterRootCA        string `envconfig:"MASTER_ROOT_CA" default:"certs/master-ca.crt"`
	NodeID              string `envconfig:"NODE_ID" default:"agent1"`
	LogLevel            string `envconfig:"LOG_LEVEL" default:"info"`
	AgentBootstrapToken string `envconfig:"AGENT_BOOTSTRAP_TOKEN" default:"praetor-secret-token"`
}

func main() {
	var cfg Config
	err := envconfig.Process("AGENT", &cfg)
	if err != nil {
		log.Fatalf("Failed to process config: %v", err)
	}

	// Initialize logger
	var logLevel slog.Level
	switch cfg.LogLevel {
	case "debug":
		logLevel = slog.LevelDebug
	case "warn":
		logLevel = slog.LevelWarn
	case "error":
		logLevel = slog.LevelError
	default:
		logLevel = slog.LevelInfo
	}
	logger = slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: logLevel}))
	logger = logger.With("node_id", cfg.NodeID)
	slog.SetDefault(logger)

	logger.Info("Agent starting...")

	// Auto-enrollment logic
	if err := runBootstrapWorkflow(cfg, logger); err != nil {
		logger.Error("Bootstrap enrollment failed", "error", err)
		os.Exit(1)
	}

	// Load TLS configurations
	natsTLSConfig, err := setupTLS(cfg.NatsClientCert, cfg.NatsClientKey, cfg.NatsRootCA)
	if err != nil {
		logger.Error("Failed to setup NATS TLS", "error", err)
		os.Exit(1)
	}
	logger.Info("NATS TLS config loaded.")

	masterTLSConfig, err := setupTLS(cfg.MasterClientCert, cfg.MasterClientKey, cfg.MasterRootCA)
	if err != nil {
		logger.Error("Failed to setup Master gRPC TLS", "error", err)
		os.Exit(1)
	}
	logger.Info("Master gRPC TLS config loaded.")

	// Connect to NATS
	nc, err := connectNATS(cfg.NatsURL, natsTLSConfig)
	if err != nil {
		logger.Error("Failed to connect to NATS", "error", err)
		os.Exit(1)
	}
	defer nc.Close()
	logger.Info("Connected to NATS server")

	// Register with the Master
	if err := nc.Publish("agent.register", []byte(cfg.NodeID)); err != nil {
		logger.Error("Failed to publish registration message", "error", err)
	} else {
		logger.Info("Registration message published.")
	}

	// Get JetStream context
	js, err := nc.JetStream()
	if err != nil {
		logger.Error("Failed to get JetStream context", "error", err)
		os.Exit(1)
	}
	logger.Info("JetStream context obtained.")

	// Ensure the Streams exist
	streamConfigs := []*nats.StreamConfig{
		{Name: "COMMANDS", Subjects: []string{"commands.>"}},
		{Name: "AGENT_TRIGGERS", Subjects: []string{"agent.trigger.>"}},
	}
	for _, sc := range streamConfigs {
		_, err = js.AddStream(sc)
		if err != nil {
			logger.Error("Failed to add JetStream stream", "stream", sc.Name, "error", err)
			if err != nats.ErrStreamNameAlreadyInUse {
				os.Exit(1)
			}
			logger.Info("JetStream stream already exists", "stream", sc.Name)
		} else {
			logger.Info("JetStream stream created", "stream", sc.Name)
		}
	}

	// Connect to Master gRPC
	masterClient, masterConn, err := connectMasterGRPC(cfg, masterTLSConfig)
	if err != nil {
		logger.Error("Failed to connect to master gRPC", "error", err)
		os.Exit(1)
	}
	defer masterConn.Close()
	logger.Info("Connected to Master gRPC server", "address", cfg.MasterGRPCAddress)

	masterPubKey, err := loadPublicKey("../master/certs/master_signing.pub")
	if err != nil {
		logger.Error("Failed to load master public key", "error", err)
		os.Exit(1)
	}
	logger.Info("Master public key loaded.")

	// Start catalog trigger subscriber
	triggerSubject := fmt.Sprintf("agent.trigger.getCatalog.%s", cfg.NodeID)
	go startJetStreamPullSubscriber(js, cfg.NodeID, triggerSubject, "AGENT_TRIGGERS", func(msg *nats.Msg) {
		logger.Info("Received catalog trigger", "subject", msg.Subject)
		fetchAndApplyCatalog(cfg, masterClient, masterPubKey)
	})

	logger.Info("Agent setup complete, waiting for triggers and commands.")
	// Keep the main function running
	select {}
}

func setupTLS(clientCert, clientKey, rootCA string) (*tls.Config, error) {
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

func connectNATS(natsURL string, tlsConfig *tls.Config) (*nats.Conn, error) {
	opts := []nats.Option{
		nats.Secure(tlsConfig),
		nats.Name("Praetor Agent"),
		nats.ReconnectWait(2 * time.Second),
		nats.MaxReconnects(-1),
	}
	return nats.Connect(natsURL, opts...)
}

func connectMasterGRPC(cfg Config, tlsConfig *tls.Config) (masterpb.ConfigurationMasterClient, *grpc.ClientConn, error) {
	creds := credentials.NewTLS(tlsConfig)
	conn, err := grpc.Dial(
		cfg.MasterGRPCAddress,
		grpc.WithTransportCredentials(creds),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to dial master gRPC server: %w", err)
	}

	client := masterpb.NewConfigurationMasterClient(conn)
	return client, conn, nil
}

type natsMessageHandler func(msg *nats.Msg)

func startJetStreamPullSubscriber(js nats.JetStreamContext, nodeID, subject, streamName string, handler natsMessageHandler) {
	consumerName := fmt.Sprintf("%s-%s-consumer", nodeID, strings.ReplaceAll(subject, ".", "-"))
	if subject == "commands.>" {
		consumerName = fmt.Sprintf("%s-commands-consumer", nodeID)
	}

	sub, err := js.PullSubscribe(subject, consumerName, nats.BindStream(streamName))
	if err != nil {
		logger.Error("Failed to create pull subscription", "subject", subject, "consumer", consumerName, "stream", streamName, "error", err)
		return
	}
	logger.Info("Created pull subscription", "subject", subject, "consumer", consumerName, "stream", streamName)

	for {
		msgs, err := sub.Fetch(1, nats.MaxWait(10*time.Second))
		if err != nil {
			if err == nats.ErrTimeout {
				continue
			}
			logger.Error("Error fetching messages from JetStream", "error", err, "subject", subject)
			time.Sleep(5 * time.Second) // Backoff on error
			continue
		}

		for _, msg := range msgs {
			logger.Info("Received message", "subject", msg.Subject)
			handler(msg)
			if err := msg.Ack(); err != nil {
				logger.Error("Failed to acknowledge message", "error", err, "subject", msg.Subject)
			} else {
				logger.Debug("Message acknowledged", "subject", msg.Subject)
			}
		}
	}
}

func fetchAndApplyCatalog(cfg Config, masterClient masterpb.ConfigurationMasterClient, masterPubKey ed25519.PublicKey) {
	logger.Info("--- Running Configuration Check ---")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	agentFacts := facts.Collect()
	stringFacts := make(map[string]string)
	for k, v := range agentFacts {
		stringFacts[k] = fmt.Sprintf("%v", v)
	}
	logger.Info("Collected facts", "facts", stringFacts)

	resp, err := masterClient.GetCatalog(ctx, &masterpb.GetCatalogRequest{NodeId: cfg.NodeID, Facts: stringFacts})
	if err != nil {
		logger.Error("Error fetching catalog from master", "error", err)
		return
	}

	catalogContent := resp.GetCatalog().GetContent()
	signature := resp.GetSignature()

	// Verify signature
	if len(signature) == 0 {
		logger.Warn("No signature found in catalog response")
		return
	}

	if !ed25519.Verify(masterPubKey, []byte(catalogContent), signature) {
		logger.Error("Catalog signature verification failed!")
		return
	}
	logger.Info("Catalog signature verified successfully.")

	var catalog Catalog
	if err := json.Unmarshal([]byte(catalogContent), &catalog); err != nil {
		logger.Error("Error unmarshalling catalog JSON", "error", err)
		return
	}
		logger.Info("Successfully fetched and parsed catalog", "resource_count", len(catalog.Spec.Resources))
	
		allCompliant := true
		var reports []*masterpb.ResourceReport

		for _, resData := range catalog.Spec.Resources {
			var typeMeta struct {
				Kind string `json:"kind"`
			}
			if err := json.Unmarshal(resData, &typeMeta); err != nil {
				logger.Error("Error unmarshalling resource kind", "error", err)
				reports = append(reports, &masterpb.ResourceReport{
					Type: "Unknown",
					Compliant: false,
					Message: fmt.Sprintf("Failed to unmarshal kind: %v", err),
				})
				allCompliant = false
				continue
			}
	
			logger.Debug("Processing resource", "kind", typeMeta.Kind, "spec", string(resData))
	
			res, err := resources.CreateResource(typeMeta.Kind, resData)
			if err != nil {
				logger.Error("Error creating resource instance", "kind", typeMeta.Kind, "error", err)
				reports = append(reports, &masterpb.ResourceReport{
					Type: typeMeta.Kind,
					Compliant: false,
					Message: fmt.Sprintf("Failed to create instance: %v", err),
				})
				allCompliant = false
				continue
			}

			report := &masterpb.ResourceReport{
				Type: res.Type(),
				Id: res.ID(),
				Compliant: true,
				Message: "Resource is compliant",
			}
	
			resLogger := logger.With("resource_type", res.Type(), "resource_id", res.ID())
	
			currentState, err := res.Get()
			if err != nil {
				resLogger.Error("Error getting state", "error", err)
				report.Compliant = false
				report.Message = fmt.Sprintf("Error getting state: %v", err)
				reports = append(reports, report)
				allCompliant = false
				continue
			}
	
			isCompliant, err := res.Test(currentState)
			if err != nil {
				resLogger.Error("Error testing state", "error", err)
				report.Compliant = false
				report.Message = fmt.Sprintf("Error testing state: %v", err)
				reports = append(reports, report)
				allCompliant = false
				continue
			}
	
			if !isCompliant {
				allCompliant = false
				report.Compliant = false
				resLogger.Info("Drift detected. Enforcing desired state...")
				err := res.Set()
				if err != nil {
						report.Message = fmt.Sprintf("Failed to enforce: %v", err)
						resLogger.Error("Error setting state", "error", err)
				} else {
						report.Message = "Drift detected but successfully enforced state"
						report.Compliant = true // Technically compliant now
						resLogger.Info("Successfully enforced state")
				}
			} else {
					resLogger.Info("Resource is compliant", "type", res.Type(), "id", res.ID())
			}
			reports = append(reports, report)
		}

		logger.Info("--- Configuration Check Finished ---")
		
		// Send the state report
		_, err = masterClient.ReportState(ctx, &masterpb.ReportStateRequest{
			NodeId:      cfg.NodeID,
			Resources:   reports,
			IsCompliant: allCompliant,
			Timestamp:   time.Now().Unix(),
		})
		if err != nil {
			logger.Error("Failed to report state to master", "error", err)
		} else {
			logger.Info("Successfully reported state to master")
		}
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

func loadPublicKey(path string) (ed25519.PublicKey, error) {
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

// Catalog represents the structure of the catalog received from the master.
type Catalog struct {
	APIVersion string             `json:"apiVersion"`
	Kind       string             `json:"kind"`
	Metadata   map[string]string  `json:"metadata"`
	Spec       struct {
		Resources []json.RawMessage `json:"resources"`
	} `json:"spec"`
}

// CatalogResource represents a single resource entry in the catalog.
type CatalogResource struct {
	Type string          `json:"type"`
	Spec json.RawMessage `json:"spec"`
}

func runBootstrapWorkflow(cfg Config, logger *slog.Logger) error {
	clientCertPath := cfg.NatsClientCert
	clientKeyPath := cfg.NatsClientKey

	if _, err := os.Stat(clientCertPath); err == nil {
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
		BootstrapToken: cfg.AgentBootstrapToken,
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

	os.MkdirAll(filepath.Dir(clientKeyPath), 0755)

	if err := os.WriteFile(clientKeyPath, privPEM, 0600); err != nil {
		return err
	}
	if err := os.WriteFile(clientCertPath, []byte(resp.GetCertificatePem()), 0644); err != nil {
		return err
	}

	logger.Info("Enrollment successful! Certificate provisioned locally.")
	return nil
}
