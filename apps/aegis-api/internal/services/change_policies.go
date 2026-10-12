package services

import (
	"context"
	"errors"
	"time"

	"company.local/aegis/apps/aegis-api/internal/domain"
	"company.local/aegis/apps/aegis-api/internal/repositories"
	"github.com/google/uuid"
)

type ChangePolicyService struct {
	policies repositories.ChangePolicyRepository
	users    repositories.UserRepository
	devices  repositories.DeviceRepository
	now      func() time.Time
}

func NewChangePolicyService(policies repositories.ChangePolicyRepository, users repositories.UserRepository, devices repositories.DeviceRepository, clock ...func() time.Time) *ChangePolicyService {
	now := time.Now
	if len(clock) > 0 && clock[0] != nil {
		now = clock[0]
	}
	return &ChangePolicyService{policies: policies, users: users, devices: devices, now: now}
}

// Read live grants on every operation, including reads. Cached JWT permission
// and group claims are never sufficient to authorize policy access.
func (s *ChangePolicyService) checkAccess(ctx context.Context, p domain.Principal, deviceID string, permissions ...string) error {
	if p.UserID == "" {
		return domain.ErrUnauthorized
	}
	user, err := s.users.GetUser(ctx, p.UserID)
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

func (s *ChangePolicyService) Create(ctx context.Context, p domain.Principal, deviceID string, input domain.PolicySetInput) (domain.PolicySet, error) {
	if err := s.checkAccess(ctx, p, deviceID, domain.PermissionPolicyManage); err != nil {
		return domain.PolicySet{}, err
	}
	in, err := domain.NormalizePolicyInput(input)
	if err != nil {
		return domain.PolicySet{}, err
	}
	now := s.now().UTC()
	definition := domain.PolicyDefinition{EngineVersion: domain.ChangePolicyEngineVersion, DeviceID: deviceID, ConfigType: in.ConfigType, Rules: in.Rules}
	_, hash, err := domain.CanonicalPolicyDefinition(definition)
	if err != nil {
		return domain.PolicySet{}, err
	}
	set := domain.PolicySet{ID: uuid.NewString(), Reference: in.Reference, DeviceID: deviceID, ConfigType: in.ConfigType, Status: domain.PolicyDraft,
		EngineVersion: domain.ChangePolicyEngineVersion, Rules: in.Rules, DefinitionHash: hash, CreatedBy: p.UserID, CreatedAt: now, UpdatedAt: now}
	if err := s.policies.CreatePolicySet(ctx, set); err != nil {
		return domain.PolicySet{}, err
	}
	return set, nil
}

func (s *ChangePolicyService) Get(ctx context.Context, p domain.Principal, deviceID, id string) (domain.PolicySet, error) {
	if err := s.checkAccess(ctx, p, deviceID, domain.PermissionPolicyRead); err != nil {
		return domain.PolicySet{}, err
	}
	return s.policies.GetPolicySet(ctx, deviceID, id)
}
func (s *ChangePolicyService) List(ctx context.Context, p domain.Principal, deviceID string, filter repositories.PolicySetFilter, page repositories.Page) ([]domain.PolicySet, int, error) {
	if err := s.checkAccess(ctx, p, deviceID, domain.PermissionPolicyRead); err != nil {
		return nil, 0, err
	}
	return s.policies.ListPolicySets(ctx, deviceID, filter, page)
}
func (s *ChangePolicyService) Activate(ctx context.Context, p domain.Principal, deviceID, id string) (domain.PolicySet, error) {
	if err := s.checkAccess(ctx, p, deviceID, domain.PermissionPolicyManage); err != nil {
		return domain.PolicySet{}, err
	}
	return s.policies.ActivatePolicySet(ctx, deviceID, id, p.UserID, s.now().UTC())
}
func (s *ChangePolicyService) Retire(ctx context.Context, p domain.Principal, deviceID, id string) (domain.PolicySet, error) {
	if err := s.checkAccess(ctx, p, deviceID, domain.PermissionPolicyManage); err != nil {
		return domain.PolicySet{}, err
	}
	return s.policies.RetirePolicySet(ctx, deviceID, id, p.UserID, s.now().UTC())
}
func (s *ChangePolicyService) Preview(ctx context.Context, p domain.Principal, deviceID, eventID string) (domain.PolicyEvaluationPreview, error) {
	if err := s.checkAccess(ctx, p, deviceID, "backups.read", domain.PermissionPolicyEvaluate); err != nil {
		return domain.PolicyEvaluationPreview{}, err
	}
	return s.policies.PreviewPolicyEvaluation(ctx, deviceID, eventID, p.UserID)
}
func (s *ChangePolicyService) Evaluate(ctx context.Context, p domain.Principal, deviceID, eventID string) (domain.ChangePolicyEvaluation, error) {
	if err := s.checkAccess(ctx, p, deviceID, "backups.read", domain.PermissionPolicyEvaluate); err != nil {
		return domain.ChangePolicyEvaluation{}, err
	}
	return s.policies.PersistPolicyEvaluation(ctx, deviceID, eventID, p.UserID, s.now().UTC())
}
func (s *ChangePolicyService) Evidence(ctx context.Context, p domain.Principal, deviceID, eventID string) (domain.ChangePolicyEvaluation, error) {
	if err := s.checkAccess(ctx, p, deviceID, "backups.read", domain.PermissionPolicyRead); err != nil {
		return domain.ChangePolicyEvaluation{}, err
	}
	return s.policies.GetPolicyEvaluation(ctx, deviceID, eventID)
}
func (s *ChangePolicyService) Items(ctx context.Context, p domain.Principal, deviceID, eventID string, page repositories.Page) ([]domain.ChangePolicyEvaluationItem, int, error) {
	if err := s.checkAccess(ctx, p, deviceID, "backups.read", domain.PermissionPolicyRead); err != nil {
		return nil, 0, err
	}
	return s.policies.ListPolicyEvaluationItems(ctx, deviceID, eventID, page)
}
