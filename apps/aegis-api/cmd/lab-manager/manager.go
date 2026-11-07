package main

import (
	"errors"
	"log/slog"
	"net/http"
	"net/netip"
	"strings"
	"sync"
	"time"

	"company.local/aegis/apps/aegis-api/internal/api"
	"company.local/aegis/apps/aegis-api/internal/connectors"
	"company.local/aegis/apps/aegis-api/internal/connectors/cisco_ios_ssh"
	"company.local/aegis/apps/aegis-api/internal/credentials"
	"company.local/aegis/apps/aegis-api/internal/domain"
	"company.local/aegis/apps/aegis-api/internal/repositories"
	"company.local/aegis/apps/aegis-api/internal/repositories/sqlite"
	"company.local/aegis/apps/aegis-api/internal/services"
	"github.com/go-chi/chi/v5"
	"golang.org/x/crypto/ssh"
)

type labManager struct {
	*api.Handler
	ssh       *cisco_ios_ssh.Client
	store     *sqlite.Store
	mutations sync.Mutex
	testsMu   sync.RWMutex
	tests     map[string]connectionTest
}

type connectionTest struct {
	At   time.Time `json:"at"`
	Code string    `json:"code"`
}

type labDevice struct {
	domain.Device
	CredentialAssigned bool            `json:"credential_assigned"`
	TargetApproved     bool            `json:"target_approved"`
	HostKeyPinned      bool            `json:"host_key_pinned"`
	ConnectionReady    bool            `json:"connection_ready"`
	LastConnectionTest *connectionTest `json:"last_connection_test,omitempty"`
	LastBackupAt       *time.Time      `json:"last_backup_at,omitempty"`
}

func newManager(store *sqlite.Store, vault *credentials.Store, client *cisco_ios_ssh.Client, backupDir string) (*labManager, error) {
	registry := connectors.NewRegistry()
	for _, platform := range []string{"ios", "ios-xe"} {
		if err := registry.Register("cisco", platform, "ssh", client); err != nil {
			return nil, err
		}
	}
	configs := services.NewConfigService(store, vault, registry, 0, 0, 0, 2, 1)
	backups, err := services.NewBackupService(store, configs, backupDir, maxConfigBytes)
	if err != nil {
		return nil, err
	}
	handler := &api.Handler{
		Credentials: vault, Devices: services.NewDeviceService(store, vault, registry),
		Configs: configs, Backups: backups, Audits: store, Logger: slog.Default(),
	}
	return &labManager{Handler: handler, ssh: client, store: store, tests: make(map[string]connectionTest)}, nil
}

func (m *labManager) describe(r *http.Request, p domain.Principal, d domain.Device) (labDevice, error) {
	_, credErr := m.Credentials.Get(r.Context(), d.CredentialID)
	approved := m.ssh.Approved(d)
	item := labDevice{
		Device: d, CredentialAssigned: credErr == nil, TargetApproved: approved,
		HostKeyPinned: approved, ConnectionReady: d.Enabled && approved && credErr == nil,
	}
	// A removed approval disables the effective state even if it was saved under
	// an earlier Lab profile. The SSH connector also checks its own immutable pins.
	item.Enabled = item.ConnectionReady
	if !p.HasPermission("credentials.manage") {
		item.CredentialID = ""
	}
	m.testsMu.RLock()
	if test, ok := m.tests[d.ID]; ok {
		item.LastConnectionTest = &test
	}
	m.testsMu.RUnlock()
	backup, err := m.store.LatestBackup(r.Context(), d.ID, "running")
	if err == nil {
		item.LastBackupAt = &backup.CreatedAt
	} else if !errors.Is(err, domain.ErrNotFound) {
		return labDevice{}, err
	}
	return item, nil
}

func (m *labManager) listDevices(w http.ResponseWriter, r *http.Request) {
	p, _ := api.Principal(r.Context())
	devices, total, err := m.Devices.List(r.Context(), p, repositories.DeviceFilter{}, repositories.Page{Limit: 100})
	if err != nil || total > 100 {
		api.WriteError(w, r, errors.Join(err, domain.ErrConflict))
		return
	}
	items := make([]labDevice, 0, len(devices))
	for _, device := range devices {
		internal, err := m.Devices.GetInternal(r.Context(), p, device.ID)
		if err == nil {
			var item labDevice
			item, err = m.describe(r, p, internal)
			items = append(items, item)
		}
		if err != nil {
			api.WriteError(w, r, err)
			return
		}
	}
	api.WriteData(w, r, http.StatusOK, items)
}

type deviceInput struct {
	DisplayName    string `json:"display_name"`
	Hostname       string `json:"hostname"`
	ManagementIP   string `json:"management_ip"`
	ManagementPort int    `json:"management_port"`
	Vendor         string `json:"vendor"`
	Platform       string `json:"platform"`
	DeviceGroup    string `json:"device_group"`
	Description    string `json:"description"`
	CredentialID   string `json:"credential_id"`
}

func (m *labManager) saveDevice(w http.ResponseWriter, r *http.Request) {
	p, _ := api.Principal(r.Context())
	var input deviceInput
	if err := api.DecodeJSON(w, r, &input); err != nil {
		api.WriteError(w, r, err)
		return
	}
	ip, err := netip.ParseAddr(strings.TrimSpace(input.ManagementIP))
	if err != nil || ip.Zone() != "" || ip.IsUnspecified() || ip.IsMulticast() ||
		input.ManagementPort < 1 || input.ManagementPort > 65535 ||
		input.Vendor != "cisco" || (input.Platform != "ios" && input.Platform != "ios-xe") ||
		len(input.DisplayName) > 100 || len(input.Hostname) > 253 || len(input.DeviceGroup) > 80 || len(input.Description) > 600 {
		labProblem(w, http.StatusBadRequest, "INVALID_DEVICE", "Enter a literal management IP, an SSH port from 1 to 65535 and a supported Cisco IOS family.")
		return
	}
	id := chi.URLParam(r, "deviceID")
	device := domain.Device{
		ID: id, DisplayName: input.DisplayName, Hostname: input.Hostname, ManagementIP: ip.Unmap().String(),
		ManagementPort: input.ManagementPort, Vendor: input.Vendor, Platform: input.Platform, Protocol: "ssh",
		DeviceGroup: input.DeviceGroup, Description: input.Description, CredentialID: input.CredentialID,
	}
	device.Enabled = m.ssh.Approved(device)
	m.mutations.Lock()
	defer m.mutations.Unlock()
	status := http.StatusCreated
	if id == "" {
		_, total, listErr := m.Devices.List(r.Context(), p, repositories.DeviceFilter{}, repositories.Page{Limit: 1})
		if listErr != nil || total >= 100 {
			api.WriteError(w, r, errors.Join(listErr, domain.ErrConflict))
			return
		}
		device, err = m.Devices.Register(r.Context(), device)
	} else {
		status = http.StatusOK
		_, err = m.Devices.GetInternal(r.Context(), p, id)
		if err == nil {
			device, err = m.Devices.Update(r.Context(), device)
		}
		if err == nil {
			m.Configs.Invalidate(id)
			m.testsMu.Lock()
			delete(m.tests, id)
			m.testsMu.Unlock()
		}
	}
	if err != nil {
		if errors.Is(err, domain.ErrValidation) {
			labProblem(w, http.StatusBadRequest, "INVALID_DEVICE", "Enter a device name and select an existing Lab SSH credential.")
			return
		}
		api.WriteError(w, r, err)
		return
	}
	item, err := m.describe(r, p, device)
	if err != nil {
		api.WriteError(w, r, err)
		return
	}
	api.WriteData(w, r, status, item)
}

func (m *labManager) createCredential(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Name       string `json:"name"`
		Username   string `json:"username"`
		AuthType   string `json:"auth_type"`
		Password   string `json:"password"`
		PrivateKey string `json:"private_key"`
	}
	if err := api.DecodeJSON(w, r, &input); err != nil {
		api.WriteError(w, r, err)
		return
	}
	if len(input.Name) > 100 || len(input.Username) > 100 || len(input.Password) > 1024 || len(input.PrivateKey) > 32<<10 {
		labProblem(w, http.StatusBadRequest, "INVALID_CREDENTIAL", "The Lab credential exceeds the allowed field length.")
		return
	}
	key := []byte(input.PrivateKey)
	defer clear(key)
	valid := input.AuthType == "password" && input.Password != "" && len(key) == 0
	if input.AuthType == "private_key" && input.Password == "" {
		_, err := ssh.ParsePrivateKey(key)
		valid = err == nil
	}
	if !valid || strings.TrimSpace(input.Name) == "" || strings.TrimSpace(input.Username) == "" {
		labProblem(w, http.StatusBadRequest, "INVALID_CREDENTIAL", "Provide a label, SSH username and a password or supported unencrypted private key.")
		return
	}
	m.mutations.Lock()
	defer m.mutations.Unlock()
	_, total, err := m.Credentials.List(r.Context(), repositories.Page{Limit: 1})
	if err != nil || total >= 100 {
		api.WriteError(w, r, errors.Join(err, domain.ErrConflict))
		return
	}
	c, err := m.Credentials.Create(r.Context(), domain.Credential{
		Name: input.Name, Username: input.Username, AuthType: input.AuthType, Password: input.Password, PrivateKey: key,
	})
	if err != nil {
		api.WriteError(w, r, err)
		return
	}
	api.WriteData(w, r, http.StatusCreated, map[string]string{"id": c.ID, "name": c.Name, "auth_type": c.AuthType})
}

func (m *labManager) deviceAction(test bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		p, _ := api.Principal(r.Context())
		id := chi.URLParam(r, "deviceID")
		m.mutations.Lock()
		defer m.mutations.Unlock()
		d, err := m.Devices.GetInternal(r.Context(), p, id)
		if err != nil {
			api.WriteError(w, r, err)
			return
		}
		if !m.ssh.Approved(d) {
			labProblem(w, http.StatusForbidden, "TARGET_BLOCKED", "No independently verified host-key pin approves this exact IP and SSH port.")
			return
		}
		if !d.Enabled {
			labProblem(w, http.StatusConflict, "DEVICE_DISABLED", "Save the device under the approved Lab profile before connecting.")
			return
		}
		if test {
			err = m.Devices.TestConnection(r.Context(), p, id)
			result := connectionTest{At: time.Now().UTC(), Code: services.ErrorCode(err)}
			m.testsMu.Lock()
			m.tests[id] = result
			m.testsMu.Unlock()
			if err != nil {
				api.WriteError(w, r, err)
				return
			}
			api.WriteData(w, r, http.StatusOK, result)
			return
		}
		config, err := m.Configs.GetConfig(r.Context(), p, id, "running", true)
		if err != nil {
			api.WriteError(w, r, err)
			return
		}
		api.WriteData(w, r, http.StatusOK, config)
	}
}
