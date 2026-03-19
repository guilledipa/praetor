package engine

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"log/slog"
	"os"
	"testing"

	masterpb "github.com/guilledipa/praetor/proto/gen/master"
	"google.golang.org/grpc"
)

type mockMasterClient struct {
	pubKey  ed25519.PublicKey
	privKey ed25519.PrivateKey
	t       *testing.T
}

func (m *mockMasterClient) GetCatalog(ctx context.Context, in *masterpb.GetCatalogRequest, opts ...grpc.CallOption) (*masterpb.GetCatalogResponse, error) {
	catalogContent := `{"apiVersion":"praetor.io/v1alpha1","kind":"Catalog","metadata":{"name":"compiled-catalog"},"spec":{"resources":[]}}`
	signature := ed25519.Sign(m.privKey, []byte(catalogContent))

	return &masterpb.GetCatalogResponse{
		Catalog: &masterpb.Catalog{
			Content: catalogContent,
			Format:  "json",
		},
		Signature:          signature,
		SignatureAlgorithm: "ed25519",
	}, nil
}

func (m *mockMasterClient) ReportState(ctx context.Context, in *masterpb.ReportStateRequest, opts ...grpc.CallOption) (*masterpb.ReportStateResponse, error) {
	return &masterpb.ReportStateResponse{Acknowledged: true}, nil
}

func TestFetchAndApplyCatalog(t *testing.T) {
	pubKey, privKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("Failed to generate keys: %v", err)
	}

	mockClient := &mockMasterClient{
		pubKey:  pubKey,
		privKey: privKey,
		t:       t,
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	executor := &Executor{
		NodeID:       "test-node",
		MasterClient: mockClient,
		MasterPubKey: pubKey,
		Logger:       logger,
	}

	// This validates the engine graph execution loop resolves gracefully on empty valid catalog
	executor.FetchAndApplyCatalog()
}
