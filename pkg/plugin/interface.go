package plugin

import (
	"context"

	hplugin "github.com/hashicorp/go-plugin"
	"google.golang.org/grpc"

	pb "github.com/guilledipa/praetor/proto/gen/plugin"
)

// ResourceProvider is the core interface every plugin must implement internally.
type ResourceProvider interface {
	Capabilities(ctx context.Context) ([]string, error)
	Get(ctx context.Context, kind string, spec []byte) ([]byte, error)
	Test(ctx context.Context, kind string, spec []byte, state []byte) (bool, error)
	Set(ctx context.Context, kind string, spec []byte) error
}

// Map the Provider to the Protocol Buffers server interface.
type GRPCServer struct {
	Impl ResourceProvider
	pb.UnimplementedResourcePluginServer
}

func (m *GRPCServer) Capabilities(ctx context.Context, req *pb.CapabilitiesRequest) (*pb.CapabilitiesResponse, error) {
	kinds, err := m.Impl.Capabilities(ctx)
	if err != nil {
		return nil, err
	}
	return &pb.CapabilitiesResponse{Kinds: kinds}, nil
}

func (m *GRPCServer) Get(ctx context.Context, req *pb.GetRequest) (*pb.GetResponse, error) {
	stateBytes, err := m.Impl.Get(ctx, req.Kind, req.Spec)
	if err != nil {
		return &pb.GetResponse{Error: err.Error()}, nil
	}
	return &pb.GetResponse{State: stateBytes}, nil
}

func (m *GRPCServer) Test(ctx context.Context, req *pb.TestRequest) (*pb.TestResponse, error) {
	match, err := m.Impl.Test(ctx, req.Kind, req.Spec, req.State)
	if err != nil {
		return &pb.TestResponse{Error: err.Error()}, nil
	}
	return &pb.TestResponse{Match: match}, nil
}

func (m *GRPCServer) Set(ctx context.Context, req *pb.SetRequest) (*pb.SetResponse, error) {
	err := m.Impl.Set(ctx, req.Kind, req.Spec)
	if err != nil {
		return &pb.SetResponse{Error: err.Error()}, nil
	}
	return &pb.SetResponse{}, nil
}

// Map the Client to our local interface
type GRPCClient struct {
	client pb.ResourcePluginClient
}

func (m *GRPCClient) Capabilities(ctx context.Context) ([]string, error) {
	resp, err := m.client.Capabilities(ctx, &pb.CapabilitiesRequest{})
	if err != nil {
		return nil, err
	}
	return resp.Kinds, nil
}

func (m *GRPCClient) Get(ctx context.Context, kind string, spec []byte) ([]byte, error) {
	resp, err := m.client.Get(ctx, &pb.GetRequest{Kind: kind, Spec: spec})
	if err != nil {
		return nil, err
	}
	if resp.Error != "" {
		return nil, context.DeadlineExceeded // Actually return a wrapped custom err
	}
	return resp.State, nil
}

func (m *GRPCClient) Test(ctx context.Context, kind string, spec []byte, state []byte) (bool, error) {
	resp, err := m.client.Test(ctx, &pb.TestRequest{Kind: kind, Spec: spec, State: state})
	if err != nil {
		return false, err
	}
	if resp.Error != "" {
		return false, context.DeadlineExceeded // Wrapped error
	}
	return resp.Match, nil
}

func (m *GRPCClient) Set(ctx context.Context, kind string, spec []byte) error {
	resp, err := m.client.Set(ctx, &pb.SetRequest{Kind: kind, Spec: spec})
	if err != nil {
		return err
	}
	if resp.Error != "" {
		return context.DeadlineExceeded // Wrapped error
	}
	return nil
}

// Plugin maps the Client/Server
type ResourcePlugin struct {
	hplugin.Plugin
	Impl ResourceProvider
}

func (p *ResourcePlugin) GRPCServer(broker *hplugin.GRPCBroker, s *grpc.Server) error {
	pb.RegisterResourcePluginServer(s, &GRPCServer{Impl: p.Impl})
	return nil
}

func (p *ResourcePlugin) GRPCClient(ctx context.Context, broker *hplugin.GRPCBroker, c *grpc.ClientConn) (interface{}, error) {
	return &GRPCClient{client: pb.NewResourcePluginClient(c)}, nil
}

// Internal definitions for the Magic Cookie
var HandshakeContext = hplugin.HandshakeConfig{
	ProtocolVersion:  1,
	MagicCookieKey:   "PRAETOR_PLUGIN",
	MagicCookieValue: "praetor-v1",
}
