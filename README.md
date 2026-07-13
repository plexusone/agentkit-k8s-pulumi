# AgentKit Kubernetes Pulumi Provider

[![Go CI][go-ci-svg]][go-ci-url]
[![Go Lint][go-lint-svg]][go-lint-url]
[![Go SAST][go-sast-svg]][go-sast-url]
[![Docs][docs-godoc-svg]][docs-godoc-url]
[![Visualization][viz-svg]][viz-url]
[![License][license-svg]][license-url]

 [go-ci-svg]: https://github.com/plexusone/agentkit-k8s-pulumi/actions/workflows/go-ci.yaml/badge.svg?branch=main
 [go-ci-url]: https://github.com/plexusone/agentkit-k8s-pulumi/actions/workflows/go-ci.yaml
 [go-lint-svg]: https://github.com/plexusone/agentkit-k8s-pulumi/actions/workflows/go-lint.yaml/badge.svg?branch=main
 [go-lint-url]: https://github.com/plexusone/agentkit-k8s-pulumi/actions/workflows/go-lint.yaml
 [go-sast-svg]: https://github.com/plexusone/agentkit-k8s-pulumi/actions/workflows/go-sast-codeql.yaml/badge.svg?branch=main
 [go-sast-url]: https://github.com/plexusone/agentkit-k8s-pulumi/actions/workflows/go-sast-codeql.yaml
 [docs-godoc-svg]: https://pkg.go.dev/badge/github.com/plexusone/agentkit-k8s-pulumi
 [docs-godoc-url]: https://pkg.go.dev/github.com/plexusone/agentkit-k8s-pulumi
 [docs-mkdoc-svg]: https://img.shields.io/badge/Go-dev%20guide-blue.svg
 [docs-mkdoc-url]: https://plexusone.dev/agentkit-k8s-pulumi
 [viz-svg]: https://img.shields.io/badge/Go-visualizaton-blue.svg
 [viz-url]: https://mango-dune-07a8b7110.1.azurestaticapps.net/?repo=plexusone%2Fagentkit-k8s-pulumi
 [loc-svg]: https://tokei.rs/b1/github/plexusone/agentkit-k8s-pulumi
 [repo-url]: https://github.com/plexusone/agentkit-k8s-pulumi
 [license-svg]: https://img.shields.io/badge/license-MIT-blue.svg
 [license-url]: https://github.com/plexusone/agentkit-k8s-pulumi/blob/main/LICENSE

A Kubernetes deployment provider for [AgentKit](https://github.com/plexusone/agentkit) using Pulumi. Enables cloud-agnostic container deployments across EKS, GKE, AKS, and on-premises clusters.

## Features

- ☁️ **Cloud-Agnostic**: Same deployment code works across all Kubernetes distributions
- 🏗️ **Pulumi-Based**: Infrastructure-as-Code with Go, no YAML required
- ☸️ **Full K8s Support**: Deployment, Service, Ingress with TLS
- 🔧 **Configurable**: Resource limits, probes, tolerations, node selectors

## Supported Platforms

| Cloud | Kubernetes Service |
|-------|-------------------|
| AWS | EKS (Elastic Kubernetes Service) |
| GCP | GKE (Google Kubernetes Engine) |
| Azure | AKS (Azure Kubernetes Service) |
| On-prem | Any Kubernetes cluster |

## Installation

```bash
go get github.com/plexusone/agentkit-k8s-pulumi
```

## Quick Start

```go
package main

import (
    "context"

    "github.com/plexusone/agentkit/deploy"
    _ "github.com/plexusone/agentkit-k8s-pulumi/deploy/providers/kubernetes"
)

func main() {
    ctx := context.Background()

    // Load deployment configuration
    cfg, err := deploy.LoadDeployConfig("deploy.yaml")
    if err != nil {
        panic(err)
    }

    // Get Kubernetes provider (auto-registered via blank import)
    provider, err := deploy.GetProviderByName(deploy.ProviderKubernetes, cfg)
    if err != nil {
        panic(err)
    }
    defer provider.Close()

    // Deploy to Kubernetes
    status, err := provider.Deploy(ctx, cfg)
    if err != nil {
        panic(err)
    }

    fmt.Printf("Deployed to namespace: %s\n", status.Outputs["namespace"])
}
```

## Configuration

### deploy.yaml

```yaml
stack:
  name: my-agent
  project: my-project
  image:
    repository: myorg/myagent
    tag: v1.0.0
  resources:
    cpu: "0.5"
    memory: "512"
    port: 8080
  environment:
    LOG_LEVEL: info

provider: kubernetes
```

### Kubernetes-Specific Options

Set via environment variables or provider extensions:

| Option | Description | Default |
|--------|-------------|---------|
| `KUBECONFIG` | Path to kubeconfig file | `~/.kube/config` |
| `KUBERNETES_CONTEXT` | Kubeconfig context to use | Current context |
| `KUBERNETES_NAMESPACE` | Target namespace | `default` |

### Provider Extensions

```go
cfg.ProviderOptions = map[string]any{
    "namespace":        "my-agents",
    "serviceType":      "LoadBalancer",
    "replicas":         3,
    "ingressEnabled":   true,
    "ingressHost":      "agent.example.com",
    "ingressClassName": "nginx",
}
```

## Capabilities

| Capability | Supported |
|------------|-----------|
| Auto Scaling | Yes (HPA) |
| Custom Domain | Yes (Ingress) |
| HTTPS/TLS | Yes (Ingress TLS) |
| VPC Networking | Yes |
| Secrets Integration | Yes (K8s Secrets) |
| Preview | Yes (Pulumi preview) |
| Rollback | Yes (K8s rollback) |

## Architecture

```
agentkit-k8s-pulumi/
├── deploy/
│   ├── providers/
│   │   └── kubernetes/
│   │       ├── config.go     # K8s-specific configuration
│   │       ├── init.go       # Provider registration
│   │       └── provider.go   # Provider implementation
│   └── pulumi/
│       └── stack.go          # Pulumi automation API wrapper
└── go.mod
```

## Dependencies

- [agentkit](https://github.com/plexusone/agentkit) v0.7.0+ - Core deployment interfaces
- [pulumi-kubernetes](https://github.com/pulumi/pulumi-kubernetes) - Kubernetes resource management
- [pulumi/sdk](https://github.com/pulumi/pulumi) - Pulumi automation API

## Related Modules

| Module | Purpose |
|--------|---------|
| [agentkit](https://github.com/plexusone/agentkit) | Core agent framework |
| [agentkit-aws-pulumi](https://github.com/plexusone/agentkit-aws-pulumi) | AWS Lightsail provider |

## License

MIT License
