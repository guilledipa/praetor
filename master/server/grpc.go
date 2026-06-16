package server

import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
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

func (s *Server) signAndPersistCatalog(ctx context.Context, nodeID string, resourcesList []any) ([]any, error) {
	signedList := make([]any, 0, len(resourcesList))

	for i, res := range resourcesList {
		resBytes, err := json.Marshal(res)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal resource %d: %w", i, err)
		}
		var resMap map[string]any
		if err := json.Unmarshal(resBytes, &resMap); err != nil {
			return nil, fmt.Errorf("failed to unmarshal resource %d to map: %w", i, err)
		}

		metadata, ok := resMap["metadata"].(map[string]any)
		if !ok {
			metadata = make(map[string]any)
			resMap["metadata"] = metadata
		}

		annotations, ok := metadata["annotations"].(map[string]any)
		if !ok {
			annotations = make(map[string]any)
			metadata["annotations"] = annotations
		}

		delete(annotations, "praetor.io/signature")

		cleanBytes, err := json.Marshal(resMap)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal clean resource %d: %w", i, err)
		}

		sig := ed25519.Sign(s.SigningKey, cleanBytes)
		sigHex := hex.EncodeToString(sig)

		annotations["praetor.io/signature"] = sigHex

		finalBytes, err := json.Marshal(resMap)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal final resource %d: %w", i, err)
		}

		if s.Store != nil {
			kind, _ := resMap["kind"].(string)
			name, _ := metadata["name"].(string)
			if kind != "" && name != "" {
				err = s.Store.StoreResourceSpec(ctx, nodeID, kind, name, finalBytes)
				if err != nil {
					s.Logger.Error("Failed to store resource spec in KV", "node_id", nodeID, "kind", kind, "name", name, "error", err)
				} else {
					s.Logger.Info("Persisted resource spec to NATS KV store", "node_id", nodeID, "kind", kind, "name", name)
				}
			}
		}

		signedList = append(signedList, resMap)
	}

	return signedList, nil
}

// CompileCatalog compiles and signs the configuration catalog for a given node.
func (s *Server) CompileCatalog(nodeID string, receivedFacts map[string]string) (string, []byte, error) {
	reqLogger := s.Logger.With("node_id", nodeID)

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
		return "", nil, fmt.Errorf("classifier evaluation failed: %w", err)
	}

	// 2. Hydrate catalog with secrets and facts
	hydratedResources, err := catalog.HydrateCatalog(rawResources, receivedFacts, s.SecretProv)
	if err != nil {
		s.Logger.Error("Failed to hydrate catalog", "error", err)
		return "", nil, fmt.Errorf("failed to hydrate catalog: %w", err)
	}

	// Sign each resource individually and store in NATS KV Spec store
	signedResources, err := s.signAndPersistCatalog(context.Background(), nodeID, hydratedResources)
	if err != nil {
		s.Logger.Error("Failed to sign and persist resources", "error", err)
		return "", nil, fmt.Errorf("failed to sign and persist resources: %w", err)
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
		}{Resources: signedResources},
	}

	// Convert Go struct to JSON
	jsonBytes, err := json.Marshal(finalCatalog)
	if err != nil {
		reqLogger.Error("Error marshalling to JSON", "error", err)
		return "", nil, fmt.Errorf("failed to marshal catalog to JSON: %w", err)
	}
	catalogContent := string(jsonBytes)

	// Sign the JSON content
	signature := ed25519.Sign(s.SigningKey, []byte(catalogContent))
	reqLogger.Info("Catalog signed", "algorithm", "ed25519")

	return catalogContent, signature, nil
}

// GetCatalog implements master.ConfigurationMasterServer
func (s *Server) GetCatalog(ctx context.Context, in *pb.GetCatalogRequest) (*pb.GetCatalogResponse, error) {
	ctx, span := otel.Tracer("master-server").Start(ctx, "GetCatalog")
	defer span.End()

	catalogContent, signature, err := s.CompileCatalog(in.GetNodeId(), in.GetFacts())
	if err != nil {
		return nil, err
	}

	return &pb.GetCatalogResponse{
		Catalog: &pb.Catalog{
			Content: catalogContent,
			Format:  "json",
		},
		Signature:          signature,
		SignatureAlgorithm: "ed25519",
	}, nil
}

// ProcessReport stores all resource reports in the state storage.
func (s *Server) ProcessReport(ctx context.Context, nodeID string, resources []*pb.ResourceReport, isCompliant bool, timestamp int64) error {
	s.Logger.Info("Processing state report from agent", "node_id", nodeID, "resource_count", len(resources))

	for _, rep := range resources {
		storeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		err := s.Store.StoreReport(storeCtx, nodeID, rep)
		cancel()
		if err != nil {
			s.Logger.Error("Failed to persist report state into JetStream", "node_id", nodeID, "resource_id", rep.GetId(), "error", err)
		} else {
			s.Logger.Debug("Resource run persisted", "node_id", nodeID, "type", rep.GetType(), "id", rep.GetId(), "compliant", rep.GetCompliant())
		}
	}
	return nil
}

// ReportState implements master.ConfigurationMasterServer
func (s *Server) ReportState(ctx context.Context, req *pb.ReportStateRequest) (*pb.ReportStateResponse, error) {
	err := s.ProcessReport(ctx, req.NodeId, req.Resources, req.IsCompliant, req.Timestamp)
	if err != nil {
		return nil, err
	}
	return &pb.ReportStateResponse{Acknowledged: true}, nil
}
