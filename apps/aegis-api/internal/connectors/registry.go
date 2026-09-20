package connectors

import (
	"fmt"
	"strings"
	"sync"

	"company.local/aegis/apps/aegis-api/internal/domain"
)

type Registry struct {
	mu         sync.RWMutex
	connectors map[string]DeviceConnector
}

func NewRegistry() *Registry { return &Registry{connectors: make(map[string]DeviceConnector)} }

func connectorKey(vendor, platform, protocol string) string {
	return strings.ToLower(strings.TrimSpace(vendor)) + "/" + strings.ToLower(strings.TrimSpace(platform)) + "/" + strings.ToLower(strings.TrimSpace(protocol))
}

func (r *Registry) Register(vendor, platform, protocol string, connector DeviceConnector) error {
	key := connectorKey(vendor, platform, protocol)
	if key == "//" || connector == nil {
		return fmt.Errorf("%w: invalid connector registration", domain.ErrValidation)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.connectors[key]; exists {
		return fmt.Errorf("%w: connector %s", domain.ErrConflict, key)
	}
	r.connectors[key] = connector
	return nil
}

func (r *Registry) Resolve(vendor, platform, protocol string) (DeviceConnector, error) {
	r.mu.RLock()
	connector, ok := r.connectors[connectorKey(vendor, platform, protocol)]
	r.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("%w: %s", domain.ErrUnsupported, connectorKey(vendor, platform, protocol))
	}
	return connector, nil
}
