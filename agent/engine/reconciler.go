package engine

import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/guilledipa/praetor/agent/resources"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

type Reconciler struct {
	NodeID       string
	NatsConn     *nats.Conn
	MasterPubKey ed25519.PublicKey
	Logger       *slog.Logger
	DryRun       bool
}

func NewReconciler(nodeID string, nc *nats.Conn, pubKey ed25519.PublicKey, logger *slog.Logger, dryRun bool) *Reconciler {
	return &Reconciler{
		NodeID:       nodeID,
		NatsConn:     nc,
		MasterPubKey: pubKey,
		Logger:       logger,
		DryRun:       dryRun,
	}
}

func (r *Reconciler) Start(ctx context.Context) error {
	js, err := jetstream.New(r.NatsConn)
	if err != nil {
		return fmt.Errorf("failed to load jetstream: %w", err)
	}

	kv, err := js.KeyValue(ctx, "PRAETOR_SPECS")
	if err != nil {
		return fmt.Errorf("failed to bind specs KV: %w", err)
	}

	statusKV, err := js.KeyValue(ctx, "PRAETOR_STATUS")
	if err != nil {
		return fmt.Errorf("failed to bind status KV: %w", err)
	}

	watcher, err := kv.Watch(ctx, fmt.Sprintf("nodes.%s.spec.>", r.NodeID))
	if err != nil {
		return fmt.Errorf("failed to watch specs KV: %w", err)
	}

	r.Logger.Info("KRM Spec watcher started successfully", "node_id", r.NodeID)

	go func() {
		defer watcher.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case entry := <-watcher.Updates():
				if entry == nil {
					continue
				}
				if entry.Operation() == jetstream.KeyValuePut {
					r.Logger.Info("KRM Spec mutation detected", "key", entry.Key())
					r.reconcile(ctx, entry.Key(), entry.Value(), statusKV)
				}
			}
		}
	}()

	return nil
}

func (r *Reconciler) reconcile(ctx context.Context, key string, val []byte, statusKV jetstream.KeyValue) {
	var resMap map[string]any
	if err := json.Unmarshal(val, &resMap); err != nil {
		r.Logger.Error("Failed to parse resource JSON from KV", "key", key, "error", err)
		return
	}

	kind, _ := resMap["kind"].(string)
	metadata, _ := resMap["metadata"].(map[string]any)
	name, _ := metadata["name"].(string)

	if kind == "" || name == "" {
		r.Logger.Error("Invalid resource structure in KV: kind or name is empty", "key", key)
		return
	}

	annotations, _ := metadata["annotations"].(map[string]any)
	sigHex, _ := annotations["praetor.io/signature"].(string)

	statusKey := fmt.Sprintf("nodes.%s.status.%s.%s", sanitize(r.NodeID), sanitize(kind), sanitize(name))

	if sigHex == "" {
		r.Logger.Error("Resource lacks cryptographic signature", "kind", kind, "name", name)
		r.writeStatus(ctx, statusKV, statusKey, kind, name, false, "Resource lacks cryptographic signature")
		return
	}

	sigBytes, err := hex.DecodeString(sigHex)
	if err != nil {
		r.Logger.Error("Invalid hex signature format", "kind", kind, "name", name, "error", err)
		r.writeStatus(ctx, statusKV, statusKey, kind, name, false, "Invalid signature format")
		return
	}

	// Clean annotations map for canonical bytes representation
	cleanAnnotations := make(map[string]any)
	for k, v := range annotations {
		if k != "praetor.io/signature" {
			cleanAnnotations[k] = v
		}
	}
	metadata["annotations"] = cleanAnnotations

	cleanBytes, err := json.Marshal(resMap)
	if err != nil {
		r.Logger.Error("Failed to marshal canonical resource", "kind", kind, "name", name, "error", err)
		return
	}

	if !ed25519.Verify(r.MasterPubKey, cleanBytes, sigBytes) {
		r.Logger.Error("Cryptographic signature verification failed for resource", "kind", kind, "name", name)
		r.writeStatus(ctx, statusKV, statusKey, kind, name, false, "Cryptographic signature verification failed")
		return
	}

	r.Logger.Info("Resource cryptographic signature verified successfully", "kind", kind, "name", name)

	res, err := resources.CreateResource(kind, val)
	if err != nil {
		r.Logger.Error("Failed to create resource instance", "kind", kind, "name", name, "error", err)
		r.writeStatus(ctx, statusKV, statusKey, kind, name, false, fmt.Sprintf("Instantiation failed: %v", err))
		return
	}

	currentState, err := res.Get()
	if err != nil {
		r.Logger.Error("Failed to get resource state", "kind", kind, "name", name, "error", err)
		r.writeStatus(ctx, statusKV, statusKey, kind, name, false, fmt.Sprintf("Get state failed: %v", err))
		return
	}

	compliant, err := res.Test(currentState)
	if err != nil {
		r.Logger.Error("Failed to test resource compliance", "kind", kind, "name", name, "error", err)
		r.writeStatus(ctx, statusKV, statusKey, kind, name, false, fmt.Sprintf("Test compliance failed: %v", err))
		return
	}

	if !compliant {
		if r.DryRun {
			r.Logger.Info("[SIMULATION] ~ Drift detected: skipping enforcement due to dry-run mode", "kind", kind, "name", name)
			r.writeStatus(ctx, statusKV, statusKey, kind, name, false, "Drift detected (Simulation / Dry-Run)")
			return
		}

		r.Logger.Info("Drift detected. Enforcing desired state...", "kind", kind, "name", name)
		if err := res.Set(); err != nil {
			r.Logger.Error("Enforcement failed", "kind", kind, "name", name, "error", err)
			r.writeStatus(ctx, statusKV, statusKey, kind, name, false, fmt.Sprintf("Enforcement failed: %v", err))
			return
		}

		r.Logger.Info("Successfully enforced state", "kind", kind, "name", name)
		r.writeStatus(ctx, statusKV, statusKey, kind, name, true, "Drift detected but successfully enforced state")
		return
	}

	r.Logger.Info("Resource is compliant", "kind", kind, "name", name)
	r.writeStatus(ctx, statusKV, statusKey, kind, name, true, "Resource is compliant")
}

func (r *Reconciler) writeStatus(ctx context.Context, statusKV jetstream.KeyValue, key, kind, name string, compliant bool, message string) {
	phase := "Succeeded"
	if !compliant {
		phase = "Failed"
	}
	statusObj := map[string]any{
		"apiVersion": "praetor.io/v1alpha1",
		"kind":       "ResourceStatus",
		"metadata": map[string]any{
			"name": name,
		},
		"status": map[string]any{
			"phase":     phase,
			"compliant": compliant,
			"message":   message,
		},
	}
	data, _ := json.Marshal(statusObj)
	_, err := statusKV.Put(ctx, key, data)
	if err != nil {
		r.Logger.Error("Failed to publish resource status to KV", "key", key, "error", err)
	}
}

func sanitize(input string) string {
	clean := strings.ReplaceAll(input, "/", "_")
	clean = strings.ReplaceAll(clean, " ", "_")
	return clean
}
