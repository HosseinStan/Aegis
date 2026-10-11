package domain

import "time"

type Device struct {
	ID             string     `json:"id"`
	DisplayName    string     `json:"display_name"`
	Hostname       string     `json:"hostname,omitempty"`
	ManagementIP   string     `json:"management_ip"`
	ManagementPort int        `json:"management_port"`
	Vendor         string     `json:"vendor"`
	Platform       string     `json:"platform"`
	Protocol       string     `json:"protocol"`
	CredentialID   string     `json:"credential_id,omitempty"`
	DeviceGroup    string     `json:"device_group,omitempty"`
	Description    string     `json:"description,omitempty"`
	Enabled        bool       `json:"enabled"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
	LastSuccessAt  *time.Time `json:"last_success_at,omitempty"`
	LastFailureAt  *time.Time `json:"last_failure_at,omitempty"`
	LastErrorCode  string     `json:"last_error_code,omitempty"`
}

type Credential struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	AuthType     string    `json:"auth_type"`
	Username     string    `json:"-"`
	Password     string    `json:"-"`
	PrivateKey   []byte    `json:"-"`
	EnableSecret string    `json:"-"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type User struct {
	ID              string    `json:"id"`
	ExternalSubject string    `json:"external_subject,omitempty"`
	Username        string    `json:"username"`
	DisplayName     string    `json:"display_name,omitempty"`
	Email           string    `json:"email,omitempty"`
	PasswordHash    []byte    `json:"-"`
	Enabled         bool      `json:"enabled"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

type Principal struct {
	UserID       string   `json:"user_id"`
	Username     string   `json:"username"`
	Permissions  []string `json:"permissions"`
	DeviceGroups []string `json:"device_groups,omitempty"`
}

func (p Principal) HasPermission(permission string) bool {
	for _, candidate := range p.Permissions {
		if candidate == permission || candidate == "*" {
			return true
		}
	}
	return false
}

func (p Principal) CanAccessGroup(group string) bool {
	if group == "" || p.HasPermission("system.manage") {
		return true
	}
	for _, candidate := range p.DeviceGroups {
		if candidate == "*" || candidate == group {
			return true
		}
	}
	return len(p.DeviceGroups) == 0
}

type AuditLog struct {
	ID           string    `json:"id"`
	RequestID    string    `json:"request_id"`
	UserID       string    `json:"user_id,omitempty"`
	Username     string    `json:"username,omitempty"`
	SourceIP     string    `json:"source_ip,omitempty"`
	DeviceID     string    `json:"device_id,omitempty"`
	Action       string    `json:"action"`
	ResourceType string    `json:"resource_type,omitempty"`
	ResourceID   string    `json:"resource_id,omitempty"`
	Succeeded    bool      `json:"succeeded"`
	ErrorCode    string    `json:"error_code,omitempty"`
	DurationMS   int64     `json:"duration_ms"`
	CreatedAt    time.Time `json:"created_at"`
}

type ConfigBackup struct {
	ID          string    `json:"id"`
	DeviceID    string    `json:"device_id"`
	ConfigType  string    `json:"config_type"`
	StoragePath string    `json:"-"`
	ContentHash string    `json:"content_hash"`
	SizeBytes   int64     `json:"size_bytes"`
	TriggerType string    `json:"trigger_type"`
	TriggeredBy string    `json:"triggered_by,omitempty"`
	RetrievedAt time.Time `json:"retrieved_at"`
	CreatedAt   time.Time `json:"created_at"`
}

const (
	AuthorizationStatusUnaccounted = "unaccounted"
	AuthorizationStatusAuthorized  = "authorized"
	PolicyStatusNotEvaluated       = "not_evaluated"
	PolicyStatusPass               = "pass"
	PolicyStatusViolation          = "violation"
	ChangeOperationAdd             = "add"
	ChangeOperationRemove          = "remove"
	ChangeAnalysisVersion          = 1
)

type ChangeEvent struct {
	ID                  string     `json:"id"`
	DeviceID            string     `json:"device_id"`
	ConfigType          string     `json:"config_type"`
	FromBackupID        string     `json:"from_backup_id"`
	ToBackupID          string     `json:"to_backup_id"`
	DetectedAt          time.Time  `json:"detected_at"`
	AddedLines          int        `json:"added_lines"`
	RemovedLines        int        `json:"removed_lines"`
	AuthorizationStatus string     `json:"authorization_status"`
	PolicyStatus        string     `json:"policy_status"`
	CreatedAt           time.Time  `json:"created_at"`
	UpdatedAt           time.Time  `json:"updated_at"`
	AnalysisVersion     int        `json:"analysis_version"`
	AnalyzedAt          *time.Time `json:"analyzed_at,omitempty"`
	ItemCount           int        `json:"item_count"`
}

type ChangeEventItem struct {
	ID                    string     `json:"id"`
	ChangeEventID         string     `json:"change_event_id"`
	Ordinal               int        `json:"ordinal"`
	Operation             string     `json:"operation"`
	ContextPath           []string   `json:"context_path"`
	Command               string     `json:"command"`
	Sensitive             bool       `json:"sensitive"`
	CreatedAt             time.Time  `json:"created_at"`
	ClassificationVersion int        `json:"classification_version"`
	Category              string     `json:"category"`
	Subcategory           string     `json:"subcategory"`
	ClassificationRule    string     `json:"classification_rule"`
	ClassifiedAt          *time.Time `json:"classified_at,omitempty"`
}

type ConfigResult struct {
	DeviceID          string    `json:"device_id"`
	Hostname          string    `json:"hostname,omitempty"`
	Vendor            string    `json:"vendor"`
	Platform          string    `json:"platform"`
	Protocol          string    `json:"protocol"`
	ConfigurationType string    `json:"configuration_type"`
	RetrievedAt       time.Time `json:"retrieved_at"`
	FromCache         bool      `json:"from_cache"`
	ContentHash       string    `json:"content_hash"`
	Config            string    `json:"config"`
}

type DeviceFacts struct {
	Hostname     string `json:"hostname,omitempty"`
	Model        string `json:"model,omitempty"`
	SerialNumber string `json:"serial_number,omitempty"`
	Software     string `json:"software,omitempty"`
	Version      string `json:"version,omitempty"`
	Uptime       string `json:"uptime,omitempty"`
	RawSummary   string `json:"raw_summary,omitempty"`
}

type InterfaceStatus struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Status      string `json:"status,omitempty"`
	VLAN        string `json:"vlan,omitempty"`
	Duplex      string `json:"duplex,omitempty"`
	Speed       string `json:"speed,omitempty"`
	Type        string `json:"type,omitempty"`
}

type VLAN struct {
	ID     int    `json:"id"`
	Name   string `json:"name"`
	Status string `json:"status,omitempty"`
}

type Neighbor struct {
	LocalInterface  string `json:"local_interface,omitempty"`
	RemoteDevice    string `json:"remote_device,omitempty"`
	RemoteInterface string `json:"remote_interface,omitempty"`
	ManagementIP    string `json:"management_ip,omitempty"`
	Protocol        string `json:"protocol,omitempty"`
}
