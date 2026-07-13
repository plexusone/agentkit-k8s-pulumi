package kubernetes

import "github.com/plexusone/agentkit/deploy"

func init() {
	// Register the Kubernetes provider with priority 50.
	// This is lower than cloud-specific providers (100) so they take precedence
	// when explicitly specified, but K8s is preferred for generic deployments.
	deploy.RegisterProvider(deploy.ProviderKubernetes, newProvider, 50)
}

// newProvider is the factory function for creating Kubernetes providers.
func newProvider(cfg *deploy.DeployConfig) (deploy.Provider, error) {
	return New(cfg)
}
