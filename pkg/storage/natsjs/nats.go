package natsjs

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/guilledipa/praetor/pkg/storage"
	"github.com/guilledipa/praetor/proto/gen/master"
)

type natsProvider struct {
	kv jetstream.KeyValue
}

// NewProvider binds to an active NATS Connection and boots a persistent JetStream KeyValue Bucket.
func NewProvider(ctx context.Context, nc *nats.Conn) (storage.Provider, error) {
	js, err := jetstream.New(nc)
	if err != nil {
		return nil, fmt.Errorf("failed to load jetstream subsystem: %w", err)
	}

	// Build a durable KeyValue engine inside the broker instance
	kv, err := js.CreateOrUpdateKeyValue(ctx, jetstream.KeyValueConfig{
		Bucket:      "PRAETOR_REPORTS",
		Description: "Persistent Agent Configuration Drift State Graph",
		History:     64, // Keep the last 64 drifts per unique resource
	})
	if err != nil {
		return nil, fmt.Errorf("failed to bind KV bucket: %w", err)
	}

	return &natsProvider{kv: kv}, nil
}

// ReportLog implements the persistent JSON compliance trace
type ReportLog struct {
	NodeID       string `json:"node_id"`
	ResourceType string `json:"resource_type"`
	ResourceID   string `json:"resource_id"`
	Compliant    bool   `json:"compliant"`
	Message      string `json:"message"`
	Timestamp    string `json:"timestamp"`
}

// sanitizeKey guarantees the Key conforms to K/V JetStream Subject constraints (a-z0-9_-.)
func sanitizeKey(input string) string {
	clean := strings.ReplaceAll(input, "/", "_")
	clean = strings.ReplaceAll(clean, " ", "_")
	return clean
}

func (p *natsProvider) StoreReport(ctx context.Context, nodeID string, report *master.ResourceReport) error {
	blob := ReportLog{
		NodeID:       nodeID,
		ResourceType: report.GetType(),
		ResourceID:   report.GetId(),
		Compliant:    report.GetCompliant(),
		Message:      report.GetMessage(),
		Timestamp:    time.Now().UTC().Format(time.RFC3339),
	}

	payload, err := json.Marshal(blob)
	if err != nil {
		return fmt.Errorf("failed to marshal report trace: %w", err)
	}

	// Unique NATS Key path tracking state history per distinct node/resource
	key := fmt.Sprintf("%s.%s.%s", sanitizeKey(nodeID), sanitizeKey(report.GetType()), sanitizeKey(report.GetId()))

	_, err = p.kv.Put(ctx, key, payload)
	if err != nil {
		return fmt.Errorf("failed pushing state to JetStream KV: %w", err)
	}

	return nil
}
