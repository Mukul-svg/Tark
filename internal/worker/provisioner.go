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

// Holds data of output of a successfully provisioned cluster
type ClusterData struct {
	PublicIp string
	// Future: SSHkey, Kubeconfig, etc
}

// Runs pulumi up programmatically to provision a k8s cluster pointing to infra/azure
func ProvisionCluster(ctx context.Context, stackName string, workDir string, configMap map[string]string) (*ClusterData, error) {
	// Currently only Azure is supported
	// Find abs path to infra/azure
	absWorkDir, err := filepath.Abs(workDir)

	if err != nil {
		return nil, err
	}

	slog.Info("Initializing Pulumi stack", "stack", stackName, "workDir", absWorkDir)

	// Initialize Stack (Pulumi stack select)
	// Using LocalSource as our Pulumi code is a generic local folder
	// Use Upsert to create if missing, or select if existing (idempotent)
	s, err := auto.UpsertStackLocalSource(ctx, stackName, absWorkDir)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize stack: %w", err)
	}

	//Set configuration
	//Applying any custom config passed
	if err := s.SetConfig(ctx, "azure:location", auto.ConfigValue{Value: "southindia"}); err != nil {
		return nil, err
	}

	for k, v := range configMap {
		if err := s.SetConfig(ctx, k, auto.ConfigValue{Value: v}); err != nil {
			return nil, err
		}
	}

	slog.Info("Starting infrastructure update... check stdout for progress bars")

	//Running pulumi up
	// Mapping underlying pulumi engine to our stdout to see the progress bars
	res, err := s.Up(ctx, optup.ProgressStreams(os.Stdout))
	if err != nil {
		return nil, fmt.Errorf("Infra update failed: %w", err)
	}

	//Retrieving Public IP from outputs
	outputs := res.Outputs
	publicIp, ok := outputs["publicIp"].Value.(string)
	if !ok {
		return nil, fmt.Errorf("failed to get publicIp from stack outputs")
	}

	slog.Info("Infrastructure provisioned successfully", "publicIp", publicIp)

	return &ClusterData{
		PublicIp: publicIp,
	}, nil
}

// Runs pulumi destroy programmatically
func DestroyCluster(ctx context.Context, stackName string, workDir string) error {
	absWorkDir, err := filepath.Abs(workDir)
	if err != nil {
		return err
	}

	slog.Info("Initializing Pulumi stack for destruction", "stack", stackName)

	s, err := auto.UpsertStackLocalSource(ctx, stackName, absWorkDir)
	if err != nil {
		return fmt.Errorf("failed to initialize stack: %w", err)
	}

	slog.Info("Starting infrastructure destruction...")

	// Verify stack exists/refresh state could be useful but destroy handles it.
	_, err = s.Destroy(ctx, optdestroy.ProgressStreams(os.Stdout))
	if err != nil {
		return fmt.Errorf("infra destroy failed: %w", err)
	}

	slog.Info("Infrastructure destroyed successfully")
	return nil
}
