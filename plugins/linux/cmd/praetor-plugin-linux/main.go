package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/guilledipa/praetor/agent/resources"
	"github.com/guilledipa/praetor/pkg/plugin"
	hplugin "github.com/hashicorp/go-plugin"
	_ "github.com/guilledipa/praetor/plugins/linux/resources/cron"
	_ "github.com/guilledipa/praetor/plugins/linux/resources/exec"
	_ "github.com/guilledipa/praetor/plugins/linux/resources/file"
	_ "github.com/guilledipa/praetor/plugins/linux/resources/group"
	_ "github.com/guilledipa/praetor/plugins/linux/resources/pkg"
	_ "github.com/guilledipa/praetor/plugins/linux/resources/svc"
	_ "github.com/guilledipa/praetor/plugins/linux/resources/user"
)

// LinuxProvider is the aggregator mapping all native linux modules
type LinuxProvider struct{}

func (l *LinuxProvider) Capabilities(ctx context.Context) ([]string, error) {
	// The native registry holds the string kinds it supports
	registry := resources.GetRegistry()
	var kinds []string
	for k := range registry {
		kinds = append(kinds, k)
	}
	return kinds, nil
}

func (l *LinuxProvider) Get(ctx context.Context, kind string, spec []byte) ([]byte, error) {
	registry := resources.GetRegistry()
	factory, ok := registry[kind]
	if !ok {
		return nil, fmt.Errorf("unsupported resource kind: %s", kind)
	}
	res, err := factory(spec)
	if err != nil {
		return nil, err
	}
	state, err := res.Get()
	if err != nil {
		return nil, err
	}
	return json.Marshal(state)
}

func (l *LinuxProvider) Test(ctx context.Context, kind string, spec []byte, state []byte) (bool, error) {
	registry := resources.GetRegistry()
	factory, ok := registry[kind]
	if !ok {
		return false, fmt.Errorf("unsupported resource kind: %s", kind)
	}
	res, err := factory(spec)
	if err != nil {
		return false, err
	}

	var parsedState resources.State
	if err := json.Unmarshal(state, &parsedState); err != nil {
		return false, err
	}

	return res.Test(parsedState)
}

func (l *LinuxProvider) Set(ctx context.Context, kind string, spec []byte) error {
	registry := resources.GetRegistry()
	factory, ok := registry[kind]
	if !ok {
		return fmt.Errorf("unsupported resource kind: %s", kind)
	}
	res, err := factory(spec)
	if err != nil {
		return err
	}
	return res.Set()
}

func main() {
	log.Println("Booting praetor-plugin-linux over UDS...")
	
	hplugin.Serve(&hplugin.ServeConfig{
		HandshakeConfig: plugin.HandshakeContext,
		Plugins: map[string]hplugin.Plugin{
			"resource": &plugin.ResourcePlugin{Impl: &LinuxProvider{}},
		},
		GRPCServer: hplugin.DefaultGRPCServer,
	})
}
