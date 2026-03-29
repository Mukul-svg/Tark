package kube

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

type Client struct {
	clientset kubernetes.Interface
}

func New() (*Client, error) {
	config, err := getKubeConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to get kubeconfig: %v", err)
	}
	config.Timeout = 10 * time.Second

	kubeClient, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create kubernetes client: %v", err)
	}

	return &Client{clientset: kubeClient}, nil
}

func NewFromKubeConfig(configBytes []byte) (*Client, error) {
	clientConfig, err := clientcmd.NewClientConfigFromBytes(configBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to create client config from bytes: %v", err)
	}

	restConfig, err := clientConfig.ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to get rest config: %v", err)
	}
	restConfig.Timeout = 10 * time.Second

	// Insecure TLS to allow connection to Public IP with localhost cert
	restConfig.TLSClientConfig.Insecure = true
	restConfig.TLSClientConfig.CAData = nil
	restConfig.TLSClientConfig.CAFile = ""

	kubeClient, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create kubernetes client: %v", err)
	}

	return &Client{clientset: kubeClient}, nil
}

func (c *Client) GetK8s() kubernetes.Interface {
	return c.clientset
}

func getKubeConfig() (*rest.Config, error) {
	if envVar := os.Getenv("KUBECONFIG"); envVar != "" {
		return clientcmd.BuildConfigFromFlags("", envVar)
	}

	if home, err := os.UserHomeDir(); err == nil {
		kubeconfigPath := filepath.Join(home, ".kube", "config")
		if _, err := os.Stat(kubeconfigPath); err == nil {
			return clientcmd.BuildConfigFromFlags("", kubeconfigPath)
		}
	}

	return rest.InClusterConfig()
}
