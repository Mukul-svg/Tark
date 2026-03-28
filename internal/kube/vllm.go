package kube

import (
	"context"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

type ModelConfig struct {
	Name     string
	ModelURL string
	NodePort int32
}

func (c *Client) DeployModel(ctx context.Context, namespace string, cfg ModelConfig) error {
	name := cfg.Name
	if name == "" {
		name = "vllm" // Default
	}
	labels := map[string]string{"app": name}

	// 1. Ensure Deployment
	_, err := c.GetK8s().AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return err
		}
		// Create Deployment: Llama.cpp CPU optimized
		replicas := int32(1)
		deployment := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
				Labels:    labels,
			},
			Spec: appsv1.DeploymentSpec{
				Replicas: &replicas,
				Selector: &metav1.LabelSelector{MatchLabels: labels},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{Labels: labels},
					Spec: corev1.PodSpec{
						// Using a shared emptyDir volume to pass the model from init-container to main container
						Volumes: []corev1.Volume{
							{
								Name: "model-storage",
								VolumeSource: corev1.VolumeSource{
									EmptyDir: &corev1.EmptyDirVolumeSource{},
								},
							},
						},
						InitContainers: []corev1.Container{
							{
								Name:  "model-downloader",
								Image: "curlimages/curl:latest",
								Command: []string{
									"curl",
									"-L",
									"-o", "/data/model.gguf",
									cfg.ModelURL,
								},
								VolumeMounts: []corev1.VolumeMount{
									{
										Name:      "model-storage",
										MountPath: "/data",
									},
								},
							},
						},
						Containers: []corev1.Container{
							{
								Name:  "llama-cpp",
								Image: "ghcr.io/ggml-org/llama.cpp:server",
								Args: []string{
									"-m", "/data/model.gguf",
									"--host", "0.0.0.0",
									"--port", "8000",
									"-c", "512", // Context window
								},
								Ports: []corev1.ContainerPort{{ContainerPort: 8000}},
								VolumeMounts: []corev1.VolumeMount{
									{
										Name:      "model-storage",
										MountPath: "/data",
									},
								},
								Resources: corev1.ResourceRequirements{
									Requests: corev1.ResourceList{
										corev1.ResourceMemory: resource.MustParse("512Mi"),
										corev1.ResourceCPU:    resource.MustParse("500m"),
									},
									Limits: corev1.ResourceList{
										corev1.ResourceMemory: resource.MustParse("2Gi"), // Should be plenty for TinyLlama
										corev1.ResourceCPU:    resource.MustParse("2"),
									},
								},
							},
						},
					},
				},
			},
		}
		if _, err := c.GetK8s().AppsV1().Deployments(namespace).Create(ctx, deployment, metav1.CreateOptions{}); err != nil {
			return err
		}
	}

	// 2. Ensure Service
	_, err = c.GetK8s().CoreV1().Services(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return err
		}
		// Create Service
		service := &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
				Labels:    labels,
			},
			Spec: corev1.ServiceSpec{
				Selector: labels,
				Ports: []corev1.ServicePort{
					{
						Port:       8000,
						TargetPort: intstr.FromInt(8000),
						NodePort:   cfg.NodePort,
					},
				},
				Type: corev1.ServiceTypeNodePort, // Expose externally
			},
		}
		if _, err := c.GetK8s().CoreV1().Services(namespace).Create(ctx, service, metav1.CreateOptions{}); err != nil {
			return err
		}
	}

	return nil
}
