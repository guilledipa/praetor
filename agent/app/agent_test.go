package app

import (
	"log/slog"
	"os"
	"testing"
)

func TestNewAgent(t *testing.T) {
	cfg := Config{
		NatsURL: "nats://test:4222",
		NodeID:  "agent-test",
	}
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	agent := NewAgent(cfg, logger)

	if agent == nil {
		t.Fatal("Expected agent to be created")
	}

	if agent.Config.NodeID != "agent-test" {
		t.Errorf("Expected NodeID agent-test, got %s", agent.Config.NodeID)
	}
}
