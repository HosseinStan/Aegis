package cisco_ios_ssh

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"company.local/aegis/apps/aegis-api/internal/domain"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

func TestClientUsesVerifiedHostKeyAndFixedShellOperation(t *testing.T) {
	server := newFakeCiscoServer(t, "version 17.9\nhostname OFFICE-SW1\nend\n")
	knownHosts := filepath.Join(t.TempDir(), "known_hosts")
	require.NoError(t, os.WriteFile(knownHosts, []byte(knownhosts.Line([]string{knownhosts.Normalize(server.address)}, server.hostKey.PublicKey())+"\n"), 0o600))
	client, err := New(knownHosts, time.Second, time.Second, time.Second, 1024*1024)
	require.NoError(t, err)
	host, portText, err := net.SplitHostPort(server.address)
	require.NoError(t, err)
	var port int
	_, err = fmt.Sscanf(portText, "%d", &port)
	require.NoError(t, err)
	result, err := client.GetRunningConfig(context.Background(), domain.Device{ID: "device-1", ManagementIP: host, ManagementPort: port, Vendor: "cisco", Platform: "ios", Protocol: "ssh"}, domain.Credential{AuthType: "password", Username: "aegis", Password: "device-password"})
	require.NoError(t, err)
	require.Contains(t, result.Config, "hostname OFFICE-SW1")
	require.NotContains(t, result.Config, "show running-config")
}

func TestClientRejectsHostKeyMismatch(t *testing.T) {
	server := newFakeCiscoServer(t, "version 17.9\n")
	wrongKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	wrongSigner, err := ssh.NewSignerFromKey(wrongKey)
	require.NoError(t, err)
	knownHosts := filepath.Join(t.TempDir(), "known_hosts")
	require.NoError(t, os.WriteFile(knownHosts, []byte(knownhosts.Line([]string{knownhosts.Normalize(server.address)}, wrongSigner.PublicKey())+"\n"), 0o600))
	client, err := New(knownHosts, time.Second, time.Second, time.Second, 1024)
	require.NoError(t, err)
	host, portText, _ := net.SplitHostPort(server.address)
	var port int
	_, _ = fmt.Sscanf(portText, "%d", &port)
	err = client.TestConnection(context.Background(), domain.Device{ManagementIP: host, ManagementPort: port}, domain.Credential{AuthType: "password", Username: "aegis", Password: "device-password"})
	require.ErrorIs(t, err, domain.ErrHostKey)
}

func TestClientEnforcesOutputLimit(t *testing.T) {
	server := newFakeCiscoServer(t, strings.Repeat("hostname oversized\n", 100))
	knownHosts := filepath.Join(t.TempDir(), "known_hosts")
	require.NoError(t, os.WriteFile(knownHosts, []byte(knownhosts.Line([]string{knownhosts.Normalize(server.address)}, server.hostKey.PublicKey())+"\n"), 0o600))
	client, err := New(knownHosts, time.Second, time.Second, time.Second, 128)
	require.NoError(t, err)
	host, portText, _ := net.SplitHostPort(server.address)
	var port int
	_, _ = fmt.Sscanf(portText, "%d", &port)
	_, err = client.GetRunningConfig(context.Background(), domain.Device{ManagementIP: host, ManagementPort: port, Vendor: "cisco", Platform: "ios", Protocol: "ssh"}, domain.Credential{AuthType: "password", Username: "aegis", Password: "device-password"})
	require.ErrorIs(t, err, domain.ErrOutputTooLarge)
}

type fakeCiscoServer struct {
	address  string
	hostKey  ssh.Signer
	listener net.Listener
}

func newFakeCiscoServer(t *testing.T, runningConfig string) *fakeCiscoServer {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	signer, err := ssh.NewSignerFromKey(key)
	require.NoError(t, err)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	server := &fakeCiscoServer{address: listener.Addr().String(), hostKey: signer, listener: listener}
	t.Cleanup(func() { _ = listener.Close() })
	config := &ssh.ServerConfig{PasswordCallback: func(metadata ssh.ConnMetadata, password []byte) (*ssh.Permissions, error) {
		if metadata.User() == "aegis" && string(password) == "device-password" {
			return nil, nil
		}
		return nil, errors.New("authentication rejected")
	}}
	config.AddHostKey(signer)
	go func() {
		for {
			connection, err := listener.Accept()
			if err != nil {
				return
			}
			go serveFakeCisco(connection, config, runningConfig)
		}
	}()
	return server
}

func serveFakeCisco(connection net.Conn, config *ssh.ServerConfig, runningConfig string) {
	serverConnection, channels, requests, err := ssh.NewServerConn(connection, config)
	if err != nil {
		_ = connection.Close()
		return
	}
	defer serverConnection.Close()
	go ssh.DiscardRequests(requests)
	for request := range channels {
		if request.ChannelType() != "session" {
			_ = request.Reject(ssh.UnknownChannelType, "session required")
			continue
		}
		channel, channelRequests, err := request.Accept()
		if err != nil {
			return
		}
		go func() {
			defer channel.Close()
			for request := range channelRequests {
				switch request.Type {
				case "pty-req":
					_ = request.Reply(true, nil)
				case "shell":
					_ = request.Reply(true, nil)
					_, _ = channel.Write([]byte("Authorized access only\r\nOFFICE-SW1#"))
					scanner := bufio.NewScanner(channel)
					for scanner.Scan() {
						command := strings.TrimSpace(scanner.Text())
						switch command {
						case "terminal length 0":
							_, _ = channel.Write([]byte("terminal length 0\r\nOFFICE-SW1#"))
						case "show running-config":
							_, _ = channel.Write([]byte("show running-config\r\n" + strings.ReplaceAll(runningConfig, "\n", "\r\n") + "OFFICE-SW1#"))
						default:
							_, _ = channel.Write([]byte("% Invalid input\r\nOFFICE-SW1#"))
						}
					}
					return
				default:
					_ = request.Reply(false, nil)
				}
			}
		}()
	}
}

func TestLabPinnedConnectorRejectsUnapprovedEndpointsAndWrongKeys(t *testing.T) {
	server := newFakeCiscoServer(t, "hostname SYNTHETIC-LAB\nend\n")
	target := netip.MustParseAddrPort(server.address)
	device := domain.Device{ManagementIP: target.Addr().String(), ManagementPort: int(target.Port())}
	credential := domain.Credential{AuthType: "password", Username: "aegis", Password: "device-password"}
	ctx := context.Background()

	blocked, err := NewPinned(nil, time.Second, time.Second, time.Second, 1024)
	require.NoError(t, err)
	require.False(t, blocked.Approved(device))
	require.ErrorIs(t, blocked.TestConnection(ctx, device, credential), domain.ErrForbidden)
	_, err = blocked.GetRunningConfig(ctx, device, credential)
	require.ErrorIs(t, err, domain.ErrForbidden)

	wrong, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	signer, err := ssh.NewSignerFromKey(wrong)
	require.NoError(t, err)
	client, err := NewPinned(map[netip.AddrPort]ssh.PublicKey{target: signer.PublicKey()}, time.Second, time.Second, time.Second, 1024)
	require.NoError(t, err)
	require.ErrorIs(t, client.TestConnection(ctx, device, credential), domain.ErrHostKey)

	pins := map[netip.AddrPort]ssh.PublicKey{target: server.hostKey.PublicKey()}
	client, err = NewPinned(pins, time.Second, time.Second, time.Second, 1024)
	require.NoError(t, err)
	delete(pins, target)
	require.True(t, client.Approved(device))
	require.NoError(t, client.TestConnection(ctx, device, credential))
	result, err := client.GetRunningConfig(ctx, device, credential)
	require.NoError(t, err)
	require.Contains(t, result.Config, "hostname SYNTHETIC-LAB")
	if device.ManagementPort == 65535 {
		device.ManagementPort--
	} else {
		device.ManagementPort++
	}
	require.ErrorIs(t, client.TestConnection(ctx, device, credential), domain.ErrForbidden)
}
