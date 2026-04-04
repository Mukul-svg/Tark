package worker

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/pulumi/pulumi/sdk/v3/go/auto"
	"github.com/pulumi/pulumi/sdk/v3/go/auto/optdestroy"
	"github.com/pulumi/pulumi/sdk/v3/go/auto/optup"
)

type ClusterData struct {
	PublicIP string
	// Future: SSHkey, Kubeconfig, etc
}

func ProvisionCluster(ctx context.Context, stackName string, workDir string, configMap map[string]string) (*ClusterData, error) {
	absWorkDir, err := filepath.Abs(workDir)

	if err != nil {
		return nil, err
	}

	slog.Info("initializing Pulumi stack", "stack", stackName, "workDir", absWorkDir)

	env := map[string]string{
		"PATH": fmt.Sprintf("%s:%s", filepath.Join(filepath.Dir(filepath.Dir(absWorkDir)), "fake-go"), os.Getenv("PATH")),
	}
	s, err := auto.UpsertStackLocalSource(ctx, stackName, absWorkDir, auto.EnvVars(env))
	if err != nil {
		return nil, fmt.Errorf("failed to initialize stack: %w", err)
	}

	for k, v := range configMap {
		if err := s.SetConfig(ctx, k, auto.ConfigValue{Value: v}); err != nil {
			return nil, err
		}
	}

	slog.Info("starting infrastructure update")

	res, err := s.Up(ctx, optup.ProgressStreams(os.Stdout))
	if err != nil {
		return nil, fmt.Errorf("infra update failed: %w", err)
	}

	outputs := res.Outputs
	publicIp, ok := outputs["publicIp"].Value.(string)
	if !ok {
		return nil, fmt.Errorf("failed to get publicIp from stack outputs")
	}

	slog.Info("infrastructure provisioned", "publicIp", publicIp)

	return &ClusterData{
		PublicIP: publicIp,
	}, nil
}

func DestroyCluster(ctx context.Context, stackName string, workDir string) error {
	absWorkDir, err := filepath.Abs(workDir)
	if err != nil {
		return err
	}

	slog.Info("initializing Pulumi stack for destruction", "stack", stackName)

	env := map[string]string{
		"PATH": fmt.Sprintf("%s:%s", filepath.Join(filepath.Dir(filepath.Dir(absWorkDir)), "fake-go"), os.Getenv("PATH")),
	}
	s, err := auto.UpsertStackLocalSource(ctx, stackName, absWorkDir, auto.EnvVars(env))
	if err != nil {
		return fmt.Errorf("failed to initialize stack: %w", err)
	}

	slog.Info("infrastructure destruction started")

	// Verify stack exists/refresh state could be useful but destroy handles it.
	_, err = s.Destroy(ctx, optdestroy.ProgressStreams(os.Stdout))
	if err != nil {
		return fmt.Errorf("infra destroy failed: %w", err)
	}

	slog.Info("infrastructure destroyed")
	return nil
}
