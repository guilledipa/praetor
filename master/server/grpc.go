package server

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/guilledipa/praetor/master/catalog"
	"github.com/guilledipa/praetor/master/classifier"
	"github.com/guilledipa/praetor/pkg/secrets"
	"github.com/guilledipa/praetor/pkg/storage"
	pb "github.com/guilledipa/praetor/proto/gen/master"
	"go.opentelemetry.io/otel"
)

// Server implements master.ConfigurationMasterServer
type Server struct {
	pb.UnimplementedConfigurationMasterServer
	SigningKey  ed25519.PrivateKey
	SecretProv  secrets.Provider
	Store       storage.Provider
	Classifier  classifier.Classifier
	Logger      *slog.Logger
}

func NewServer(signingKey ed25519.PrivateKey, secretProv secrets.Provider, store storage.Provider, cls classifier.Classifier, logger *slog.Logger) *Server {
	return &Server{
		SigningKey: signingKey,
		SecretProv: secretProv,
		Store:      store,
		Classifier: cls,
		Logger:     logger,
	}
}

// GetCatalog implements master.ConfigurationMasterServer
func (s *Server) GetCatalog(ctx context.Context, in *pb.GetCatalogRequest) (*pb.GetCatalogResponse, error) {
	ctx, span := otel.Tracer("master-server").Start(ctx, "GetCatalog")
	defer span.End()

	nodeID := in.GetNodeId()
	receivedFacts := in.GetFacts()
	reqLogger := s.Logger.With("node_id", nodeID)
	reqLogger.Info("Received GetCatalog request", "facts", receivedFacts)

	// 1. Evaluate Classifications
	var rawResources []json.RawMessage
	var err error
	func() {
		defer func() {
			if r := recover(); r != nil {
				err = fmt.Errorf("classifier evaluation panicked: %v", r)
			}
		}()
		rawResources, err = s.Classifier.Evaluate(nodeID, receivedFacts)
	}()

	if err != nil {
		reqLogger.Error("Error evaluating classifier", "error", err)
		return nil, fmt.Errorf("classifier evaluation failed: %w", err)
	}

	// 2. Hydrate catalog with secrets and facts
	hydratedResources, err := catalog.HydrateCatalog(rawResources, receivedFacts, s.SecretProv)
	if err != nil {
		s.Logger.Error("Failed to hydrate catalog", "error", err)
		return nil, fmt.Errorf("failed to hydrate catalog: %w", err)
	}

	// Create the final catalog structure with hydrated resources
	finalCatalog := struct {
		APIVersion string         `json:"apiVersion"`
		Kind       string         `json:"kind"`
		Metadata   map[string]any `json:"metadata"`
		Spec       struct {
			Resources []any `json:"resources"`
		} `json:"spec"`
	}{
		APIVersion: "praetor.io/v1alpha1",
		Kind:       "Catalog",
		Metadata:   map[string]any{"name": "compiled-catalog"},
		Spec: struct {
			Resources []any `json:"resources"`
		}{Resources: hydratedResources},
	}

	// Convert Go struct to JSON
	jsonBytes, err := json.Marshal(finalCatalog)
	if err != nil {
		reqLogger.Error("Error marshalling to JSON", "error", err)
		return nil, fmt.Errorf("failed to marshal catalog to JSON: %w", err)
	}
	catalogContent := string(jsonBytes)

	// Sign the JSON content
	signature := ed25519.Sign(s.SigningKey, []byte(catalogContent))
	reqLogger.Info("Catalog signed", "algorithm", "ed25519")

	return &pb.GetCatalogResponse{
		Catalog: &pb.Catalog{
			Content: catalogContent,
			Format:  "json",
		},
		Signature:          signature,
		SignatureAlgorithm: "ed25519",
	}, nil
}

// ReportState implements master.ConfigurationMasterServer
func (s *Server) ReportState(ctx context.Context, req *pb.ReportStateRequest) (*pb.ReportStateResponse, error) {
	s.Logger.Info("Received state report from agent", "node_id", req.NodeId, "resource_count", len(req.Resources))

	for _, rep := range req.Resources {
		storeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		err := s.Store.StoreReport(storeCtx, req.NodeId, rep)
		cancel()
		if err != nil {
			s.Logger.Error("Failed to persist report state into JetStream", "node_id", req.NodeId, "resource_id", rep.GetId(), "error", err)
		} else {
			s.Logger.Debug("Resource run persisted", "node_id", req.NodeId, "type", rep.GetType(), "id", rep.GetId(), "compliant", rep.GetCompliant())
		}
	}

	return &pb.ReportStateResponse{Acknowledged: true}, nil
}
