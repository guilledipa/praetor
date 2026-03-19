package server

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"log/slog"
	"os"
	"testing"

	pb "github.com/guilledipa/praetor/proto/gen/master"
)

type mockStore struct{
	savedReports []*pb.ResourceReport
}

func (m *mockStore) StoreReport(ctx context.Context, nodeID string, report *pb.ResourceReport) error {
	m.savedReports = append(m.savedReports, report)
	return nil
}

func (m *mockStore) ListAgents(ctx context.Context) ([]string, error) {
	return []string{}, nil
}

func (m *mockStore) GetAgentReports(ctx context.Context, nodeID string) ([]*pb.ResourceReport, error) {
	return []*pb.ResourceReport{}, nil
}

func (m *mockStore) StoreAuditLog(ctx context.Context, action string, targetNode string, operatorID string) error {
	return nil
}

type mockClassifier struct{}

func (m *mockClassifier) Evaluate(nodeID string, facts map[string]string) ([]json.RawMessage, error) {
	return []json.RawMessage{[]byte(`{"test":"true"}`)}, nil
}

type mockSecretProvider struct{}

func (m *mockSecretProvider) GetSecret(namespace, name, key string) (string, error) { return "test-secret", nil }

func TestServerGetCatalogAndReport(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	pubKey, privKey, _ := ed25519.GenerateKey(rand.Reader)

	s := NewServer(privKey, &mockSecretProvider{}, &mockStore{}, &mockClassifier{}, logger)
	ctx := context.Background()

	resp, err := s.GetCatalog(ctx, &pb.GetCatalogRequest{NodeId: "node-123"})
	if err != nil {
		t.Fatalf("GetCatalog failed: %v", err)
	}

	if !ed25519.Verify(pubKey, []byte(resp.Catalog.Content), resp.Signature) {
		t.Fatalf("Signature verification failed")
	}

	repResp, err := s.ReportState(ctx, &pb.ReportStateRequest{NodeId: "node-123", Resources: []*pb.ResourceReport{{Id: "test-res"}}})
	if err != nil || !repResp.Acknowledged {
		t.Fatalf("ReportState failed")
	}
}
