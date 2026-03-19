package natsjs

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/guilledipa/praetor/proto/gen/master"
)

// mockKV perfectly mimics a NATS jetstream KeyValue bucket
type mockKV struct {
	jetstream.KeyValue // embed interface so we don't have to stub everything
	bucket map[string][]byte
}

func (m *mockKV) Put(ctx context.Context, key string, val []byte) (uint64, error) {
	if m.bucket == nil {
		m.bucket = make(map[string][]byte)
	}
	m.bucket[key] = val
	return 1, nil
}

func TestStoreReport_JetStreamMock(t *testing.T) {
	mkv := &mockKV{}
	provider := &natsProvider{kv: mkv}

	rep := &master.ResourceReport{
		Type:      "File",
		Id:        "/etc/praetor/secret.key",
		Compliant: true,
		Message:   "Synchronized with master templates",
	}

	err := provider.StoreReport(context.Background(), "node-x-12", rep)
	if err != nil {
		t.Fatalf("StoreReport failed pushing to JetStream mock: %v", err)
	}

	expectedKey := "node-x-12.File._etc_praetor_secret.key"
	payload, exists := mkv.bucket[expectedKey]
	if !exists {
		t.Fatalf("Expected JetStream Key %q, but map only contains %+v", expectedKey, mkv.bucket)
	}

	var trace ReportLog
	if err := json.Unmarshal(payload, &trace); err != nil {
		t.Fatalf("Failed unmarshaling JSON report schema from KV: %v", err)
	}

	if trace.NodeID != "node-x-12" {
		t.Errorf("Expected node node-x-12, got %s", trace.NodeID)
	}
	if trace.Compliant != true {
		t.Errorf("Expected compliance true")
	}
	if trace.Message != "Synchronized with master templates" {
		t.Errorf("Message corrupted during transit")
	}
}
