package services

import (
	"context"
	"errors"
	"slices"
	"time"

	"company.local/aegis/apps/aegis-api/internal/changes"
	"company.local/aegis/apps/aegis-api/internal/domain"
	"company.local/aegis/apps/aegis-api/internal/repositories"
	"github.com/google/uuid"
)

type ChangeAuthorizationService struct {
	authorizations repositories.ChangeAuthorizationRepository
	users          repositories.UserRepository
	devices        repositories.DeviceRepository
	events         repositories.ChangeEventRepository
	items          repositories.ChangeEventItemRepository
	now            func() time.Time
}

func NewChangeAuthorizationService(authorizations repositories.ChangeAuthorizationRepository, users repositories.UserRepository,
	devices repositories.DeviceRepository, events repositories.ChangeEventRepository, items repositories.ChangeEventItemRepository,
	clock ...func() time.Time) *ChangeAuthorizationService {
	now := time.Now
	if len(clock) > 0 && clock[0] != nil {
		now = clock[0]
	}
	return &ChangeAuthorizationService{authorizations: authorizations, users: users, devices: devices, events: events, items: items, now: now}
}

// UserID is supplied only by trusted authentication. Ignore cached permission
// and group claims and load the live user and grants on every operation.
func (s *ChangeAuthorizationService) checkAccess(ctx context.Context, principal domain.Principal, deviceID string, permissions ...string) error {
	if principal.UserID == "" {
		return domain.ErrUnauthorized
	}
	user, err := s.users.GetUser(ctx, principal.UserID)
	if errors.Is(err, domain.ErrNotFound) {
		return domain.ErrForbidden
	}
	if err != nil {
		return err
	}
	if !user.Enabled {
		return domain.ErrForbidden
	}
	live, err := s.users.GetPrincipal(ctx, user.ID)
	if err != nil {
		return err
	}
	for _, permission := range append([]string{"devices.read"}, permissions...) {
		if !live.HasPermission(permission) {
			return domain.ErrForbidden
		}
	}
	device, err := s.devices.GetDevice(ctx, deviceID)
	if err != nil {
		return err
	}
	if !live.CanAccessGroup(device.DeviceGroup) {
		return domain.ErrForbidden
	}
	return nil
}

func (s *ChangeAuthorizationService) Create(ctx context.Context, p domain.Principal, deviceID string, input domain.ChangeAuthorizationInput) (domain.ChangeAuthorization, error) {
	if err := s.checkAccess(ctx, p, deviceID, domain.PermissionAuthorizationRequest); err != nil {
		return domain.ChangeAuthorization{}, err
	}
	now := s.now().UTC()
	in, err := domain.NormalizeChangeAuthorizationInput(input, now)
	if err != nil {
		return domain.ChangeAuthorization{}, err
	}
	a := domain.ChangeAuthorization{ID: uuid.NewString(), Reference: in.Reference, DeviceID: deviceID, ConfigType: in.ConfigType,
		AllowedCategories: in.AllowedCategories, WindowStart: in.WindowStart, WindowEnd: in.WindowEnd, Status: domain.ChangeAuthorizationPending,
		RequestedBy: p.UserID, RequestedAt: now, CreatedAt: now, UpdatedAt: now}
	if err := s.authorizations.CreateChangeAuthorization(ctx, a); err != nil {
		return domain.ChangeAuthorization{}, err
	}
	return a, nil
}

func (s *ChangeAuthorizationService) Get(ctx context.Context, p domain.Principal, deviceID, id string) (domain.ChangeAuthorization, error) {
	if err := s.checkAccess(ctx, p, deviceID, domain.PermissionAuthorizationRead); err != nil {
		return domain.ChangeAuthorization{}, err
	}
	return s.authorizations.GetChangeAuthorization(ctx, deviceID, id)
}

func (s *ChangeAuthorizationService) List(ctx context.Context, p domain.Principal, deviceID string, filter repositories.ChangeAuthorizationFilter, page repositories.Page) ([]domain.ChangeAuthorization, int, error) {
	if err := s.checkAccess(ctx, p, deviceID, domain.PermissionAuthorizationRead); err != nil {
		return nil, 0, err
	}
	if !authorizationPageValid(page) || !authorizationStatusValid(filter.Status) || !authorizationConfigValid(filter.ConfigType) {
		return nil, 0, domain.ErrValidation
	}
	return s.authorizations.ListChangeAuthorizations(ctx, deviceID, filter, page)
}

func authorizationPageValid(p repositories.Page) bool {
	return p.Limit >= 1 && p.Limit <= 200 && p.Offset >= 0
}
func authorizationConfigValid(value string) bool {
	return value == "" || value == "running" || value == "startup"
}
func authorizationStatusValid(value string) bool {
	return value == "" || value == domain.ChangeAuthorizationPending || value == domain.ChangeAuthorizationApproved || value == domain.ChangeAuthorizationRejected || value == domain.ChangeAuthorizationRevoked
}

func (s *ChangeAuthorizationService) Approve(ctx context.Context, p domain.Principal, deviceID, id string) (domain.ChangeAuthorization, error) {
	return s.transition(ctx, p, deviceID, id, domain.ChangeAuthorizationApprovedAction)
}
func (s *ChangeAuthorizationService) Reject(ctx context.Context, p domain.Principal, deviceID, id string) (domain.ChangeAuthorization, error) {
	return s.transition(ctx, p, deviceID, id, domain.ChangeAuthorizationRejectedAction)
}
func (s *ChangeAuthorizationService) Revoke(ctx context.Context, p domain.Principal, deviceID, id string) (domain.ChangeAuthorization, error) {
	return s.transition(ctx, p, deviceID, id, domain.ChangeAuthorizationRevokedAction)
}
func (s *ChangeAuthorizationService) transition(ctx context.Context, p domain.Principal, deviceID, id, action string) (domain.ChangeAuthorization, error) {
	if err := s.checkAccess(ctx, p, deviceID, domain.PermissionAuthorizationApprove); err != nil {
		return domain.ChangeAuthorization{}, err
	}
	return s.authorizations.TransitionChangeAuthorization(ctx, deviceID, id, p.UserID, action, s.now().UTC())
}

func (s *ChangeAuthorizationService) Reconcile(ctx context.Context, p domain.Principal, deviceID, eventID, authorizationID, attestation string) (domain.ChangeEventAuthorization, error) {
	if err := s.checkAccess(ctx, p, deviceID, "backups.read", domain.PermissionAuthorizationReconcile); err != nil {
		return domain.ChangeEventAuthorization{}, err
	}
	if attestation != domain.ChangeAuthorizationAttestation || authorizationID == "" {
		return domain.ChangeEventAuthorization{}, domain.ErrValidation
	}
	link := domain.ChangeEventAuthorization{EventID: eventID, AuthorizationID: authorizationID, LinkedBy: p.UserID, LinkedAt: s.now().UTC(), Attestation: attestation}
	if err := s.authorizations.LinkChangeEventAuthorization(ctx, deviceID, link); err != nil {
		return domain.ChangeEventAuthorization{}, err
	}
	return link, nil
}

func (s *ChangeAuthorizationService) GetEvent(ctx context.Context, p domain.Principal, deviceID, eventID string) (domain.ChangeEvent, error) {
	if err := s.checkAccess(ctx, p, deviceID, "backups.read"); err != nil {
		return domain.ChangeEvent{}, err
	}
	return s.scopedEvent(ctx, deviceID, eventID)
}

func (s *ChangeAuthorizationService) scopedEvent(ctx context.Context, deviceID, eventID string) (domain.ChangeEvent, error) {
	e, err := s.events.GetChangeEvent(ctx, eventID)
	if err != nil {
		return domain.ChangeEvent{}, err
	}
	if e.DeviceID != deviceID {
		return domain.ChangeEvent{}, domain.ErrNotFound
	}
	return e, nil
}

func (s *ChangeAuthorizationService) ListEvents(ctx context.Context, p domain.Principal, deviceID string, filter repositories.ChangeEventFilter, page repositories.Page) ([]domain.ChangeEvent, int, error) {
	if err := s.checkAccess(ctx, p, deviceID, "backups.read"); err != nil {
		return nil, 0, err
	}
	if !authorizationPageValid(page) || !authorizationConfigValid(filter.ConfigType) ||
		(filter.AuthorizationStatus != "" && filter.AuthorizationStatus != domain.AuthorizationStatusUnaccounted && filter.AuthorizationStatus != domain.AuthorizationStatusAuthorized) ||
		(filter.PolicyStatus != "" && filter.PolicyStatus != domain.PolicyStatusNotEvaluated && filter.PolicyStatus != domain.PolicyStatusPass && filter.PolicyStatus != domain.PolicyStatusViolation) {
		return nil, 0, domain.ErrValidation
	}
	filter.DeviceID = deviceID
	return s.events.ListChangeEvents(ctx, filter, page)
}

func (s *ChangeAuthorizationService) ListItems(ctx context.Context, p domain.Principal, deviceID, eventID string, page repositories.Page) ([]domain.ChangeEventItem, int, error) {
	if err := s.checkAccess(ctx, p, deviceID, "backups.read"); err != nil {
		return nil, 0, err
	}
	if !authorizationPageValid(page) {
		return nil, 0, domain.ErrValidation
	}
	if _, err := s.scopedEvent(ctx, deviceID, eventID); err != nil {
		return nil, 0, err
	}
	items, total, err := s.items.ListChangeEventItems(ctx, eventID, page)
	if err != nil {
		return nil, 0, err
	}
	// Fail closed if database corruption or a non-service writer introduced
	// recognized secret-bearing material. Never return a raw fallback.
	for _, item := range items {
		command, path, sensitive := changes.Sanitize(item.Command, item.ContextPath)
		if command != item.Command || !slices.Equal(path, item.ContextPath) || (sensitive && !item.Sensitive) {
			return nil, 0, domain.ErrValidation
		}
	}
	return items, total, nil
}

func (s *ChangeAuthorizationService) Evidence(ctx context.Context, p domain.Principal, deviceID, eventID string) (domain.ChangeEventAuthorization, error) {
	if err := s.checkAccess(ctx, p, deviceID, domain.PermissionAuthorizationRead); err != nil {
		return domain.ChangeEventAuthorization{}, err
	}
	return s.authorizations.GetChangeEventAuthorization(ctx, deviceID, eventID)
}

func (s *ChangeAuthorizationService) History(ctx context.Context, p domain.Principal, deviceID, id string, page repositories.Page) ([]domain.ChangeAuthorizationHistory, int, error) {
	if err := s.checkAccess(ctx, p, deviceID, domain.PermissionAuthorizationRead); err != nil {
		return nil, 0, err
	}
	if !authorizationPageValid(page) {
		return nil, 0, domain.ErrValidation
	}
	return s.authorizations.ListChangeAuthorizationHistory(ctx, deviceID, id, page)
}
