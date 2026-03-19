package secrets

// Provider defines the interface for fetching secrets.
// This interface is intentionally modeled after Kubernetes `v1.Secret` retrieval
// so that a `KubeProvider` can trivially be written in the future to drop-in replace
// the local file backend.
type Provider interface {
	GetSecret(namespace, name, key string) (string, error)
}
