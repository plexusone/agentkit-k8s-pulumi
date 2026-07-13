// Package pulumi provides utilities for working with Pulumi automation API.
package pulumi

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/pulumi/pulumi/sdk/v3/go/auto"
	"github.com/pulumi/pulumi/sdk/v3/go/auto/optdestroy"
	"github.com/pulumi/pulumi/sdk/v3/go/auto/optup"
	"github.com/pulumi/pulumi/sdk/v3/go/common/tokens"
	"github.com/pulumi/pulumi/sdk/v3/go/common/workspace"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// StackOptions configures a Pulumi stack.
type StackOptions struct {
	// ProjectName is the Pulumi project name.
	ProjectName string

	// StackName is the Pulumi stack name.
	StackName string

	// WorkDir is the working directory for the stack.
	// If empty, a temporary directory is created.
	WorkDir string

	// BackendURL is the Pulumi state backend URL.
	// Examples: "file://~/.pulumi", "s3://my-bucket/pulumi", "https://api.pulumi.com"
	BackendURL string

	// Kubeconfig is the path to the kubeconfig file.
	Kubeconfig string

	// KubeconfigContent is the raw kubeconfig YAML content.
	KubeconfigContent string

	// KubeContext is the kubeconfig context to use.
	KubeContext string

	// Config contains Pulumi config values.
	Config map[string]string

	// SecretConfig contains secret config values.
	SecretConfig map[string]string

	// EnvVars are environment variables for Pulumi operations.
	EnvVars map[string]string
}

// Stack wraps the Pulumi automation API stack.
type Stack struct {
	stack    auto.Stack
	opts     StackOptions
	workDir  string
	isTempWD bool
}

// UpResult contains the results of a stack update.
type UpResult struct {
	Outputs map[string]string
	Summary auto.UpdateSummary
}

// NewStack creates a new Pulumi stack with the given program.
func NewStack(ctx context.Context, opts StackOptions, program pulumi.RunFunc) (*Stack, error) {
	s := &Stack{opts: opts}

	// Set up working directory
	if opts.WorkDir != "" {
		s.workDir = opts.WorkDir
	} else {
		tmpDir, err := os.MkdirTemp("", "pulumi-k8s-*")
		if err != nil {
			return nil, fmt.Errorf("create temp dir: %w", err)
		}
		s.workDir = tmpDir
		s.isTempWD = true
	}

	// Set up environment
	envVars := s.buildEnvVars()

	// Create or select stack
	stackOpts := []auto.LocalWorkspaceOption{
		auto.Project(workspace.Project{
			Name:    tokens.PackageName(opts.ProjectName),
			Runtime: workspace.NewProjectRuntimeInfo("go", nil),
		}),
		auto.WorkDir(s.workDir),
		auto.EnvVars(envVars),
	}

	if opts.BackendURL != "" {
		stackOpts = append(stackOpts, auto.Stacks(map[string]workspace.ProjectStack{}))
	}

	stack, err := auto.UpsertStackInlineSource(ctx, opts.StackName, opts.ProjectName, program, stackOpts...)
	if err != nil {
		s.cleanup()
		return nil, fmt.Errorf("create stack: %w", err)
	}

	s.stack = stack

	// Set config values
	if err := s.setConfig(ctx); err != nil {
		s.cleanup()
		return nil, err
	}

	return s, nil
}

// NewStackForStatus creates a stack reference for status/destroy operations.
func NewStackForStatus(ctx context.Context, opts StackOptions) (*Stack, error) {
	s := &Stack{opts: opts}

	// Set up working directory
	if opts.WorkDir != "" {
		s.workDir = opts.WorkDir
	} else {
		tmpDir, err := os.MkdirTemp("", "pulumi-k8s-*")
		if err != nil {
			return nil, fmt.Errorf("create temp dir: %w", err)
		}
		s.workDir = tmpDir
		s.isTempWD = true
	}

	// Set up environment
	envVars := s.buildEnvVars()

	// Select existing stack
	stackOpts := []auto.LocalWorkspaceOption{
		auto.Project(workspace.Project{
			Name:    tokens.PackageName(opts.ProjectName),
			Runtime: workspace.NewProjectRuntimeInfo("go", nil),
		}),
		auto.WorkDir(s.workDir),
		auto.EnvVars(envVars),
	}

	stack, err := auto.SelectStackInlineSource(ctx, opts.StackName, opts.ProjectName, nil, stackOpts...)
	if err != nil {
		s.cleanup()
		return nil, fmt.Errorf("select stack: %w", err)
	}

	s.stack = stack
	return s, nil
}

// Up runs a Pulumi update.
func (s *Stack) Up(ctx context.Context, opts ...optup.Option) (*UpResult, error) {
	result, err := s.stack.Up(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("pulumi up: %w", err)
	}

	outputs := make(map[string]string)
	for k, v := range result.Outputs {
		if str, ok := v.Value.(string); ok {
			outputs[k] = str
		} else {
			outputs[k] = fmt.Sprintf("%v", v.Value)
		}
	}

	return &UpResult{
		Outputs: outputs,
		Summary: result.Summary,
	}, nil
}

// Destroy removes all resources.
func (s *Stack) Destroy(ctx context.Context, opts ...optdestroy.Option) error {
	_, err := s.stack.Destroy(ctx, opts...)
	if err != nil {
		return fmt.Errorf("pulumi destroy: %w", err)
	}
	return nil
}

// Outputs returns the stack outputs.
func (s *Stack) Outputs(ctx context.Context) (map[string]string, error) {
	outputs, err := s.stack.Outputs(ctx)
	if err != nil {
		return nil, fmt.Errorf("get outputs: %w", err)
	}

	result := make(map[string]string)
	for k, v := range outputs {
		if str, ok := v.Value.(string); ok {
			result[k] = str
		} else {
			result[k] = fmt.Sprintf("%v", v.Value)
		}
	}

	return result, nil
}

// Close cleans up resources.
func (s *Stack) Close() error {
	s.cleanup()
	return nil
}

// buildEnvVars constructs environment variables for Pulumi.
func (s *Stack) buildEnvVars() map[string]string {
	env := make(map[string]string)

	// Copy user-provided env vars
	for k, v := range s.opts.EnvVars {
		env[k] = v
	}

	// Set Pulumi backend if specified
	if s.opts.BackendURL != "" {
		env["PULUMI_BACKEND_URL"] = s.opts.BackendURL
	}

	// Set kubeconfig for Kubernetes provider
	if s.opts.KubeconfigContent != "" {
		// Write kubeconfig to temp file
		kubeconfigPath := filepath.Join(s.workDir, "kubeconfig")
		if err := os.WriteFile(kubeconfigPath, []byte(s.opts.KubeconfigContent), 0600); err == nil {
			env["KUBECONFIG"] = kubeconfigPath
		}
	} else if s.opts.Kubeconfig != "" {
		env["KUBECONFIG"] = s.opts.Kubeconfig
	}

	if s.opts.KubeContext != "" {
		env["KUBERNETES_CONTEXT"] = s.opts.KubeContext
	}

	return env
}

// setConfig sets Pulumi config values.
func (s *Stack) setConfig(ctx context.Context) error {
	// Set regular config
	for k, v := range s.opts.Config {
		if err := s.stack.SetConfig(ctx, k, auto.ConfigValue{Value: v}); err != nil {
			return fmt.Errorf("set config %s: %w", k, err)
		}
	}

	// Set secret config
	for k, v := range s.opts.SecretConfig {
		if err := s.stack.SetConfig(ctx, k, auto.ConfigValue{Value: v, Secret: true}); err != nil {
			return fmt.Errorf("set secret config %s: %w", k, err)
		}
	}

	return nil
}

// cleanup removes temporary resources.
func (s *Stack) cleanup() {
	if s.isTempWD && s.workDir != "" {
		_ = os.RemoveAll(s.workDir)
	}
}
