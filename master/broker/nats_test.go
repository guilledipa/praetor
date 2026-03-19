package broker

import (
	"log/slog"
	"os"
	"testing"
	"time"
)

type mockMessage struct {
	data []byte
}

func (m *mockMessage) Data() []byte {
	return m.data
}

func (m *mockMessage) Subject() string {
	return "test"
}

func (m *mockMessage) Ack() error {
	return nil
}

func TestNatsBroadcaster_handleAgentRegistration(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	b := NewNatsBroadcaster(Config{}, logger)
	msg := &mockMessage{data: []byte("agent-123")}

	b.handleAgentRegistration(msg)

	val, ok := b.activeAgents.Load("agent-123")
	if !ok {
		t.Fatalf("Agent not registered")
	}

	_, ok = val.(time.Time)
	if !ok {
		t.Fatalf("Value not a time.Time")
	}
}
