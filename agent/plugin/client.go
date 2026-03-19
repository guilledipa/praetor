package plugin

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	agentResources "github.com/guilledipa/praetor/agent/resources"
	shared "github.com/guilledipa/praetor/pkg/plugin"
	hplugin "github.com/hashicorp/go-plugin"
)

var (
	pluginClients = make(map[string]*hplugin.Client)
	resourceCache = make(map[string]shared.ResourceProvider)
)

// InitPlugins discovers and boots all binaries residing in /opt/praetor/plugins
// or a local ./plugins directory for testing.
func InitPlugins(logger *slog.Logger) error {
	pluginDirs := []string{"./plugins/bin", "/opt/praetor/plugins"}
	
	// Ensure cleanup on shutdown
	hplugin.CleanupClients()

	for _, dir := range pluginDirs {
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			continue
		}

		entries, err := os.ReadDir(dir)
		if err != nil {
			logger.Error("Failed to read plugin directory", "dir", dir, "error", err)
			continue
		}

		for _, e := range entries {
			if e.IsDir() || !strings.HasPrefix(e.Name(), "praetor-plugin-") {
				continue
			}

			pluginPath := filepath.Join(dir, e.Name())
			logger.Info("Booting external RPC plugin", "binary", pluginPath)

			client := hplugin.NewClient(&hplugin.ClientConfig{
				HandshakeConfig:  shared.HandshakeContext,
				Plugins: map[string]hplugin.Plugin{
					"resource": &shared.ResourcePlugin{},
				},
				Cmd:              exec.Command(pluginPath),
				AllowedProtocols: []hplugin.Protocol{hplugin.ProtocolGRPC},
			})

			rpcClient, err := client.Client()
			if err != nil {
				client.Kill()
				logger.Error("Failed to handshake plugin", "plugin", e.Name(), "error", err)
				continue
			}

			raw, err := rpcClient.Dispense("resource")
			if err != nil {
				client.Kill()
				logger.Error("Failed to dispense plugin", "plugin", e.Name(), "error", err)
				continue
			}

			provider := raw.(shared.ResourceProvider)
			pluginClients[e.Name()] = client

			// Register natively mapped resources discovered over UDS
			kinds, err := provider.Capabilities(context.Background())
			if err != nil {
				logger.Error("Failed to fetch capabilities", "plugin", e.Name(), "error", err)
				continue
			}

			// For every Kind discovered (User, package, file), dynamically bind a proxy factory!
			for _, kind := range kinds {
				logger.Info("Plugin dynamically registered kind", "plugin", e.Name(), "kind", kind)
				k := kind
				agentResources.RegisterType(k, func(spec json.RawMessage) (agentResources.Resource, error) {
					// Extract Name
					var meta struct {
						Metadata map[string]string `json:"metadata"`
						Spec     struct {
							Requires []agentResources.Dependency `json:"require"`
							Before   []agentResources.Dependency `json:"before"`
						} `json:"spec"`
					}
					if err := json.Unmarshal(spec, &meta); err != nil {
						return nil, fmt.Errorf("failed to extract metadata for kind %s", k)
					}
					
					return &PluginProxy{
						kind:     k,
						id:       meta.Metadata["name"],
						spec:     spec,
						provider: provider,
						requires: meta.Spec.Requires,
						before:   meta.Spec.Before,
					}, nil
				})
			}
		}
	}
	return nil
}

type PluginProxy struct {
	kind     string
	id       string
	spec     []byte
	provider shared.ResourceProvider
	requires []agentResources.Dependency
	before   []agentResources.Dependency
}

func (p *PluginProxy) Type() string { return p.kind }
func (p *PluginProxy) ID() string   { return p.id }

func (p *PluginProxy) Requires() []agentResources.Dependency { return p.requires }
func (p *PluginProxy) Before() []agentResources.Dependency   { return p.before }

func (p *PluginProxy) Get() (agentResources.State, error) {
	b, err := p.provider.Get(context.Background(), p.kind, p.spec)
	if err != nil {
		return nil, err
	}
	var state agentResources.State
	if err := json.Unmarshal(b, &state); err != nil {
		return nil, err
	}
	return state, nil
}

func (p *PluginProxy) Test(currentState agentResources.State) (bool, error) {
	b, _ := json.Marshal(currentState)
	return p.provider.Test(context.Background(), p.kind, p.spec, b)
}

func (p *PluginProxy) Set() error {
	return p.provider.Set(context.Background(), p.kind, p.spec)
}

func Cleanup() {
	hplugin.CleanupClients()
}
