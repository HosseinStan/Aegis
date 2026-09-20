package cisco_ios_ssh

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"strings"
	"time"

	"company.local/aegis/apps/aegis-api/internal/domain"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

type Client struct {
	KnownHostsFile   string
	ConnectTimeout   time.Duration
	HandshakeTimeout time.Duration
	CommandTimeout   time.Duration
	MaxOutputBytes   int64
}

func New(knownHostsFile string, connectTimeout, handshakeTimeout, commandTimeout time.Duration, maxOutputBytes int64) (*Client, error) {
	if strings.TrimSpace(knownHostsFile) == "" {
		return nil, errors.New("known-hosts file is required")
	}
	if connectTimeout <= 0 || handshakeTimeout <= 0 || commandTimeout <= 0 || maxOutputBytes <= 0 {
		return nil, errors.New("SSH limits must be positive")
	}
	return &Client{KnownHostsFile: knownHostsFile, ConnectTimeout: connectTimeout, HandshakeTimeout: handshakeTimeout, CommandTimeout: commandTimeout, MaxOutputBytes: maxOutputBytes}, nil
}

func (c *Client) TestConnection(ctx context.Context, device domain.Device, credential domain.Credential) error {
	client, err := c.dial(ctx, device, credential)
	if err != nil {
		return err
	}
	return client.Close()
}

func (c *Client) GetRunningConfig(ctx context.Context, device domain.Device, credential domain.Credential) (domain.ConfigResult, error) {
	output, err := c.execute(ctx, device, credential, "show running-config")
	if err != nil {
		return domain.ConfigResult{}, err
	}
	return makeConfigResult(device, "running", output), nil
}

func (c *Client) GetStartupConfig(ctx context.Context, device domain.Device, credential domain.Credential) (domain.ConfigResult, error) {
	output, err := c.execute(ctx, device, credential, "show startup-config")
	if err != nil {
		return domain.ConfigResult{}, err
	}
	return makeConfigResult(device, "startup", output), nil
}

func (c *Client) GetFacts(ctx context.Context, device domain.Device, credential domain.Credential) (domain.DeviceFacts, error) {
	version, err := c.execute(ctx, device, credential, "show version")
	if err != nil {
		return domain.DeviceFacts{}, err
	}
	inventory, err := c.execute(ctx, device, credential, "show inventory")
	if err != nil {
		return domain.DeviceFacts{}, err
	}
	return parseFacts(version, inventory), nil
}

func (c *Client) GetInterfaces(ctx context.Context, device domain.Device, credential domain.Credential) ([]domain.InterfaceStatus, error) {
	output, err := c.execute(ctx, device, credential, "show interfaces status")
	if err != nil {
		return nil, err
	}
	return parseInterfaces(output), nil
}

func (c *Client) GetVLANs(context.Context, domain.Device, domain.Credential) ([]domain.VLAN, error) {
	return nil, fmt.Errorf("%w: Cisco VLAN parsing is not part of this milestone", domain.ErrUnsupported)
}

func (c *Client) GetNeighbors(context.Context, domain.Device, domain.Credential) ([]domain.Neighbor, error) {
	return nil, fmt.Errorf("%w: Cisco neighbor parsing is not part of this milestone", domain.ErrUnsupported)
}

func (c *Client) execute(ctx context.Context, device domain.Device, credential domain.Credential, command string) (string, error) {
	client, err := c.dial(ctx, device, credential)
	if err != nil {
		return "", err
	}
	defer client.Close()
	commandCtx, cancel := context.WithTimeout(ctx, c.CommandTimeout)
	defer cancel()
	type result struct {
		output string
		err    error
	}
	completed := make(chan result, 1)
	go func() {
		output, err := executeShell(commandCtx, client, credential.EnableSecret, command, c.MaxOutputBytes)
		completed <- result{output, err}
	}()
	select {
	case <-commandCtx.Done():
		_ = client.Close()
		return "", fmt.Errorf("%w: %v", domain.ErrTimeout, commandCtx.Err())
	case result := <-completed:
		return result.output, mapSSHError(result.err)
	}
}

func (c *Client) dial(ctx context.Context, device domain.Device, credential domain.Credential) (*ssh.Client, error) {
	if _, err := os.Stat(c.KnownHostsFile); err != nil {
		return nil, fmt.Errorf("%w: known-hosts file: %v", domain.ErrHostKey, err)
	}
	hostKeyCallback, err := knownhosts.New(c.KnownHostsFile)
	if err != nil {
		return nil, fmt.Errorf("%w: load known-hosts: %v", domain.ErrHostKey, err)
	}
	authMethod, err := sshAuthMethod(credential)
	if err != nil {
		return nil, err
	}
	configuration := &ssh.ClientConfig{User: credential.Username, Auth: []ssh.AuthMethod{authMethod}, HostKeyCallback: hostKeyCallback, Timeout: c.HandshakeTimeout}
	address := net.JoinHostPort(device.ManagementIP, fmt.Sprintf("%d", device.ManagementPort))
	netConn, err := (&net.Dialer{Timeout: c.ConnectTimeout}).DialContext(ctx, "tcp", address)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", domain.ErrConnection, err)
	}
	deadline := time.Now().Add(c.HandshakeTimeout)
	_ = netConn.SetDeadline(deadline)
	connection, channels, requests, err := ssh.NewClientConn(netConn, address, configuration)
	if err != nil {
		_ = netConn.Close()
		return nil, mapSSHError(err)
	}
	_ = netConn.SetDeadline(time.Time{})
	return ssh.NewClient(connection, channels, requests), nil
}

func sshAuthMethod(credential domain.Credential) (ssh.AuthMethod, error) {
	switch credential.AuthType {
	case "password":
		return ssh.Password(credential.Password), nil
	case "private_key":
		signer, err := ssh.ParsePrivateKey(credential.PrivateKey)
		if err != nil {
			return nil, fmt.Errorf("%w: invalid SSH private key", domain.ErrValidation)
		}
		return ssh.PublicKeys(signer), nil
	default:
		return nil, fmt.Errorf("%w: unsupported SSH credential type", domain.ErrValidation)
	}
}

func mapSSHError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, domain.ErrTimeout) || errors.Is(err, domain.ErrOutputTooLarge) || errors.Is(err, domain.ErrHostKey) || errors.Is(err, domain.ErrDeviceAuth) || errors.Is(err, domain.ErrConnection) {
		return err
	}
	var keyError *knownhosts.KeyError
	if errors.As(err, &keyError) {
		return fmt.Errorf("%w: host key did not match", domain.ErrHostKey)
	}
	message := strings.ToLower(err.Error())
	if strings.Contains(message, "unable to authenticate") || strings.Contains(message, "no supported methods remain") {
		return fmt.Errorf("%w: SSH authentication rejected", domain.ErrDeviceAuth)
	}
	if strings.Contains(message, "deadline exceeded") || strings.Contains(message, "i/o timeout") {
		return fmt.Errorf("%w: SSH operation timed out", domain.ErrTimeout)
	}
	return fmt.Errorf("%w: SSH operation failed", domain.ErrConnection)
}
