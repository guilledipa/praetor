package app

import (
	"crypto/tls"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/guilledipa/praetor/agent/engine"
	"github.com/guilledipa/praetor/agent/pki"
	masterpb "github.com/guilledipa/praetor/proto/gen/master"
	"github.com/nats-io/nats.go"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

type Config struct {
	NatsURL             string
	NatsClientCert      string
	NatsClientKey       string
	NatsRootCA          string
	MasterGRPCAddress   string
	MasterClientCert    string
	MasterClientKey     string
	MasterRootCA        string
	NodeID              string
	LogLevel            string
	AgentBootstrapToken string
}

type Agent struct {
	Config Config
	Logger *slog.Logger
}

func NewAgent(cfg Config, logger *slog.Logger) *Agent {
	return &Agent{
		Config: cfg,
		Logger: logger,
	}
}

func (a *Agent) Run() error {
	cfg := a.Config
	logger := a.Logger

	natsTLSConfig, err := pki.SetupTLS(cfg.NatsClientCert, cfg.NatsClientKey, cfg.NatsRootCA)
	if err != nil {
		return fmt.Errorf("failed to setup NATS TLS: %w", err)
	}

	masterTLSConfig, err := pki.SetupTLS(cfg.MasterClientCert, cfg.MasterClientKey, cfg.MasterRootCA)
	if err != nil {
		return fmt.Errorf("failed to setup Master gRPC TLS: %w", err)
	}

	nc, err := connectNATS(cfg.NatsURL, natsTLSConfig)
	if err != nil {
		return fmt.Errorf("failed to connect to NATS: %w", err)
	}
	defer nc.Close()
	logger.Info("Connected to NATS server")

	if err := nc.Publish("agent.register", []byte(cfg.NodeID)); err != nil {
		logger.Error("Failed to publish registration message", "error", err)
	} else {
		logger.Info("Registration message published.")
	}

	js, err := nc.JetStream()
	if err != nil {
		return fmt.Errorf("failed to get JetStream context: %w", err)
	}

	streamConfigs := []*nats.StreamConfig{
		{Name: "COMMANDS", Subjects: []string{"commands.>"}},
		{Name: "AGENT_TRIGGERS", Subjects: []string{"agent.trigger.>"}},
	}
	for _, sc := range streamConfigs {
		_, err = js.AddStream(sc)
		if err != nil {
			if err != nats.ErrStreamNameAlreadyInUse {
				return fmt.Errorf("failed to add JetStream stream %s: %w", sc.Name, err)
			}
		}
	}

	masterClient, masterConn, err := connectMasterGRPC(cfg, masterTLSConfig)
	if err != nil {
		return fmt.Errorf("failed to connect to master gRPC: %w", err)
	}
	defer masterConn.Close()
	logger.Info("Connected to Master gRPC server", "address", cfg.MasterGRPCAddress)

	masterPubKey, err := pki.LoadPublicKey("../master/certs/master_signing.pub")
	if err != nil {
		return fmt.Errorf("failed to load master public key: %w", err)
	}

	triggerSubject := fmt.Sprintf("agent.trigger.getCatalog.%s", cfg.NodeID)
	
	go startJetStreamPullSubscriber(js, cfg.NodeID, triggerSubject, "AGENT_TRIGGERS", logger, func(msg *nats.Msg) {
		logger.Info("Received catalog trigger", "subject", msg.Subject)
		exec := &engine.Executor{
			NodeID:       cfg.NodeID,
			MasterClient: masterClient,
			MasterPubKey: masterPubKey,
			Logger:       logger,
		}
		exec.FetchAndApplyCatalog()
	})

	logger.Info("Agent setup complete, waiting for triggers and commands.")
	select {}
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

func startJetStreamPullSubscriber(js nats.JetStreamContext, nodeID, subject, streamName string, logger *slog.Logger, handler natsMessageHandler) {
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
