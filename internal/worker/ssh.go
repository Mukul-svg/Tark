package worker

import (
	"bytes"
	"fmt"
	"os"
	"time"

	"golang.org/x/crypto/ssh"
)

func FetchRemoteKubeConfig(hostIP string, privateKeyPath string) ([]byte, error) {
	if err := os.Chmod(privateKeyPath, 0600); err != nil {
		return nil, fmt.Errorf("failed to set private key permissions: %v", err)
	}
	key, err := os.ReadFile(privateKeyPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read private key: %v", err)
	}

	signer, err := ssh.ParsePrivateKey(key)
	if err != nil {
		return nil, fmt.Errorf("failed to parse private key: %v", err)
	}

	config := &ssh.ClientConfig{
		User: "azureuser", // Default user for Azure VM
		Auth: []ssh.AuthMethod{
			ssh.PublicKeys(signer),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(), // TODO: Use strict checking in production
		Timeout:         10 * time.Second,
	}

	conn, err := ssh.Dial("tcp", fmt.Sprintf("%s:22", hostIP), config)
	if err != nil {
		return nil, fmt.Errorf("failed to dial SSH: %v", err)
	}
	defer conn.Close()

	session, err := conn.NewSession()
	if err != nil {
		return nil, fmt.Errorf("failed to create SSH session: %v", err)
	}
	defer session.Close()

	var stderrBuf bytes.Buffer
	session.Stderr = &stderrBuf

	output, err := session.Output("cat /home/azureuser/client.config")
	if err != nil {
		return nil, fmt.Errorf("failed to run command to fetch kubeconfig: %v, stderr: %q", err, stderrBuf.String())
	}
	return output, nil
}
