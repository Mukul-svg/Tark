package worker

import (
	"fmt"
	"os"
	"time"

	"golang.org/x/crypto/ssh"
)

// FetchRemoteKubeConfig connects the VM via SSH and fetches the kubeconfig file
func FetchRemoteKubeConfig(hostIP string, privateKeyPath string) ([]byte, error) {
	//Read private key
	key, err := os.ReadFile(privateKeyPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read private key: %v", err)
	}

	// Create the Signer for SSH configuration
	signer, err := ssh.ParsePrivateKey(key)
	if err != nil {
		return nil, fmt.Errorf("failed to parse private key: %v", err)
	}

	// Configure SSH client
	config := &ssh.ClientConfig{
		User: "azureuser", // Default user for Azure VM
		Auth: []ssh.AuthMethod{
			ssh.PublicKeys(signer),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(), // TODO: Use strict checking in production
		Timeout:         10 * time.Second,
	}

	// Connect to the server
	conn, err := ssh.Dial("tcp", fmt.Sprintf("%s:22", hostIP), config)
	if err != nil {
		return nil, fmt.Errorf("failed to dial SSH: %v", err)
	}
	defer conn.Close()

	// Create a new session
	session, err := conn.NewSession()
	if err != nil {
		return nil, fmt.Errorf("failed to create SSH session: %v", err)
	}
	defer session.Close()

	// Run the command to fetch kubeconfig
	output, err := session.Output("cat /home/azureuser/client.config")
	if err != nil {
		return nil, fmt.Errorf("failed to run command to fetch kubeconfig: %v", err)
	}
	return output, nil
}
