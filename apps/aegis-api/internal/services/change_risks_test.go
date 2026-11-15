package services

import (
	"context"
	"runtime"
	"sync/atomic"
	"testing"
	"time"

	"company.local/aegis/apps/aegis-api/internal/domain"
	"company.local/aegis/apps/aegis-api/internal/repositories"
	"github.com/stretchr/testify/require"
)

type riskServiceRepository struct {
	repositories.ChangeRiskRepository
	entered chan repositories.RiskScope
	release chan struct{}
	records atomic.Int64
	denied  atomic.Bool
}

func (f *riskServiceRepository) PreviewRisk(ctx context.Context, scope repositories.RiskScope, at time.Time) (repositories.RiskPreview, error) {
	select {
	case f.entered <- scope:
	case <-ctx.Done():
		return repositories.RiskPreview{}, ctx.Err()
	}
	select {
	case <-f.release:
		return repositories.RiskPreview{ModelVersion: 1}, nil
	case <-ctx.Done():
		return repositories.RiskPreview{}, ctx.Err()
	}
}
func (f *riskServiceRepository) RecordRisk(_ context.Context, scope repositories.RiskScope, _ *string, at time.Time) (repositories.RiskRecord, error) {
	f.records.Add(1)
	if f.denied.Load() {
		return repositories.RiskRecord{}, domain.ErrForbidden
	}
	return repositories.RiskRecord{Assessment: domain.ChangeRiskAssessment{AssessedBy: scope.ActorID, DeviceID: scope.DeviceID, EventID: scope.EventID, CapturedAt: at.Format(time.RFC3339Nano)}}, nil
}

func TestRiskServiceDelegatesLiveAuthorizationAndServerClock(t *testing.T) {
	clock := time.Date(2026, 10, 19, 13, 0, 0, 123, time.UTC)
	repo := &riskServiceRepository{}
	s := NewChangeRiskService(repo, func() time.Time { return clock })
	p := domain.Principal{UserID: "authenticated-principal", Permissions: []string{"forged-cached-grant"}, DeviceGroups: []string{"*"}}
	response, err := s.Record(context.Background(), p, "scoped-device", "scoped-event", nil)
	require.NoError(t, err)
	require.Equal(t, p.UserID, response.Assessment.AssessedBy)
	require.Equal(t, clock.Format(time.RFC3339Nano), response.Assessment.CapturedAt)
	repo.denied.Store(true)
	_, err = s.Record(context.Background(), p, "scoped-device", "scoped-event", nil)
	require.ErrorIs(t, err, domain.ErrForbidden)
	require.EqualValues(t, 2, repo.records.Load(), "no service-cached idempotent response or cached JWT grant")
	require.Empty(t, changeRiskAdmission.active)
	require.Empty(t, changeRiskAdmission.admitted)
}

func TestRiskServiceProcessWideAdmissionAndCancellation(t *testing.T) {
	repo := &riskServiceRepository{entered: make(chan repositories.RiskScope, 2), release: make(chan struct{})}
	first, second := NewChangeRiskService(repo), NewChangeRiskService(repo)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	results := make(chan error, 10)
	launch := func(s *ChangeRiskService) {
		go func() { _, err := s.Preview(ctx, domain.Principal{UserID: "actor"}, "device", "event"); results <- err }()
	}
	launch(first)
	launch(second)
	for i := 0; i < 2; i++ {
		select {
		case <-repo.entered:
		case <-time.After(3 * time.Second):
			t.Fatal("active operations did not enter")
		}
	}
	for i := 0; i < 8; i++ {
		launch(first)
	}
	deadline := time.Now().Add(3 * time.Second)
	for len(changeRiskAdmission.admitted) != 10 && time.Now().Before(deadline) {
		runtime.Gosched()
	}
	require.Len(t, changeRiskAdmission.admitted, 10)
	require.Len(t, changeRiskAdmission.active, 2)
	_, err := second.Preview(context.Background(), domain.Principal{}, "device", "event")
	require.ErrorIs(t, err, repositories.ErrRiskUnavailable)
	cancel()
	for i := 0; i < 10; i++ {
		select {
		case err := <-results:
			require.Error(t, err)
		case <-time.After(3 * time.Second):
			t.Fatal("cancelled admission not released")
		}
	}
	require.Empty(t, changeRiskAdmission.admitted)
	require.Empty(t, changeRiskAdmission.active)
	bounded, release, err := riskWork(context.Background())
	require.NoError(t, err)
	defer release()
	at, ok := bounded.Deadline()
	require.True(t, ok)
	require.InDelta(t, 10, time.Until(at).Seconds(), 0.5)
}
