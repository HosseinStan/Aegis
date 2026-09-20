package mock

import (
	"context"
	"crypto/sha256"
	"fmt"
	"sync"
	"time"

	"company.local/aegis/apps/aegis-api/internal/domain"
)

type Connector struct {
	mu         sync.Mutex
	Running    string
	Startup    string
	Facts      domain.DeviceFacts
	Interfaces []domain.InterfaceStatus
	Err        error
	Calls      int
}

func (c *Connector) call() error             { c.mu.Lock(); defer c.mu.Unlock(); c.Calls++; return c.Err }
func (c *Connector) CallCount() int          { c.mu.Lock(); defer c.mu.Unlock(); return c.Calls }
func (c *Connector) SetRunning(value string) { c.mu.Lock(); c.Running = value; c.mu.Unlock() }
func (c *Connector) SetError(err error)      { c.mu.Lock(); c.Err = err; c.mu.Unlock() }
func (c *Connector) TestConnection(context.Context, domain.Device, domain.Credential) error {
	return c.call()
}
func (c *Connector) config(device domain.Device, kind, content string) (domain.ConfigResult, error) {
	if err := c.call(); err != nil {
		return domain.ConfigResult{}, err
	}
	hash := sha256.Sum256([]byte(content))
	return domain.ConfigResult{DeviceID: device.ID, Hostname: device.Hostname, Vendor: device.Vendor, Platform: device.Platform, Protocol: device.Protocol, ConfigurationType: kind, RetrievedAt: time.Now().UTC(), ContentHash: fmt.Sprintf("%x", hash), Config: content}, nil
}
func (c *Connector) GetRunningConfig(_ context.Context, d domain.Device, _ domain.Credential) (domain.ConfigResult, error) {
	c.mu.Lock()
	content := c.Running
	c.mu.Unlock()
	return c.config(d, "running", content)
}
func (c *Connector) GetStartupConfig(_ context.Context, d domain.Device, _ domain.Credential) (domain.ConfigResult, error) {
	return c.config(d, "startup", c.Startup)
}
func (c *Connector) GetFacts(context.Context, domain.Device, domain.Credential) (domain.DeviceFacts, error) {
	if err := c.call(); err != nil {
		return domain.DeviceFacts{}, err
	}
	return c.Facts, nil
}
func (c *Connector) GetInterfaces(context.Context, domain.Device, domain.Credential) ([]domain.InterfaceStatus, error) {
	if err := c.call(); err != nil {
		return nil, err
	}
	return c.Interfaces, nil
}
func (c *Connector) GetVLANs(context.Context, domain.Device, domain.Credential) ([]domain.VLAN, error) {
	return nil, domain.ErrUnsupported
}
func (c *Connector) GetNeighbors(context.Context, domain.Device, domain.Credential) ([]domain.Neighbor, error) {
	return nil, domain.ErrUnsupported
}
