package kube

import (
	"context"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

// newTestClient creates a kube.Client backed by a fake Kubernetes clientset.
// All API calls go to an in-memory store — no real cluster is needed.
func newTestClient() *Client {
	return &Client{clientset: fake.NewClientset()}
}

func TestDeployModel_CreatesDeploymentAndService(t *testing.T) {
	ctx := context.Background()
	c := newTestClient()

	cfg := ModelConfig{
		Name:     "tinyllama",
		ModelURL: "https://example.com/model.gguf",
		NodePort: 30001,
	}

	// Act: deploy the model.
	if err := c.DeployModel(ctx, "default", cfg); err != nil {
		t.Fatalf("DeployModel() returned error: %v", err)
	}

	// Assert: the Deployment was created in the "default" namespace.
	dep, err := c.clientset.AppsV1().Deployments("default").Get(ctx, "tinyllama", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("expected Deployment 'tinyllama' to exist, got error: %v", err)
	}
	if *dep.Spec.Replicas != 1 {
		t.Errorf("expected 1 replica, got %d", *dep.Spec.Replicas)
	}

	// Assert: the init container downloads the model from the correct URL.
	initContainers := dep.Spec.Template.Spec.InitContainers
	if len(initContainers) != 1 {
		t.Fatalf("expected 1 init container, got %d", len(initContainers))
	}
	// The init container's command is: ["curl", "-L", "-o", "/data/model.gguf", <modelURL>]
	cmd := initContainers[0].Command
	if len(cmd) < 5 || cmd[4] != cfg.ModelURL {
		t.Errorf("expected init container to download from %q, got command: %v", cfg.ModelURL, cmd)
	}

	// Assert: the Service was created with the correct NodePort.
	svc, err := c.clientset.CoreV1().Services("default").Get(ctx, "tinyllama", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("expected Service 'tinyllama' to exist, got error: %v", err)
	}
	if svc.Spec.Ports[0].NodePort != 30001 {
		t.Errorf("expected NodePort 30001, got %d", svc.Spec.Ports[0].NodePort)
	}
}

func TestDeployModel_IsIdempotent(t *testing.T) {
	ctx := context.Background()
	c := newTestClient()

	cfg := ModelConfig{
		Name:     "tinyllama",
		ModelURL: "https://example.com/model.gguf",
		NodePort: 30001,
	}

	// Deploy twice — second call should not error (idempotent).
	if err := c.DeployModel(ctx, "default", cfg); err != nil {
		t.Fatalf("first DeployModel() returned error: %v", err)
	}
	if err := c.DeployModel(ctx, "default", cfg); err != nil {
		t.Fatalf("second DeployModel() returned error (should be idempotent): %v", err)
	}

	// Assert: still only one Deployment exists.
	list, err := c.clientset.AppsV1().Deployments("default").List(ctx, metav1.ListOptions{})
	if err != nil {
		t.Fatalf("list deployments error: %v", err)
	}
	if len(list.Items) != 1 {
		t.Errorf("expected 1 deployment after idempotent deploy, got %d", len(list.Items))
	}
}

func TestDeployModel_DefaultsNameToVllm(t *testing.T) {
	ctx := context.Background()
	c := newTestClient()

	cfg := ModelConfig{
		Name:     "", // empty — should default to "vllm"
		ModelURL: "https://example.com/model.gguf",
		NodePort: 30002,
	}

	if err := c.DeployModel(ctx, "default", cfg); err != nil {
		t.Fatalf("DeployModel() returned error: %v", err)
	}

	// Assert: Deployment name defaults to "vllm".
	_, err := c.clientset.AppsV1().Deployments("default").Get(ctx, "vllm", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("expected Deployment 'vllm' to exist when Name is empty, got error: %v", err)
	}
}

func TestDeleteModel_RemovesDeploymentAndService(t *testing.T) {
	ctx := context.Background()
	c := newTestClient()

	cfg := ModelConfig{
		Name:     "tinyllama",
		ModelURL: "https://example.com/model.gguf",
		NodePort: 30001,
	}

	// Setup: deploy first.
	if err := c.DeployModel(ctx, "default", cfg); err != nil {
		t.Fatalf("DeployModel() returned error: %v", err)
	}

	// Act: delete the model.
	if err := c.DeleteModel(ctx, "default", "tinyllama"); err != nil {
		t.Fatalf("DeleteModel() returned error: %v", err)
	}

	// Assert: Deployment is gone.
	_, err := c.clientset.AppsV1().Deployments("default").Get(ctx, "tinyllama", metav1.GetOptions{})
	if err == nil {
		t.Error("expected Deployment 'tinyllama' to be deleted, but it still exists")
	}

	// Assert: Service is gone.
	_, err = c.clientset.CoreV1().Services("default").Get(ctx, "tinyllama", metav1.GetOptions{})
	if err == nil {
		t.Error("expected Service 'tinyllama' to be deleted, but it still exists")
	}
}

func TestDeleteModel_IsIdempotent(t *testing.T) {
	ctx := context.Background()
	c := newTestClient()

	// Delete something that doesn't exist — should not error.
	if err := c.DeleteModel(ctx, "default", "nonexistent"); err != nil {
		t.Fatalf("DeleteModel() on nonexistent resource should not error, got: %v", err)
	}
}

func TestDeployModel_SetsCorrectLabels(t *testing.T) {
	ctx := context.Background()
	c := newTestClient()

	cfg := ModelConfig{
		Name:     "my-model",
		ModelURL: "https://example.com/model.gguf",
		NodePort: 30003,
	}

	if err := c.DeployModel(ctx, "default", cfg); err != nil {
		t.Fatalf("DeployModel() returned error: %v", err)
	}

	dep, err := c.clientset.AppsV1().Deployments("default").Get(ctx, "my-model", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get deployment error: %v", err)
	}

	// Assert: the label selector matches the pod template labels.
	selectorLabels := dep.Spec.Selector.MatchLabels
	templateLabels := dep.Spec.Template.ObjectMeta.Labels

	if selectorLabels["app"] != "my-model" {
		t.Errorf("expected selector label app=my-model, got %v", selectorLabels)
	}
	if templateLabels["app"] != "my-model" {
		t.Errorf("expected template label app=my-model, got %v", templateLabels)
	}
}
