package services

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

	"company.local/aegis/apps/aegis-api/internal/connectors"
	"company.local/aegis/apps/aegis-api/internal/credentials"
	"company.local/aegis/apps/aegis-api/internal/domain"
	"company.local/aegis/apps/aegis-api/internal/observability"
	"company.local/aegis/apps/aegis-api/internal/repositories"
	"github.com/google/uuid"
)

type DeviceService struct {
	repository  repositories.DeviceRepository
	credentials *credentials.Store
	connectors  *connectors.Registry
	metrics     *observability.Metrics
}

func NewDeviceService(repository repositories.DeviceRepository, credentialStore *credentials.Store, registry *connectors.Registry, metricSet ...*observability.Metrics) *DeviceService {
	var metrics *observability.Metrics
	if len(metricSet) > 0 {
		metrics = metricSet[0]
	}
	return &DeviceService{repository: repository, credentials: credentialStore, connectors: registry, metrics: metrics}
}

func (s *DeviceService) Create(ctx context.Context, d domain.Device) (domain.Device, error) {
	d = normalizeDevice(d)
	d.ID = uuid.NewString()
	d.Enabled = true
	d.CreatedAt = time.Now().UTC()
	d.UpdatedAt = d.CreatedAt
	if err := s.validate(ctx, d); err != nil {
		return domain.Device{}, err
	}
	if err := s.repository.CreateDevice(ctx, d); err != nil {
		return domain.Device{}, err
	}
	return d, nil
}

func (s *DeviceService) Update(ctx context.Context, d domain.Device) (domain.Device, error) {
	d = normalizeDevice(d)
	existing, err := s.repository.GetDevice(ctx, d.ID)
	if err != nil {
		return domain.Device{}, deviceError(err)
	}
	d.CreatedAt = existing.CreatedAt
	d.UpdatedAt = time.Now().UTC()
	if err := s.validate(ctx, d); err != nil {
		return domain.Device{}, err
	}
	if err := s.repository.UpdateDevice(ctx, d); err != nil {
		return domain.Device{}, err
	}
	return d, nil
}

func (s *DeviceService) Get(ctx context.Context, principal domain.Principal, id string) (domain.Device, error) {
	d, err := s.repository.GetDevice(ctx, id)
	if err != nil {
		return domain.Device{}, deviceError(err)
	}
	if !principal.CanAccessGroup(d.DeviceGroup) {
		return domain.Device{}, domain.ErrForbidden
	}
	return redactDevice(d, principal), nil
}

func (s *DeviceService) GetInternal(ctx context.Context, principal domain.Principal, id string) (domain.Device, error) {
	d, err := s.repository.GetDevice(ctx, id)
	if err != nil {
		return domain.Device{}, deviceError(err)
	}
	if !principal.CanAccessGroup(d.DeviceGroup) {
		return domain.Device{}, domain.ErrForbidden
	}
	return d, nil
}

func (s *DeviceService) List(ctx context.Context, principal domain.Principal, filter repositories.DeviceFilter, page repositories.Page) ([]domain.Device, int, error) {
	if len(principal.DeviceGroups) == 0 || principal.HasPermission("system.manage") {
		devices, total, err := s.repository.ListDevices(ctx, filter, page)
		if err != nil {
			return nil, 0, err
		}
		for index := range devices {
			devices[index] = redactDevice(devices[index], principal)
		}
		return devices, total, nil
	}
	devices, _, err := s.repository.ListDevices(ctx, filter, repositories.Page{Limit: 100000, Offset: 0})
	if err != nil {
		return nil, 0, err
	}
	visible := make([]domain.Device, 0, len(devices))
	for _, d := range devices {
		if principal.CanAccessGroup(d.DeviceGroup) {
			visible = append(visible, redactDevice(d, principal))
		}
	}
	total := len(visible)
	start := page.Offset
	if start > total {
		start = total
	}
	end := start + page.Limit
	if end > total {
		end = total
	}
	return visible[start:end], total, nil
}

func (s *DeviceService) Delete(ctx context.Context, id string) error {
	return s.repository.DeleteDevice(ctx, id)
}

func (s *DeviceService) TestConnection(ctx context.Context, principal domain.Principal, id string) error {
	d, err := s.GetInternal(ctx, principal, id)
	if err != nil {
		return err
	}
	if !d.Enabled {
		return domain.ErrDeviceDisabled
	}
	credential, err := s.credentials.Get(ctx, d.CredentialID)
	if err != nil {
		return err
	}
	connector, err := s.connectors.Resolve(d.Vendor, d.Platform, d.Protocol)
	if err != nil {
		return err
	}
	if s.metrics != nil {
		s.metrics.DeviceAttempts.Add(1)
		s.metrics.ActiveConnections.Add(1)
		defer s.metrics.ActiveConnections.Add(-1)
	}
	err = connector.TestConnection(ctx, d, credential)
	if err != nil && s.metrics != nil {
		s.metrics.DeviceFailures.Add(1)
	}
	_ = s.repository.RecordDeviceResult(ctx, d.ID, ErrorCode(err))
	return err
}

func (s *DeviceService) validate(ctx context.Context, d domain.Device) error {
	if d.DisplayName == "" || net.ParseIP(d.ManagementIP) == nil {
		return fmt.Errorf("%w: display_name and a valid management_ip are required", domain.ErrValidation)
	}
	if d.ManagementPort < 1 || d.ManagementPort > 65535 {
		return fmt.Errorf("%w: management_port must be between 1 and 65535", domain.ErrValidation)
	}
	if d.Vendor == "" || d.Platform == "" || d.Protocol == "" || d.CredentialID == "" {
		return fmt.Errorf("%w: vendor, platform, protocol, and credential_id are required", domain.ErrValidation)
	}
	if _, err := s.credentials.Get(ctx, d.CredentialID); err != nil {
		return fmt.Errorf("%w: credential does not exist", domain.ErrValidation)
	}
	if _, err := s.connectors.Resolve(d.Vendor, d.Platform, d.Protocol); err != nil {
		return err
	}
	return nil
}

func normalizeDevice(d domain.Device) domain.Device {
	d.DisplayName = strings.TrimSpace(d.DisplayName)
	d.Hostname = strings.TrimSpace(d.Hostname)
	d.ManagementIP = strings.TrimSpace(d.ManagementIP)
	d.Vendor = strings.ToLower(strings.TrimSpace(d.Vendor))
	d.Platform = strings.ToLower(strings.TrimSpace(d.Platform))
	d.Protocol = strings.ToLower(strings.TrimSpace(d.Protocol))
	d.CredentialID = strings.TrimSpace(d.CredentialID)
	d.DeviceGroup = strings.TrimSpace(d.DeviceGroup)
	d.Description = strings.TrimSpace(d.Description)
	return d
}

func redactDevice(d domain.Device, principal domain.Principal) domain.Device {
	if !principal.HasPermission("credentials.manage") {
		d.CredentialID = ""
	}
	return d
}

func deviceError(err error) error {
	if errors.Is(err, domain.ErrNotFound) {
		return domain.ErrDeviceNotFound
	}
	return err
}
