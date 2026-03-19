package server

import (
	"context"
	"fmt"
	"log/slog"

	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/credentials"
	"github.com/nats-io/nats.go"
	"github.com/guilledipa/praetor/pkg/storage"
	pb "github.com/guilledipa/praetor/proto/gen/master"
)

type OperatorServer struct {
	pb.UnimplementedOperatorServer
	Store  storage.Provider
	Nc     *nats.Conn
	Logger *slog.Logger
}

func NewOperatorServer(store storage.Provider, nc *nats.Conn, logger *slog.Logger) *OperatorServer {
	return &OperatorServer{
		Store:  store,
		Nc:     nc,
		Logger: logger,
	}
}

func (s *OperatorServer) ListAgents(ctx context.Context, req *pb.ListAgentsRequest) (*pb.ListAgentsResponse, error) {
	agents, err := s.Store.ListAgents(ctx)
	if err != nil {
		s.Logger.Error("Failed to list agents", "error", err)
		return nil, err
	}

	var summaries []*pb.AgentSummary
	for _, a := range agents {
		// Provide a basic summary without fetching full reports for list optimization
		summaries = append(summaries, &pb.AgentSummary{
			NodeId: a,
		})
	}

	return &pb.ListAgentsResponse{
		Agents: summaries,
	}, nil
}

func (s *OperatorServer) GetAgentStatus(ctx context.Context, req *pb.AgentStatusRequest) (*pb.AgentStatusResponse, error) {
	reports, err := s.Store.GetAgentReports(ctx, req.NodeId)
	if err != nil {
		s.Logger.Error("Failed to get agent reports", "node", req.NodeId, "error", err)
		return nil, err
	}

	// Calculate overall compliance
	compliant := true
	for _, r := range reports {
		if !r.Compliant {
			compliant = false
			break
		}
	}

	return &pb.AgentStatusResponse{
		NodeId:      req.NodeId,
		IsCompliant: compliant,
		Resources:   reports,
	}, nil
}

func (s *OperatorServer) TriggerSync(ctx context.Context, req *pb.TriggerSyncRequest) (*pb.TriggerSyncResponse, error) {
	// Extract caller's TLS identity
	operatorID := "unknown"
	if p, ok := peer.FromContext(ctx); ok {
		if tlsInfo, ok := p.AuthInfo.(credentials.TLSInfo); ok {
			if len(tlsInfo.State.PeerCertificates) > 0 {
				operatorID = tlsInfo.State.PeerCertificates[0].Subject.CommonName
			}
		}
	}

	// Persist the request to the immutable JetStream Audit ledger
	err := s.Store.StoreAuditLog(ctx, "TRIGGER_SYNC", req.NodeId, operatorID)
	if err != nil {
		s.Logger.Error("Failed to store audit log", "error", err)
		return nil, err
	}

	// Awaken the agent synchronously
	subject := fmt.Sprintf("agent.trigger.getCatalog.%s", req.NodeId)
	if err := s.Nc.Publish(subject, []byte("Ad-Hoc Sync Initiated")); err != nil {
		return nil, err
	}

	s.Logger.Info("Ad-Hoc Orchestration Authorized", "operator", operatorID, "target", req.NodeId, "action", "TRIGGER_SYNC")

	return &pb.TriggerSyncResponse{
		Success: true,
		Message: fmt.Sprintf("Trigger broadcasted safely by %s", operatorID),
	}, nil
}
