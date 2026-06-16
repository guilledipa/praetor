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

func (m *mockKV) Keys(ctx context.Context, opts ...jetstream.WatchOpt) ([]string, error) {
	var keys []string
	for k := range m.bucket {
		keys = append(keys, k)
	}
	if len(keys) == 0 {
		return nil, jetstream.ErrNoKeysFound
	}
	return keys, nil
}

type mockKVEntry struct {
	jetstream.KeyValueEntry
	val []byte
}

func (e *mockKVEntry) Value() []byte { return e.val }

func (m *mockKV) Get(ctx context.Context, key string) (jetstream.KeyValueEntry, error) {
	val, exists := m.bucket[key]
	if !exists {
		return nil, jetstream.ErrKeyNotFound
	}
	return &mockKVEntry{val: val}, nil
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

func TestKRM_JetStreamMock(t *testing.T) {
	mockSpecs := &mockKV{}
	mockStatuses := &mockKV{}
	provider := &natsProvider{specs: mockSpecs, statuses: mockStatuses}

	ctx := context.Background()
	nodeID := "node-1"
	kind := "File"
	name := "/etc/motd"
	specData := []byte(`{"content":"hello"}`)
	statusData := []byte(`{"status":"Succeeded"}`)

	if err := provider.StoreResourceSpec(ctx, nodeID, kind, name, specData); err != nil {
		t.Fatalf("StoreResourceSpec failed: %v", err)
	}
	if err := provider.StoreResourceStatus(ctx, nodeID, kind, name, statusData); err != nil {
		t.Fatalf("StoreResourceStatus failed: %v", err)
	}

	specs, err := provider.GetAgentSpecs(ctx, nodeID)
	if err != nil {
		t.Fatalf("GetAgentSpecs failed: %v", err)
	}
	expectedSpecKey := "nodes.node-1.spec.File._etc_motd"
	if string(specs[expectedSpecKey]) != string(specData) {
		t.Errorf("Expected spec data %s, got %s", specData, specs[expectedSpecKey])
	}

	statuses, err := provider.GetAgentStatuses(ctx, nodeID)
	if err != nil {
		t.Fatalf("GetAgentStatuses failed: %v", err)
	}
	expectedStatusKey := "nodes.node-1.status.File._etc_motd"
	if string(statuses[expectedStatusKey]) != string(statusData) {
		t.Errorf("Expected status data %s, got %s", statusData, statuses[expectedStatusKey])
	}
}
