package engine

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"log/slog"
	"os"
	"testing"

	"github.com/guilledipa/praetor/agent/resources"
	pkgbroker "github.com/guilledipa/praetor/pkg/broker"
	masterpb "github.com/guilledipa/praetor/proto/gen/master"
	natsserverv2 "github.com/nats-io/nats-server/v2/server"
	natsserver "github.com/nats-io/nats-server/v2/test"
	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/assert"
)

type mockMasterClient struct {
	pubKey        ed25519.PublicKey
	privKey       ed25519.PrivateKey
	t             *testing.T
	customCatalog string
	lastReport    *masterpb.ReportStateRequest
}

func startTestNats(t *testing.T) (*nats.Conn, *natsserverv2.Server) {
	opts := natsserver.DefaultTestOptions
	opts.Port = -1
	s := natsserver.RunServer(&opts)

	nc, err := nats.Connect(s.ClientURL())
	if err != nil {
		s.Shutdown()
		t.Fatalf("failed to connect to test NATS: %v", err)
	}
	return nc, s
}

// TestResource implements resources.Resource for test verification
type TestResource struct {
	kind      string
	id        string
	compliant bool
	setCalled bool
}

func (r *TestResource) Type() string { return r.kind }
func (r *TestResource) ID() string   { return r.id }
func (r *TestResource) Get() (resources.State, error) {
	return resources.State{}, nil
}
func (r *TestResource) Test(currentState resources.State) (bool, error) {
	return r.compliant, nil
}
func (r *TestResource) Set() error {
	r.setCalled = true
	return nil
}
func (r *TestResource) Requires() []resources.Dependency { return nil }
func (r *TestResource) Before() []resources.Dependency   { return nil }

var globalTestResource *TestResource

func init() {
	resources.RegisterType("TestResource", func(spec json.RawMessage) (resources.Resource, error) {
		var meta struct {
			Metadata struct {
				Name string `json:"name"`
			} `json:"metadata"`
		}
		_ = json.Unmarshal(spec, &meta)
		globalTestResource = &TestResource{
			kind:      "TestResource",
			id:        meta.Metadata.Name,
			compliant: false,
		}
		return globalTestResource, nil
	})
}

func TestFetchAndApplyCatalog(t *testing.T) {
	pubKey, privKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("Failed to generate keys: %v", err)
	}

	nc, ns := startTestNats(t)
	defer ns.Shutdown()
	defer nc.Close()

	_, err = nc.Subscribe("agent.catalog.request.*", func(m *nats.Msg) {
		catalogContent := `{"apiVersion":"praetor.io/v1alpha1","kind":"Catalog","metadata":{"name":"compiled-catalog"},"spec":{"resources":[]}}`
		signature := ed25519.Sign(privKey, []byte(catalogContent))

		resp := pkgbroker.CatalogResponse{
			Catalog:            catalogContent,
			Signature:          signature,
			SignatureAlgorithm: "ed25519",
		}
		respBytes, _ := json.Marshal(resp)
		m.Respond(respBytes)
	})
	assert.NoError(t, err)

	_, err = nc.Subscribe("agent.state.report.*", func(m *nats.Msg) {
		resp := pkgbroker.StateReportResponse{Acknowledged: true}
		respBytes, _ := json.Marshal(resp)
		m.Respond(respBytes)
	})
	assert.NoError(t, err)

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	executor := &Executor{
		NodeID:       "test-node",
		NatsConn:     nc,
		MasterPubKey: pubKey,
		Logger:       logger,
	}

	// This validates the engine graph execution loop resolves gracefully on empty valid catalog
	executor.FetchAndApplyCatalog(context.Background())
}

func TestFetchAndApplyCatalog_DryRun(t *testing.T) {
	pubKey, privKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("Failed to generate keys: %v", err)
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	t.Run("with dry-run true", func(t *testing.T) {
		nc, ns := startTestNats(t)
		defer ns.Shutdown()
		defer nc.Close()

		mockClient := &mockMasterClient{
			pubKey:  pubKey,
			privKey: privKey,
			t:       t,
			customCatalog: `{
				"apiVersion": "praetor.io/v1alpha1",
				"kind": "Catalog",
				"metadata": { "name": "test-catalog" },
				"spec": {
					"resources": [
						{
							"apiVersion": "praetor.io/v1alpha1",
							"kind": "TestResource",
							"metadata": { "name": "my-test-res" }
						}
					]
				}
			}`,
		}

		_, err = nc.Subscribe("agent.catalog.request.*", func(m *nats.Msg) {
			catalogContent := mockClient.customCatalog
			signature := ed25519.Sign(privKey, []byte(catalogContent))

			resp := pkgbroker.CatalogResponse{
				Catalog:            catalogContent,
				Signature:          signature,
				SignatureAlgorithm: "ed25519",
			}
			respBytes, _ := json.Marshal(resp)
			m.Respond(respBytes)
		})
		assert.NoError(t, err)

		_, err = nc.Subscribe("agent.state.report.*", func(m *nats.Msg) {
			var req pkgbroker.StateReportRequest
			_ = json.Unmarshal(m.Data, &req)

			var pbResources []*masterpb.ResourceReport
			for _, r := range req.Resources {
				pbResources = append(pbResources, &masterpb.ResourceReport{
					Type:      r.Type,
					Id:        r.Id,
					Compliant: r.Compliant,
					Message:   r.Message,
				})
			}
			mockClient.lastReport = &masterpb.ReportStateRequest{
				NodeId:      req.NodeID,
				Resources:   pbResources,
				IsCompliant: req.IsCompliant,
				Timestamp:   req.Timestamp,
			}

			resp := pkgbroker.StateReportResponse{Acknowledged: true}
			respBytes, _ := json.Marshal(resp)
			m.Respond(respBytes)
		})
		assert.NoError(t, err)

		executor := &Executor{
			NodeID:       "test-node",
			NatsConn:     nc,
			MasterPubKey: pubKey,
			Logger:       logger,
			DryRun:       true,
		}

		executor.FetchAndApplyCatalog(context.Background())

		assert.NotNil(t, globalTestResource)
		assert.False(t, globalTestResource.setCalled, "Set should NOT be called in dry-run mode")
		assert.NotNil(t, mockClient.lastReport)
		assert.False(t, mockClient.lastReport.IsCompliant)
		assert.Len(t, mockClient.lastReport.Resources, 1)
		assert.False(t, mockClient.lastReport.Resources[0].Compliant)
		assert.Equal(t, "Drift detected (Simulation / Dry-Run)", mockClient.lastReport.Resources[0].Message)
	})

	t.Run("with dry-run false", func(t *testing.T) {
		nc, ns := startTestNats(t)
		defer ns.Shutdown()
		defer nc.Close()

		mockClient := &mockMasterClient{
			pubKey:  pubKey,
			privKey: privKey,
			t:       t,
			customCatalog: `{
				"apiVersion": "praetor.io/v1alpha1",
				"kind": "Catalog",
				"metadata": { "name": "test-catalog" },
				"spec": {
					"resources": [
						{
							"apiVersion": "praetor.io/v1alpha1",
							"kind": "TestResource",
							"metadata": { "name": "my-test-res-2" }
						}
					]
				}
			}`,
		}

		_, err = nc.Subscribe("agent.catalog.request.*", func(m *nats.Msg) {
			catalogContent := mockClient.customCatalog
			signature := ed25519.Sign(privKey, []byte(catalogContent))

			resp := pkgbroker.CatalogResponse{
				Catalog:            catalogContent,
				Signature:          signature,
				SignatureAlgorithm: "ed25519",
			}
			respBytes, _ := json.Marshal(resp)
			m.Respond(respBytes)
		})
		assert.NoError(t, err)

		_, err = nc.Subscribe("agent.state.report.*", func(m *nats.Msg) {
			var req pkgbroker.StateReportRequest
			_ = json.Unmarshal(m.Data, &req)

			var pbResources []*masterpb.ResourceReport
			for _, r := range req.Resources {
				pbResources = append(pbResources, &masterpb.ResourceReport{
					Type:      r.Type,
					Id:        r.Id,
					Compliant: r.Compliant,
					Message:   r.Message,
				})
			}
			mockClient.lastReport = &masterpb.ReportStateRequest{
				NodeId:      req.NodeID,
				Resources:   pbResources,
				IsCompliant: req.IsCompliant,
				Timestamp:   req.Timestamp,
			}

			resp := pkgbroker.StateReportResponse{Acknowledged: true}
			respBytes, _ := json.Marshal(resp)
			m.Respond(respBytes)
		})
		assert.NoError(t, err)

		executor := &Executor{
			NodeID:       "test-node",
			NatsConn:     nc,
			MasterPubKey: pubKey,
			Logger:       logger,
			DryRun:       false,
		}

		executor.FetchAndApplyCatalog(context.Background())

		assert.NotNil(t, globalTestResource)
		assert.True(t, globalTestResource.setCalled, "Set SHOULD be called when dry-run is false")
		assert.NotNil(t, mockClient.lastReport)
		assert.False(t, mockClient.lastReport.IsCompliant, "Should report non-compliant overall because drift was detected during the run")
		assert.Len(t, mockClient.lastReport.Resources, 1)
		assert.True(t, mockClient.lastReport.Resources[0].Compliant, "Individual resource should be compliant after successful enforcement")
		assert.Equal(t, "Drift detected but successfully enforced state", mockClient.lastReport.Resources[0].Message)
	})
}
