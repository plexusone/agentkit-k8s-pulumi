package kubernetes

import (
	"context"
	"fmt"
	"time"

	"github.com/plexusone/agentkit/deploy"
	appsv1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/apps/v1"
	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	networkingv1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/networking/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	pulumiutil "github.com/plexusone/agentkit-k8s-pulumi/deploy/pulumi"
)

// Provider implements deploy.Provider for Kubernetes workloads.
// It deploys containers to any Kubernetes cluster (EKS, GKE, AKS, on-prem).
type Provider struct {
	cfg    *deploy.DeployConfig
	k8sCfg *Config
	closed bool
}

// Ensure Provider implements deploy.Provider.
var _ deploy.Provider = (*Provider)(nil)

// New creates a new Kubernetes provider.
func New(cfg *deploy.DeployConfig) (*Provider, error) {
	k8sCfg, err := GetConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("kubernetes: %w", err)
	}

	return &Provider{
		cfg:    cfg,
		k8sCfg: k8sCfg,
	}, nil
}

// Name returns the provider name.
func (p *Provider) Name() string {
	return string(deploy.ProviderKubernetes)
}

// Capabilities returns the capabilities of this provider.
func (p *Provider) Capabilities() deploy.Capabilities {
	return deploy.Capabilities{
		AutoScaling:        true,  // HPA support
		CustomDomain:       true,  // Via Ingress
		HTTPS:              true,  // Via Ingress TLS
		VPC:                true,  // K8s networking
		SecretsIntegration: true,  // K8s Secrets
		Preview:            true,  // Pulumi preview
		Rollback:           true,  // K8s rollback
		MaxMemoryMB:        65536, // Depends on cluster nodes
	}
}

// Deploy deploys the workload to Kubernetes.
func (p *Provider) Deploy(ctx context.Context, cfg *deploy.DeployConfig) (*deploy.DeploymentStatus, error) {
	startTime := time.Now()
	status := &deploy.DeploymentStatus{
		StackName: cfg.Stack.Name,
		State:     deploy.StateInProgress,
		Provider:  p.Name(),
		StartTime: startTime,
		Outputs:   make(map[string]string),
	}

	// Create Pulumi stack
	stackOpts := pulumiutil.StackOptions{
		ProjectName:       cfg.Stack.Project,
		StackName:         cfg.Stack.Name,
		BackendURL:        cfg.PulumiBackend,
		Kubeconfig:        p.k8sCfg.Kubeconfig,
		KubeconfigContent: p.k8sCfg.KubeconfigContent,
		KubeContext:       p.k8sCfg.Context,
	}

	stack, err := pulumiutil.NewStack(ctx, stackOpts, p.createProgram(cfg))
	if err != nil {
		status.State = deploy.StateFailed
		status.Error = err.Error()
		return status, fmt.Errorf("kubernetes: failed to create stack: %w", err)
	}
	defer func() { _ = stack.Close() }()

	// Run deployment
	result, err := stack.Up(ctx)
	if err != nil {
		status.State = deploy.StateFailed
		status.Error = err.Error()
		return status, fmt.Errorf("kubernetes: deployment failed: %w", err)
	}

	// Update status
	status.State = deploy.StateSucceeded
	status.EndTime = time.Now()
	status.Duration = status.EndTime.Sub(startTime)
	status.Outputs = result.Outputs

	// Add resource summary
	status.Resources = []deploy.Resource{
		{Type: "kubernetes:apps/v1:Deployment", Name: p.getDeploymentName(cfg), State: "created"},
		{Type: "kubernetes:core/v1:Service", Name: p.getServiceName(cfg), State: "created"},
	}
	if p.k8sCfg.IngressEnabled {
		status.Resources = append(status.Resources, deploy.Resource{
			Type: "kubernetes:networking.k8s.io/v1:Ingress", Name: p.getIngressName(cfg), State: "created",
		})
	}

	return status, nil
}

// Status returns the current deployment status.
func (p *Provider) Status(ctx context.Context, stackName string) (*deploy.DeploymentStatus, error) {
	stackOpts := pulumiutil.StackOptions{
		ProjectName:       p.cfg.Stack.Project,
		StackName:         stackName,
		BackendURL:        p.cfg.PulumiBackend,
		Kubeconfig:        p.k8sCfg.Kubeconfig,
		KubeconfigContent: p.k8sCfg.KubeconfigContent,
		KubeContext:       p.k8sCfg.Context,
	}

	stack, err := pulumiutil.NewStackForStatus(ctx, stackOpts)
	if err != nil {
		return nil, fmt.Errorf("kubernetes: failed to get stack: %w", err)
	}
	defer func() { _ = stack.Close() }()

	outputs, err := stack.Outputs(ctx)
	if err != nil {
		return &deploy.DeploymentStatus{
			StackName: stackName,
			State:     deploy.StateUnknown,
			Provider:  p.Name(),
			Error:     err.Error(),
		}, nil
	}

	return &deploy.DeploymentStatus{
		StackName: stackName,
		State:     deploy.StateSucceeded,
		Provider:  p.Name(),
		Outputs:   outputs,
	}, nil
}

// Destroy removes all resources.
func (p *Provider) Destroy(ctx context.Context, stackName string) error {
	stackOpts := pulumiutil.StackOptions{
		ProjectName:       p.cfg.Stack.Project,
		StackName:         stackName,
		BackendURL:        p.cfg.PulumiBackend,
		Kubeconfig:        p.k8sCfg.Kubeconfig,
		KubeconfigContent: p.k8sCfg.KubeconfigContent,
		KubeContext:       p.k8sCfg.Context,
	}

	stack, err := pulumiutil.NewStackForStatus(ctx, stackOpts)
	if err != nil {
		return fmt.Errorf("kubernetes: failed to get stack: %w", err)
	}
	defer func() { _ = stack.Close() }()

	if err := stack.Destroy(ctx); err != nil {
		return fmt.Errorf("kubernetes: destroy failed: %w", err)
	}

	return nil
}

// Close releases resources.
func (p *Provider) Close() error {
	p.closed = true
	return nil
}

// createProgram returns the Pulumi program for deploying K8s resources.
func (p *Provider) createProgram(cfg *deploy.DeployConfig) pulumi.RunFunc {
	return func(ctx *pulumi.Context) error {
		labels := pulumi.StringMap{
			"app.kubernetes.io/name":       pulumi.String(cfg.Stack.Name),
			"app.kubernetes.io/managed-by": pulumi.String("agentkit"),
		}
		for k, v := range cfg.Stack.Tags {
			labels[k] = pulumi.String(v)
		}

		// Create namespace if requested
		var namespace pulumi.StringOutput
		if p.k8sCfg.CreateNamespace && p.k8sCfg.Namespace != "default" {
			ns, err := corev1.NewNamespace(ctx, p.k8sCfg.Namespace, &corev1.NamespaceArgs{
				Metadata: &metav1.ObjectMetaArgs{
					Name:   pulumi.String(p.k8sCfg.Namespace),
					Labels: labels,
				},
			})
			if err != nil {
				return fmt.Errorf("failed to create namespace: %w", err)
			}
			namespace = ns.Metadata.Name().Elem()
		} else {
			namespace = pulumi.String(p.k8sCfg.Namespace).ToStringOutput()
		}

		// Build container spec
		container := p.buildContainerSpec(cfg)

		// Build pod spec
		podSpec := p.buildPodSpec(cfg, container)

		// Create Deployment
		deploymentName := p.getDeploymentName(cfg)
		deployment, err := appsv1.NewDeployment(ctx, deploymentName, &appsv1.DeploymentArgs{
			Metadata: &metav1.ObjectMetaArgs{
				Name:      pulumi.String(deploymentName),
				Namespace: namespace,
				Labels:    labels,
			},
			Spec: &appsv1.DeploymentSpecArgs{
				Replicas: pulumi.Int(p.k8sCfg.Replicas),
				Selector: &metav1.LabelSelectorArgs{
					MatchLabels: labels,
				},
				Template: &corev1.PodTemplateSpecArgs{
					Metadata: &metav1.ObjectMetaArgs{
						Labels:      p.mergePodLabels(labels),
						Annotations: p.getPodAnnotations(),
					},
					Spec: podSpec,
				},
			},
		})
		if err != nil {
			return fmt.Errorf("failed to create deployment: %w", err)
		}

		// Create Service
		serviceName := p.getServiceName(cfg)
		service, err := corev1.NewService(ctx, serviceName, &corev1.ServiceArgs{
			Metadata: &metav1.ObjectMetaArgs{
				Name:      pulumi.String(serviceName),
				Namespace: namespace,
				Labels:    labels,
			},
			Spec: &corev1.ServiceSpecArgs{
				Type:     pulumi.String(p.k8sCfg.ServiceType),
				Selector: labels,
				Ports: corev1.ServicePortArray{
					&corev1.ServicePortArgs{
						Name:       pulumi.String("http"),
						Port:       pulumi.Int(cfg.Stack.Resources.Port),
						TargetPort: pulumi.Int(cfg.Stack.Resources.Port),
						Protocol:   pulumi.String("TCP"),
					},
				},
			},
		})
		if err != nil {
			return fmt.Errorf("failed to create service: %w", err)
		}

		// Create Ingress if enabled
		if p.k8sCfg.IngressEnabled {
			if err := p.createIngress(ctx, cfg, namespace, serviceName, labels); err != nil {
				return err
			}
		}

		// Export outputs
		ctx.Export("deploymentName", deployment.Metadata.Name())
		ctx.Export("serviceName", service.Metadata.Name())
		ctx.Export("namespace", namespace)
		ctx.Export("serviceType", pulumi.String(p.k8sCfg.ServiceType))

		if p.k8sCfg.ServiceType == "LoadBalancer" {
			ctx.Export("loadBalancerIP", service.Status.LoadBalancer().Ingress().Index(pulumi.Int(0)).Ip())
		}

		return nil
	}
}

// buildContainerSpec builds the container specification.
func (p *Provider) buildContainerSpec(cfg *deploy.DeployConfig) *corev1.ContainerArgs {
	container := &corev1.ContainerArgs{
		Name:            pulumi.String("app"),
		Image:           pulumi.String(p.getImageURI(cfg)),
		ImagePullPolicy: pulumi.String(p.k8sCfg.ImagePullPolicy),
		Ports: corev1.ContainerPortArray{
			&corev1.ContainerPortArgs{
				ContainerPort: pulumi.Int(cfg.Stack.Resources.Port),
				Protocol:      pulumi.String("TCP"),
			},
		},
		Env:       p.buildEnvVars(cfg),
		Resources: p.buildResourceRequirements(cfg),
	}

	// Add probes if configured
	if p.k8sCfg.LivenessProbe != nil {
		container.LivenessProbe = p.buildProbe(p.k8sCfg.LivenessProbe, cfg.Stack.Resources.Port)
	}
	if p.k8sCfg.ReadinessProbe != nil {
		container.ReadinessProbe = p.buildProbe(p.k8sCfg.ReadinessProbe, cfg.Stack.Resources.Port)
	} else if cfg.Stack.HealthCheck != nil {
		// Use health check config for readiness probe
		container.ReadinessProbe = p.buildProbeFromHealthCheck(cfg.Stack.HealthCheck, cfg.Stack.Resources.Port)
	}

	return container
}

// buildPodSpec builds the pod specification.
func (p *Provider) buildPodSpec(cfg *deploy.DeployConfig, container *corev1.ContainerArgs) *corev1.PodSpecArgs {
	podSpec := &corev1.PodSpecArgs{
		Containers: corev1.ContainerArray{container},
	}

	if p.k8sCfg.ServiceAccountName != "" {
		podSpec.ServiceAccountName = pulumi.String(p.k8sCfg.ServiceAccountName)
	}

	if len(p.k8sCfg.ImagePullSecrets) > 0 {
		var secrets corev1.LocalObjectReferenceArray
		for _, s := range p.k8sCfg.ImagePullSecrets {
			secrets = append(secrets, &corev1.LocalObjectReferenceArgs{
				Name: pulumi.String(s),
			})
		}
		podSpec.ImagePullSecrets = secrets
	}

	if len(p.k8sCfg.NodeSelector) > 0 {
		podSpec.NodeSelector = pulumi.ToStringMap(p.k8sCfg.NodeSelector)
	}

	if len(p.k8sCfg.Tolerations) > 0 {
		var tolerations corev1.TolerationArray
		for _, t := range p.k8sCfg.Tolerations {
			tolerations = append(tolerations, &corev1.TolerationArgs{
				Key:      pulumi.String(t.Key),
				Operator: pulumi.String(t.Operator),
				Value:    pulumi.String(t.Value),
				Effect:   pulumi.String(t.Effect),
			})
		}
		podSpec.Tolerations = tolerations
	}

	return podSpec
}

// buildEnvVars builds environment variables from config.
func (p *Provider) buildEnvVars(cfg *deploy.DeployConfig) corev1.EnvVarArray {
	var envVars corev1.EnvVarArray
	for k, v := range cfg.Stack.Environment {
		envVars = append(envVars, &corev1.EnvVarArgs{
			Name:  pulumi.String(k),
			Value: pulumi.String(v),
		})
	}
	return envVars
}

// buildResourceRequirements builds resource limits and requests.
func (p *Provider) buildResourceRequirements(cfg *deploy.DeployConfig) *corev1.ResourceRequirementsArgs {
	limits := pulumi.StringMap{}
	requests := pulumi.StringMap{}

	// Set limits from config
	if cfg.Stack.Resources.CPU != "" {
		limits["cpu"] = pulumi.String(cfg.Stack.Resources.CPU)
	}
	if cfg.Stack.Resources.Memory != "" {
		limits["memory"] = pulumi.String(cfg.Stack.Resources.Memory + "Mi")
	}

	// Set requests (default to limits if not specified)
	if p.k8sCfg.ResourceRequests != nil {
		if p.k8sCfg.ResourceRequests.CPU != "" {
			requests["cpu"] = pulumi.String(p.k8sCfg.ResourceRequests.CPU)
		}
		if p.k8sCfg.ResourceRequests.Memory != "" {
			requests["memory"] = pulumi.String(p.k8sCfg.ResourceRequests.Memory)
		}
	} else {
		// Default requests to 50% of limits
		requests = limits
	}

	return &corev1.ResourceRequirementsArgs{
		Limits:   limits,
		Requests: requests,
	}
}

// buildProbe builds a probe from ProbeConfig.
func (p *Provider) buildProbe(pc *ProbeConfig, defaultPort int) *corev1.ProbeArgs {
	port := pc.HTTPPort
	if port == 0 {
		port = defaultPort
	}

	probe := &corev1.ProbeArgs{
		HttpGet: &corev1.HTTPGetActionArgs{
			Path: pulumi.String(pc.HTTPPath),
			Port: pulumi.Int(port),
		},
	}

	if pc.InitialDelaySeconds > 0 {
		probe.InitialDelaySeconds = pulumi.Int(pc.InitialDelaySeconds)
	}
	if pc.PeriodSeconds > 0 {
		probe.PeriodSeconds = pulumi.Int(pc.PeriodSeconds)
	}
	if pc.TimeoutSeconds > 0 {
		probe.TimeoutSeconds = pulumi.Int(pc.TimeoutSeconds)
	}
	if pc.FailureThreshold > 0 {
		probe.FailureThreshold = pulumi.Int(pc.FailureThreshold)
	}
	if pc.SuccessThreshold > 0 {
		probe.SuccessThreshold = pulumi.Int(pc.SuccessThreshold)
	}

	return probe
}

// buildProbeFromHealthCheck builds a probe from HealthCheckConfig.
func (p *Provider) buildProbeFromHealthCheck(hc *deploy.HealthCheckConfig, defaultPort int) *corev1.ProbeArgs {
	path := hc.Path
	if path == "" {
		path = "/health"
	}

	probe := &corev1.ProbeArgs{
		HttpGet: &corev1.HTTPGetActionArgs{
			Path: pulumi.String(path),
			Port: pulumi.Int(defaultPort),
		},
	}

	if hc.IntervalSeconds > 0 {
		probe.PeriodSeconds = pulumi.Int(hc.IntervalSeconds)
	}
	if hc.TimeoutSeconds > 0 {
		probe.TimeoutSeconds = pulumi.Int(hc.TimeoutSeconds)
	}
	if hc.UnhealthyThreshold > 0 {
		probe.FailureThreshold = pulumi.Int(hc.UnhealthyThreshold)
	}
	if hc.HealthyThreshold > 0 {
		probe.SuccessThreshold = pulumi.Int(hc.HealthyThreshold)
	}

	return probe
}

// createIngress creates an Ingress resource.
func (p *Provider) createIngress(ctx *pulumi.Context, cfg *deploy.DeployConfig, namespace pulumi.StringOutput, serviceName string, labels pulumi.StringMap) error {
	ingressName := p.getIngressName(cfg)

	// Build spec with all fields set at initialization
	ingressSpec := &networkingv1.IngressSpecArgs{
		Rules: networkingv1.IngressRuleArray{
			&networkingv1.IngressRuleArgs{
				Host: pulumi.String(p.k8sCfg.IngressHost),
				Http: &networkingv1.HTTPIngressRuleValueArgs{
					Paths: networkingv1.HTTPIngressPathArray{
						&networkingv1.HTTPIngressPathArgs{
							Path:     pulumi.String("/"),
							PathType: pulumi.String("Prefix"),
							Backend: &networkingv1.IngressBackendArgs{
								Service: &networkingv1.IngressServiceBackendArgs{
									Name: pulumi.String(serviceName),
									Port: &networkingv1.ServiceBackendPortArgs{
										Number: pulumi.Int(cfg.Stack.Resources.Port),
									},
								},
							},
						},
					},
				},
			},
		},
	}

	if p.k8sCfg.IngressClassName != "" {
		ingressSpec.IngressClassName = pulumi.StringPtr(p.k8sCfg.IngressClassName)
	}

	if p.k8sCfg.IngressTLSEnabled {
		ingressSpec.Tls = networkingv1.IngressTLSArray{
			&networkingv1.IngressTLSArgs{
				Hosts:      pulumi.StringArray{pulumi.String(p.k8sCfg.IngressHost)},
				SecretName: pulumi.String(p.k8sCfg.IngressTLSSecretName),
			},
		}
	}

	ingress, err := networkingv1.NewIngress(ctx, ingressName, &networkingv1.IngressArgs{
		Metadata: &metav1.ObjectMetaArgs{
			Name:      pulumi.String(ingressName),
			Namespace: namespace,
			Labels:    labels,
		},
		Spec: ingressSpec,
	})
	if err != nil {
		return fmt.Errorf("failed to create ingress: %w", err)
	}

	ctx.Export("ingressName", ingress.Metadata.Name())
	ctx.Export("ingressHost", pulumi.String(p.k8sCfg.IngressHost))

	return nil
}

// Helper functions

func (p *Provider) getDeploymentName(cfg *deploy.DeployConfig) string {
	return cfg.Stack.Name
}

func (p *Provider) getServiceName(cfg *deploy.DeployConfig) string {
	return cfg.Stack.Name + "-svc"
}

func (p *Provider) getIngressName(cfg *deploy.DeployConfig) string {
	return cfg.Stack.Name + "-ingress"
}

func (p *Provider) getImageURI(cfg *deploy.DeployConfig) string {
	if cfg.Stack.Image.Digest != "" {
		return cfg.Stack.Image.Repository + "@" + cfg.Stack.Image.Digest
	}
	tag := cfg.Stack.Image.Tag
	if tag == "" {
		tag = "latest"
	}
	return cfg.Stack.Image.Repository + ":" + tag
}

func (p *Provider) mergePodLabels(base pulumi.StringMap) pulumi.StringMap {
	result := pulumi.StringMap{}
	for k, v := range base {
		result[k] = v
	}
	for k, v := range p.k8sCfg.PodLabels {
		result[k] = pulumi.String(v)
	}
	return result
}

func (p *Provider) getPodAnnotations() pulumi.StringMap {
	if len(p.k8sCfg.PodAnnotations) == 0 {
		return nil
	}
	annotations := pulumi.StringMap{}
	for k, v := range p.k8sCfg.PodAnnotations {
		annotations[k] = pulumi.String(v)
	}
	return annotations
}
