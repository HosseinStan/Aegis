package connectors

import (
	"context"

	"company.local/aegis/apps/aegis-api/internal/domain"
)

type DeviceConnector interface {
	TestConnection(context.Context, domain.Device, domain.Credential) error
	GetRunningConfig(context.Context, domain.Device, domain.Credential) (domain.ConfigResult, error)
	GetStartupConfig(context.Context, domain.Device, domain.Credential) (domain.ConfigResult, error)
	GetFacts(context.Context, domain.Device, domain.Credential) (domain.DeviceFacts, error)
	GetInterfaces(context.Context, domain.Device, domain.Credential) ([]domain.InterfaceStatus, error)
	GetVLANs(context.Context, domain.Device, domain.Credential) ([]domain.VLAN, error)
	GetNeighbors(context.Context, domain.Device, domain.Credential) ([]domain.Neighbor, error)
}
