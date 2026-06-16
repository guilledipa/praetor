package engine

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"os"
	"testing"
	"time"

	natsserverv2 "github.com/nats-io/nats-server/v2/server"
	natsserver "github.com/nats-io/nats-server/v2/test"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/assert"
)

func startTestNatsWithJetStream(t *testing.T) (*nats.Conn, *natsserverv2.Server) {
	opts := natsserver.DefaultTestOptions
	opts.Port = -1
	opts.JetStream = true
	opts.StoreDir = t.TempDir()
	s := natsserver.RunServer(&opts)

	nc, err := nats.Connect(s.ClientURL())
	if err != nil {
		s.Shutdown()
		t.Fatalf("failed to connect to test NATS: %v", err)
	}
	return nc, s
}

func TestReconciler_ReconcileAndVerify(t *testing.T) {
	pubKey, privKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("Failed to generate keys: %v", err)
	}

	nc, ns := startTestNatsWithJetStream(t)
	defer ns.Shutdown()
	defer nc.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	js, err := jetstream.New(nc)
	assert.NoError(t, err)

	specsKV, err := js.CreateOrUpdateKeyValue(ctx, jetstream.KeyValueConfig{
		Bucket: "PRAETOR_SPECS",
	})
	assert.NoError(t, err)

	statusKV, err := js.CreateOrUpdateKeyValue(ctx, jetstream.KeyValueConfig{
		Bucket: "PRAETOR_STATUS",
	})
	assert.NoError(t, err)

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	reconciler := NewReconciler("node-1", nc, pubKey, logger, false)

	err = reconciler.Start(ctx)
	assert.NoError(t, err)

	// Create a resource structure
	res := map[string]any{
		"apiVersion": "praetor.io/v1alpha1",
		"kind":       "TestResource",
		"metadata": map[string]any{
			"name": "my-test-res",
			"annotations": map[string]any{
				"praetor.io/signature": "",
			},
		},
		"spec": map[string]any{},
	}

	// Sign it
	delete(res["metadata"].(map[string]any)["annotations"].(map[string]any), "praetor.io/signature")
	cleanBytes, err := json.Marshal(res)
	assert.NoError(t, err)
	sig := ed25519.Sign(privKey, cleanBytes)
	
	// Reinject annotations and set the signature
	res["metadata"].(map[string]any)["annotations"] = map[string]any{
		"praetor.io/signature": hex.EncodeToString(sig),
	}

	finalBytes, err := json.Marshal(res)
	assert.NoError(t, err)

	// Publish to Specs KV
	specKey := "nodes.node-1.spec.TestResource.my-test-res"
	_, err = specsKV.Put(ctx, specKey, finalBytes)
	assert.NoError(t, err)

	// Wait for status to be written in Status KV
	statusKey := "nodes.node-1.status.TestResource.my-test-res"
	var entry jetstream.KeyValueEntry
	for i := 0; i < 20; i++ {
		entry, err = statusKV.Get(ctx, statusKey)
		if err == nil {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	assert.NoError(t, err, "Status should be published in KV store")
	var statusObj struct {
		Status struct {
			Phase     string `json:"phase"`
			Compliant bool   `json:"compliant"`
		} `json:"status"`
	}
	err = json.Unmarshal(entry.Value(), &statusObj)
	assert.NoError(t, err)
	assert.Equal(t, "Succeeded", statusObj.Status.Phase)
	assert.True(t, statusObj.Status.Compliant)
}
