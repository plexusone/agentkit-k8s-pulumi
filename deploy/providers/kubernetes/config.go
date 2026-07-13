// Package kubernetes provides a cloud-agnostic Kubernetes workload provider.
// It deploys containers to any Kubernetes cluster (EKS, GKE, AKS, on-prem).
package kubernetes

import (
	"fmt"

	"github.com/plexusone/agentkit/deploy"
)

// Config contains Kubernetes-specific deployment configuration.
type Config struct {
	// Kubeconfig is the path to kubeconfig file or the kubeconfig content.
	// If empty, uses default kubeconfig (~/.kube/config) or in-cluster config.
	Kubeconfig string `yaml:"kubeconfig" json:"kubeconfig"`

	// KubeconfigContent is the raw kubeconfig YAML content.
	// Takes precedence over Kubeconfig path if both are set.
	KubeconfigContent string `yaml:"kubeconfig_content" json:"kubeconfig_content"`

	// Context is the kubeconfig context to use. If empty, uses current context.
	Context string `yaml:"context" json:"context"`

	// Namespace is the Kubernetes namespace to deploy to.
	// Defaults to "default".
	Namespace string `yaml:"namespace" json:"namespace"`

	// CreateNamespace creates the namespace if it doesn't exist.
	CreateNamespace bool `yaml:"create_namespace" json:"create_namespace"`

	// ServiceType is the Kubernetes Service type.
	// Options: "ClusterIP", "NodePort", "LoadBalancer"
	// Defaults to "ClusterIP".
	ServiceType string `yaml:"service_type" json:"service_type"`

	// IngressEnabled creates an Ingress resource for external access.
	IngressEnabled bool `yaml:"ingress_enabled" json:"ingress_enabled"`

	// IngressClassName is the Ingress class (e.g., "nginx", "alb", "gce").
	IngressClassName string `yaml:"ingress_class_name" json:"ingress_class_name"`

	// IngressHost is the hostname for the Ingress rule.
	IngressHost string `yaml:"ingress_host" json:"ingress_host"`

	// IngressTLSEnabled enables TLS for the Ingress.
	IngressTLSEnabled bool `yaml:"ingress_tls_enabled" json:"ingress_tls_enabled"`

	// IngressTLSSecretName is the name of the TLS secret.
	IngressTLSSecretName string `yaml:"ingress_tls_secret_name" json:"ingress_tls_secret_name"`

	// Replicas is the number of pod replicas.
	// Defaults to 1.
	Replicas int `yaml:"replicas" json:"replicas"`

	// ImagePullPolicy is the container image pull policy.
	// Options: "Always", "IfNotPresent", "Never"
	// Defaults to "IfNotPresent".
	ImagePullPolicy string `yaml:"image_pull_policy" json:"image_pull_policy"`

	// ImagePullSecrets is a list of secret names for private registries.
	ImagePullSecrets []string `yaml:"image_pull_secrets" json:"image_pull_secrets"`

	// ServiceAccountName is the service account to use for pods.
	ServiceAccountName string `yaml:"service_account_name" json:"service_account_name"`

	// NodeSelector is a map of node labels for pod scheduling.
	NodeSelector map[string]string `yaml:"node_selector" json:"node_selector"`

	// Tolerations for pod scheduling.
	Tolerations []Toleration `yaml:"tolerations" json:"tolerations"`

	// PodAnnotations are additional annotations for pods.
	PodAnnotations map[string]string `yaml:"pod_annotations" json:"pod_annotations"`

	// PodLabels are additional labels for pods.
	PodLabels map[string]string `yaml:"pod_labels" json:"pod_labels"`

	// LivenessProbe configures the liveness probe.
	LivenessProbe *ProbeConfig `yaml:"liveness_probe" json:"liveness_probe"`

	// ReadinessProbe configures the readiness probe.
	ReadinessProbe *ProbeConfig `yaml:"readiness_probe" json:"readiness_probe"`

	// ResourceRequests specifies resource requests (separate from limits).
	ResourceRequests *ResourceSpec `yaml:"resource_requests" json:"resource_requests"`
}

// Toleration represents a Kubernetes toleration.
type Toleration struct {
	Key      string `yaml:"key" json:"key"`
	Operator string `yaml:"operator" json:"operator"`
	Value    string `yaml:"value" json:"value"`
	Effect   string `yaml:"effect" json:"effect"`
}

// ProbeConfig configures a liveness or readiness probe.
type ProbeConfig struct {
	HTTPPath            string `yaml:"http_path" json:"http_path"`
	HTTPPort            int    `yaml:"http_port" json:"http_port"`
	InitialDelaySeconds int    `yaml:"initial_delay_seconds" json:"initial_delay_seconds"`
	PeriodSeconds       int    `yaml:"period_seconds" json:"period_seconds"`
	TimeoutSeconds      int    `yaml:"timeout_seconds" json:"timeout_seconds"`
	FailureThreshold    int    `yaml:"failure_threshold" json:"failure_threshold"`
	SuccessThreshold    int    `yaml:"success_threshold" json:"success_threshold"`
}

// ResourceSpec specifies CPU and memory.
type ResourceSpec struct {
	CPU    string `yaml:"cpu" json:"cpu"`
	Memory string `yaml:"memory" json:"memory"`
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() *Config {
	return &Config{
		Namespace:       "default",
		ServiceType:     "ClusterIP",
		Replicas:        1,
		ImagePullPolicy: "IfNotPresent",
	}
}

// Validate checks the configuration for errors.
func (c *Config) Validate() error {
	if c.Replicas < 0 {
		return fmt.Errorf("replicas must be >= 0, got %d", c.Replicas)
	}

	validServiceTypes := map[string]bool{
		"ClusterIP":    true,
		"NodePort":     true,
		"LoadBalancer": true,
		"":             true, // defaults to ClusterIP
	}
	if !validServiceTypes[c.ServiceType] {
		return fmt.Errorf("invalid service_type: %q", c.ServiceType)
	}

	validPullPolicies := map[string]bool{
		"Always":       true,
		"IfNotPresent": true,
		"Never":        true,
		"":             true, // defaults to IfNotPresent
	}
	if !validPullPolicies[c.ImagePullPolicy] {
		return fmt.Errorf("invalid image_pull_policy: %q", c.ImagePullPolicy)
	}

	if c.IngressEnabled && c.IngressHost == "" {
		return fmt.Errorf("ingress_host is required when ingress_enabled is true")
	}

	return nil
}

// ApplyDefaults fills in default values for unset fields.
func (c *Config) ApplyDefaults() {
	if c.Namespace == "" {
		c.Namespace = "default"
	}
	if c.ServiceType == "" {
		c.ServiceType = "ClusterIP"
	}
	if c.Replicas == 0 {
		c.Replicas = 1
	}
	if c.ImagePullPolicy == "" {
		c.ImagePullPolicy = "IfNotPresent"
	}
}

// GetConfig extracts and validates the Kubernetes config from DeployConfig.
func GetConfig(cfg *deploy.DeployConfig) (*Config, error) {
	k8sCfg := DefaultConfig()

	if cfg.ProviderConfig != nil {
		switch c := cfg.ProviderConfig.(type) {
		case *Config:
			k8sCfg = c
		case Config:
			k8sCfg = &c
		default:
			return nil, fmt.Errorf("invalid provider config type: %T", cfg.ProviderConfig)
		}
	}

	k8sCfg.ApplyDefaults()

	if err := k8sCfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid kubernetes config: %w", err)
	}

	return k8sCfg, nil
}
