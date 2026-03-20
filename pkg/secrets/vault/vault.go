package vault

import (
	"context"
	"fmt"

	vaultapi "github.com/hashicorp/vault/api"
)

// Provider implements secrets.Provider using HashiCorp Vault.
type Provider struct {
	client *vaultapi.Client
}

// NewProvider initializes a new Vault secrets provider.
// It relies on standard Vault environment variables (VAULT_ADDR, VAULT_TOKEN)
// to automatically configure the client.
func NewProvider() (*Provider, error) {
	config := vaultapi.DefaultConfig()
	if err := config.ReadEnvironment(); err != nil {
		return nil, fmt.Errorf("failed to read vault environment: %w", err)
	}

	client, err := vaultapi.NewClient(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create vault client: %w", err)
	}

	return &Provider{
		client: client,
	}, nil
}

// GetSecret fetches a secret from Vault.
// In Vault, secrets are often structured as kv-v2 engines.
// We map `namespace/name` to a generic `secret/data/{namespace}/{name}` path.
func (p *Provider) GetSecret(namespace, name, key string) (string, error) {
	// Note: We use the generic KV v2 pathing.
	// You might want to make the mount path ("secret") configurable later.
	path := fmt.Sprintf("secret/data/%s/%s", namespace, name)

	secret, err := p.client.Logical().ReadWithContext(context.Background(), path)
	if err != nil {
		return "", fmt.Errorf("failed to read vault path %s: %w", path, err)
	}
	if secret == nil || secret.Data == nil {
		return "", fmt.Errorf("secret not found or no data at path %s", path)
	}

	dataMap, ok := secret.Data["data"].(map[string]interface{})
	if !ok {
		return "", fmt.Errorf("vault secret path %s does not contain a nested 'data' block (ensure you are using KV-v2)", path)
	}

	val, ok := dataMap[key]
	if !ok {
		return "", fmt.Errorf("key '%s' not found within secret data at path %s", key, path)
	}

	strVal, ok := val.(string)
	if !ok {
		return "", fmt.Errorf("value for key '%s' was not a string", key)
	}

	return strVal, nil
}
