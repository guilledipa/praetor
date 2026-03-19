package local

import (
	"fmt"
	"gopkg.in/yaml.v3"
	"os"
)

// Provider implements secrets.Provider reading from a local YAML file.
type Provider struct {
	// Nested map: map[namespace]map[name]map[key]value
	secrets map[string]map[string]map[string]string
}

// NewProvider loads the mock secrets YAML from the given path.
func NewProvider(path string) (*Provider, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read secrets file: %w", err)
	}

	var parsed map[string]map[string]map[string]string
	if err := yaml.Unmarshal(data, &parsed); err != nil {
		return nil, fmt.Errorf("failed to unmarshal secrets yaml: %w", err)
	}

	return &Provider{secrets: parsed}, nil
}

// GetSecret fetches a specific secret value mirroring K8s layout
func (p *Provider) GetSecret(namespace, name, key string) (string, error) {
	ns, exists := p.secrets[namespace]
	if !exists {
		return "", fmt.Errorf("namespace '%s' not found", namespace)
	}

	secret, exists := ns[name]
	if !exists {
		return "", fmt.Errorf("secret '%s' not found in namespace '%s'", name, namespace)
	}

	val, exists := secret[key]
	if !exists {
		return "", fmt.Errorf("key '%s' not found in secret '%s'", key, name)
	}

	return val, nil
}
