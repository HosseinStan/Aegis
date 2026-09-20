package services

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"company.local/aegis/apps/aegis-api/internal/connectors"
	"company.local/aegis/apps/aegis-api/internal/credentials"
	"company.local/aegis/apps/aegis-api/internal/domain"
	"company.local/aegis/apps/aegis-api/internal/observability"
	"company.local/aegis/apps/aegis-api/internal/repositories"
	"golang.org/x/sync/singleflight"
)

type cacheEntry struct {
	value     any
	expiresAt time.Time
}

type ConfigService struct {
	devices       repositories.DeviceRepository
	credentials   *credentials.Store
	connectors    *connectors.Registry
	configTTL     time.Duration
	factsTTL      time.Duration
	interfacesTTL time.Duration
	global        chan struct{}
	perDeviceMax  int
	locksMu       sync.Mutex
	locks         map[string]chan struct{}
	cacheMu       sync.RWMutex
	cache         map[string]cacheEntry
	flight        singleflight.Group
	metrics       *observability.Metrics
}

func NewConfigService(devices repositories.DeviceRepository, credentialStore *credentials.Store, registry *connectors.Registry, configTTL, factsTTL, interfacesTTL time.Duration, globalMax, perDeviceMax int, metricSet ...*observability.Metrics) *ConfigService {
	var metrics *observability.Metrics
	if len(metricSet) > 0 {
		metrics = metricSet[0]
	}
	return &ConfigService{devices: devices, credentials: credentialStore, connectors: registry, configTTL: configTTL, factsTTL: factsTTL, interfacesTTL: interfacesTTL, global: make(chan struct{}, globalMax), perDeviceMax: perDeviceMax, locks: make(map[string]chan struct{}), cache: make(map[string]cacheEntry), metrics: metrics}
}

func (s *ConfigService) GetConfig(ctx context.Context, principal domain.Principal, deviceID, kind string, bypass bool) (domain.ConfigResult, error) {
	if kind != "running" && kind != "startup" {
		return domain.ConfigResult{}, fmt.Errorf("%w: unknown configuration type", domain.ErrValidation)
	}
	key := deviceID + ":config:" + kind
	if !bypass {
		if value, ok := s.cached(key); ok {
			s.cacheHit()
			result := value.(domain.ConfigResult)
			result.FromCache = true
			return result, nil
		}
	}
	s.cacheMiss()
	value, err, _ := s.flight.Do(key, func() (any, error) {
		if !bypass {
			if value, ok := s.cached(key); ok {
				result := value.(domain.ConfigResult)
				result.FromCache = true
				return result, nil
			}
		}
		d, credential, connector, err := s.resolve(ctx, principal, deviceID)
		if err != nil {
			return nil, err
		}
		if err := s.acquire(ctx, deviceID); err != nil {
			return nil, err
		}
		defer s.release(deviceID)
		var result domain.ConfigResult
		if kind == "running" {
			result, err = connector.GetRunningConfig(ctx, d, credential)
		} else {
			result, err = connector.GetStartupConfig(ctx, d, credential)
		}
		s.recordDeviceAttempt(err)
		_ = s.devices.RecordDeviceResult(ctx, deviceID, ErrorCode(err))
		if err == nil {
			s.put(key, result, s.configTTL)
		}
		return result, err
	})
	if err != nil {
		return domain.ConfigResult{}, err
	}
	return value.(domain.ConfigResult), nil
}

func (s *ConfigService) GetFacts(ctx context.Context, principal domain.Principal, deviceID string, bypass bool) (domain.DeviceFacts, bool, error) {
	key := deviceID + ":facts"
	if !bypass {
		if value, ok := s.cached(key); ok {
			s.cacheHit()
			return value.(domain.DeviceFacts), true, nil
		}
	}
	s.cacheMiss()
	value, err, _ := s.flight.Do(key, func() (any, error) {
		d, credential, connector, err := s.resolve(ctx, principal, deviceID)
		if err != nil {
			return nil, err
		}
		if err := s.acquire(ctx, deviceID); err != nil {
			return nil, err
		}
		defer s.release(deviceID)
		facts, err := connector.GetFacts(ctx, d, credential)
		s.recordDeviceAttempt(err)
		_ = s.devices.RecordDeviceResult(ctx, deviceID, ErrorCode(err))
		if err == nil {
			s.put(key, facts, s.factsTTL)
		}
		return facts, err
	})
	if err != nil {
		return domain.DeviceFacts{}, false, err
	}
	return value.(domain.DeviceFacts), false, nil
}

func (s *ConfigService) GetInterfaces(ctx context.Context, principal domain.Principal, deviceID string, bypass bool) ([]domain.InterfaceStatus, bool, error) {
	key := deviceID + ":interfaces"
	if !bypass {
		if value, ok := s.cached(key); ok {
			s.cacheHit()
			return value.([]domain.InterfaceStatus), true, nil
		}
	}
	s.cacheMiss()
	value, err, _ := s.flight.Do(key, func() (any, error) {
		d, credential, connector, err := s.resolve(ctx, principal, deviceID)
		if err != nil {
			return nil, err
		}
		if err := s.acquire(ctx, deviceID); err != nil {
			return nil, err
		}
		defer s.release(deviceID)
		items, err := connector.GetInterfaces(ctx, d, credential)
		s.recordDeviceAttempt(err)
		_ = s.devices.RecordDeviceResult(ctx, deviceID, ErrorCode(err))
		if err == nil {
			s.put(key, items, s.interfacesTTL)
		}
		return items, err
	})
	if err != nil {
		return nil, false, err
	}
	return value.([]domain.InterfaceStatus), false, nil
}

func (s *ConfigService) Invalidate(deviceID string) {
	s.cacheMu.Lock()
	defer s.cacheMu.Unlock()
	for key := range s.cache {
		if strings.HasPrefix(key, deviceID+":") {
			delete(s.cache, key)
		}
	}
}

func (s *ConfigService) resolve(ctx context.Context, principal domain.Principal, deviceID string) (domain.Device, domain.Credential, connectors.DeviceConnector, error) {
	d, err := s.devices.GetDevice(ctx, deviceID)
	if err != nil {
		return domain.Device{}, domain.Credential{}, nil, deviceError(err)
	}
	if !principal.CanAccessGroup(d.DeviceGroup) {
		return domain.Device{}, domain.Credential{}, nil, domain.ErrForbidden
	}
	if !d.Enabled {
		return domain.Device{}, domain.Credential{}, nil, domain.ErrDeviceDisabled
	}
	credential, err := s.credentials.Get(ctx, d.CredentialID)
	if err != nil {
		return domain.Device{}, domain.Credential{}, nil, err
	}
	connector, err := s.connectors.Resolve(d.Vendor, d.Platform, d.Protocol)
	if err != nil {
		return domain.Device{}, domain.Credential{}, nil, err
	}
	return d, credential, connector, nil
}

func (s *ConfigService) acquire(ctx context.Context, deviceID string) error {
	s.locksMu.Lock()
	lock := s.locks[deviceID]
	if lock == nil {
		lock = make(chan struct{}, s.perDeviceMax)
		s.locks[deviceID] = lock
	}
	s.locksMu.Unlock()
	select {
	case s.global <- struct{}{}:
	case <-ctx.Done():
		return fmt.Errorf("%w: %v", domain.ErrTimeout, ctx.Err())
	}
	select {
	case lock <- struct{}{}:
		if s.metrics != nil {
			s.metrics.ActiveConnections.Add(1)
		}
		return nil
	case <-ctx.Done():
		<-s.global
		return fmt.Errorf("%w: %v", domain.ErrTimeout, ctx.Err())
	}
}

func (s *ConfigService) release(deviceID string) {
	s.locksMu.Lock()
	lock := s.locks[deviceID]
	s.locksMu.Unlock()
	<-lock
	<-s.global
	if s.metrics != nil {
		s.metrics.ActiveConnections.Add(-1)
	}
}
func (s *ConfigService) cached(key string) (any, bool) {
	s.cacheMu.RLock()
	entry, ok := s.cache[key]
	s.cacheMu.RUnlock()
	if !ok || time.Now().After(entry.expiresAt) {
		if ok {
			s.cacheMu.Lock()
			delete(s.cache, key)
			s.cacheMu.Unlock()
		}
		return nil, false
	}
	return entry.value, true
}
func (s *ConfigService) put(key string, value any, ttl time.Duration) {
	s.cacheMu.Lock()
	s.cache[key] = cacheEntry{value: value, expiresAt: time.Now().Add(ttl)}
	s.cacheMu.Unlock()
}

func (s *ConfigService) cacheHit() {
	if s.metrics != nil {
		s.metrics.CacheHits.Add(1)
	}
}
func (s *ConfigService) cacheMiss() {
	if s.metrics != nil {
		s.metrics.CacheMisses.Add(1)
	}
}
func (s *ConfigService) recordDeviceAttempt(err error) {
	if s.metrics != nil {
		s.metrics.DeviceAttempts.Add(1)
		if err != nil {
			s.metrics.DeviceFailures.Add(1)
		}
	}
}
