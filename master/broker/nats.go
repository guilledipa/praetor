package broker

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"time"

	pkgbroker "github.com/guilledipa/praetor/pkg/broker"
	"github.com/guilledipa/praetor/pkg/broker/nats"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
)

type Config struct {
	NatsURL         string
	NatsClientCert  string
	NatsClientKey   string
	NatsRootCA      string
	TriggerInterval time.Duration
	TargetNodeID    string
}

type NatsBroadcaster struct {
	cfg          Config
	logger       *slog.Logger
	activeAgents sync.Map
	broker       pkgbroker.Broker
}

func NewNatsBroadcaster(cfg Config, logger *slog.Logger) *NatsBroadcaster {
	return &NatsBroadcaster{
		cfg:    cfg,
		logger: logger,
	}
}

func (n *NatsBroadcaster) handleAgentRegistration(msg pkgbroker.Message) {
	agentID := string(msg.Data())
	if agentID != "" {
		n.activeAgents.Store(agentID, time.Now())
		n.logger.Info("Agent registered dynamically", "node_id", agentID)
	}
}

func (n *NatsBroadcaster) SetupBroker() (pkgbroker.Broker, error) {
	cert, err := tls.LoadX509KeyPair(n.cfg.NatsClientCert, n.cfg.NatsClientKey)
	if err != nil {
		return nil, fmt.Errorf("failed to load client key pair: %w", err)
	}

	caCert, err := os.ReadFile(n.cfg.NatsRootCA)
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
	err = b.Connect("Praetor Master", n.cfg.NatsURL, tlsConfig)
	if err != nil {
		return nil, err
	}

	// Subscribe to agent registrations
	err = b.Subscribe("agent.register", n.handleAgentRegistration)
	if err != nil {
		n.logger.Error("Failed to subscribe to agent.register", "error", err)
	}

	// Ensure the AGENT_TRIGGERS stream exists
	err = b.EnsureStream("AGENT_TRIGGERS", []string{"agent.trigger.>"})
	if err != nil {
		b.Close()
		return nil, fmt.Errorf("failed to add AGENT_TRIGGERS stream: %w", err)
	}
	n.broker = b
	return b, nil
}

func (n *NatsBroadcaster) StartTriggerPublisher(b pkgbroker.Broker) {
	ticker := time.NewTicker(n.cfg.TriggerInterval)
	defer ticker.Stop()

	// Seed with configured target if you want it to still trigger agent1 statically safely
	if n.cfg.TargetNodeID != "" {
		n.activeAgents.Store(n.cfg.TargetNodeID, time.Now())
	}

	for {
		select {
		case <-ticker.C:
			ctx, span := otel.Tracer("praetor-master").Start(context.Background(), "TriggerCatalogUpdate")
			
			count := 0
			n.activeAgents.Range(func(key, value interface{}) bool {
				agentID := key.(string)
				subject := fmt.Sprintf("agent.trigger.getCatalog.%s", agentID)

				// Create a child span per agent if needed, or just link it.
				agentCtx, agentSpan := otel.Tracer("praetor-master").Start(ctx, "PublishTrigger")
				agentSpan.SetAttributes(attribute.String("agent.id", agentID))

				n.logger.Debug("Publishing catalog trigger", "subject", subject)
				err := b.Publish(agentCtx, subject, nil)
				if err != nil {
					n.logger.Error("Failed to publish trigger message", "subject", subject, "error", err)
					agentSpan.RecordError(err)
				}
				agentSpan.End()
				count++
				return true
			})
			n.logger.Info("Published triggers completed", "agents_triggered", count)
			span.End()
		}
	}
}
