package domain

import (
	"slices"
	"strings"
	"time"
)

const (
	ChangeAuthorizationPending         = "pending"
	ChangeAuthorizationApproved        = "approved"
	ChangeAuthorizationRejected        = "rejected"
	ChangeAuthorizationRevoked         = "revoked"
	ChangeAuthorizationRequestedAction = "requested"
	ChangeAuthorizationApprovedAction  = "approved"
	ChangeAuthorizationRejectedAction  = "rejected"
	ChangeAuthorizationRevokedAction   = "revoked"
	ChangeAuthorizationLinkedAction    = "linked"
	ChangeAuthorizationAttestation     = "reviewed_change_items_against_ticket"
	PermissionAuthorizationRead        = "changes.authorization.read"
	PermissionAuthorizationRequest     = "changes.authorization.request"
	PermissionAuthorizationApprove     = "changes.authorization.approve"
	PermissionAuthorizationReconcile   = "changes.authorization.reconcile"
	MaxChangeAuthorizationWindow       = 24 * time.Hour
)

type ChangeAuthorizationInput struct {
	Reference         string    `json:"reference"`
	ConfigType        string    `json:"config_type"`
	AllowedCategories []string  `json:"allowed_categories"`
	WindowStart       time.Time `json:"window_start"`
	WindowEnd         time.Time `json:"window_end"`
}

type ChangeAuthorization struct {
	ID                string     `json:"id"`
	Reference         string     `json:"reference"`
	DeviceID          string     `json:"device_id"`
	ConfigType        string     `json:"config_type"`
	AllowedCategories []string   `json:"allowed_categories"`
	WindowStart       time.Time  `json:"window_start"`
	WindowEnd         time.Time  `json:"window_end"`
	Status            string     `json:"status"`
	RequestedBy       string     `json:"requested_by"`
	RequestedAt       time.Time  `json:"requested_at"`
	ApprovedBy        string     `json:"approved_by,omitempty"`
	ApprovedAt        *time.Time `json:"approved_at,omitempty"`
	RejectedBy        string     `json:"rejected_by,omitempty"`
	RejectedAt        *time.Time `json:"rejected_at,omitempty"`
	RevokedBy         string     `json:"revoked_by,omitempty"`
	RevokedAt         *time.Time `json:"revoked_at,omitempty"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
}

type ChangeEventAuthorization struct {
	EventID         string    `json:"event_id"`
	AuthorizationID string    `json:"authorization_id"`
	LinkedBy        string    `json:"linked_by"`
	LinkedAt        time.Time `json:"linked_at"`
	Attestation     string    `json:"attestation"`
}

type ChangeAuthorizationHistory struct {
	ID              string    `json:"id"`
	AuthorizationID string    `json:"authorization_id"`
	EventID         string    `json:"event_id,omitempty"`
	Action          string    `json:"action"`
	ActorUserID     string    `json:"actor_user_id"`
	OccurredAt      time.Time `json:"occurred_at"`
}

// NormalizeChangeAuthorizationInput is shared by service validation and the
// repository's defensive validation. It never repairs duplicate categories.
func NormalizeChangeAuthorizationInput(in ChangeAuthorizationInput, now time.Time) (ChangeAuthorizationInput, error) {
	if strings.ContainsAny(in.Reference, "\r\n") {
		return in, ErrValidation
	}
	in.Reference = strings.TrimSpace(in.Reference)
	if len(in.Reference) < 3 || len(in.Reference) > 64 {
		return in, ErrValidation
	}
	for _, c := range in.Reference {
		if !(c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '.' || c == '_' || c == ':' || c == '-') {
			return in, ErrValidation
		}
	}
	in.Reference = strings.ToUpper(in.Reference)
	if in.ConfigType != "running" && in.ConfigType != "startup" {
		return in, ErrValidation
	}
	if len(in.AllowedCategories) == 0 || len(in.AllowedCategories) > 13 {
		return in, ErrValidation
	}
	in.AllowedCategories = slices.Clone(in.AllowedCategories)
	seen := make(map[string]bool, len(in.AllowedCategories))
	for i, category := range in.AllowedCategories {
		category = strings.ToLower(strings.TrimSpace(category))
		if !IsChangeAuthorizationCategory(category) || seen[category] {
			return in, ErrValidation
		}
		seen[category] = true
		in.AllowedCategories[i] = category
	}
	slices.Sort(in.AllowedCategories)
	in.WindowStart, in.WindowEnd = in.WindowStart.UTC(), in.WindowEnd.UTC()
	if now.IsZero() || in.WindowStart.IsZero() || in.WindowEnd.IsZero() || !in.WindowStart.Before(in.WindowEnd) ||
		in.WindowEnd.Sub(in.WindowStart) > MaxChangeAuthorizationWindow || !in.WindowEnd.After(now) {
		return in, ErrValidation
	}
	return in, nil
}

func IsChangeAuthorizationCategory(category string) bool {
	switch category {
	case ChangeCategorySystem, ChangeCategoryInterface, ChangeCategorySwitching, ChangeCategoryRouting,
		ChangeCategoryAccessControl, ChangeCategoryAAA, ChangeCategoryManagement, ChangeCategoryMonitoring,
		ChangeCategoryServices, ChangeCategoryCryptoVPN, ChangeCategoryQoS, ChangeCategoryRedundancy, ChangeCategoryOther:
		return true
	default:
		return false
	}
}
