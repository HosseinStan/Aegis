package services

import (
	"context"
	"time"

	"company.local/aegis/apps/aegis-api/internal/domain"
	"company.local/aegis/apps/aegis-api/internal/repositories"
)

// Shared admission bounds resources, not database correctness. Independent
// Stores/processes are serialized by SQLite snapshots and sealed constraints.
var changeRiskAdmission = struct{ active, admitted chan struct{} }{make(chan struct{}, 2), make(chan struct{}, 10)}

type ChangeRiskService struct {
	repo repositories.ChangeRiskRepository
	now  func() time.Time
}

func NewChangeRiskService(repo repositories.ChangeRiskRepository, clock ...func() time.Time) *ChangeRiskService {
	now := time.Now
	if len(clock) > 0 && clock[0] != nil {
		now = clock[0]
	}
	return &ChangeRiskService{repo: repo, now: now}
}
func riskWork(ctx context.Context) (context.Context, func(), error) {
	if ctx.Err() != nil {
		return nil, nil, &repositories.RiskFailure{Kind: domain.ErrTimeout, Cause: ctx.Err()}
	}
	select {
	case changeRiskAdmission.admitted <- struct{}{}:
	default:
		return nil, nil, repositories.ErrRiskUnavailable
	}
	bounded, cancel := context.WithTimeout(ctx, 10*time.Second)
	select {
	case changeRiskAdmission.active <- struct{}{}:
		return bounded, func() { <-changeRiskAdmission.active; <-changeRiskAdmission.admitted; cancel() }, nil
	case <-bounded.Done():
		<-changeRiskAdmission.admitted
		cancel()
		return nil, nil, &repositories.RiskFailure{Kind: domain.ErrTimeout, Cause: bounded.Err()}
	}
}
func riskServiceScope(p domain.Principal, device, event string) repositories.RiskScope {
	return repositories.RiskScope{ActorID: p.UserID, DeviceID: device, EventID: event}
}
func (s *ChangeRiskService) Preview(ctx context.Context, p domain.Principal, device, event string) (repositories.RiskPreview, error) {
	ctx, release, err := riskWork(ctx)
	if err != nil {
		return repositories.RiskPreview{}, err
	}
	defer release()
	return s.repo.PreviewRisk(ctx, riskServiceScope(p, device, event), s.now().UTC())
}
func (s *ChangeRiskService) Record(ctx context.Context, p domain.Principal, device, event string, expected *string) (repositories.RiskRecord, error) {
	ctx, release, err := riskWork(ctx)
	if err != nil {
		return repositories.RiskRecord{}, err
	}
	defer release()
	return s.repo.RecordRisk(ctx, riskServiceScope(p, device, event), expected, s.now().UTC())
}
func (s *ChangeRiskService) Get(ctx context.Context, p domain.Principal, device, event, id string) (domain.ChangeRiskAssessment, error) {
	ctx, release, err := riskWork(ctx)
	if err != nil {
		return domain.ChangeRiskAssessment{}, err
	}
	defer release()
	return s.repo.GetRisk(ctx, riskServiceScope(p, device, event), id)
}
func (s *ChangeRiskService) List(ctx context.Context, p domain.Principal, device, event string, page repositories.Page) ([]domain.ChangeRiskAssessment, int, error) {
	ctx, release, err := riskWork(ctx)
	if err != nil {
		return nil, 0, err
	}
	defer release()
	return s.repo.ListRisks(ctx, riskServiceScope(p, device, event), page)
}
func (s *ChangeRiskService) Findings(ctx context.Context, p domain.Principal, device, event, id string, page repositories.Page) (repositories.RiskFindingPage, error) {
	ctx, release, err := riskWork(ctx)
	if err != nil {
		return repositories.RiskFindingPage{}, err
	}
	defer release()
	return s.repo.ListRiskFindings(ctx, riskServiceScope(p, device, event), id, page)
}
