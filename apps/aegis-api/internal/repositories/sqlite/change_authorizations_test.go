package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"company.local/aegis/apps/aegis-api/internal/changes"
	"company.local/aegis/apps/aegis-api/internal/domain"
	"company.local/aegis/apps/aegis-api/internal/repositories"
	"company.local/aegis/apps/aegis-api/migrations"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

type authorizationFixture struct {
	s                                        *Store
	device, otherDevice, requester, approver string
	now                                      time.Time
}

func authorizationStore(t *testing.T) authorizationFixture {
	t.Helper()
	s, devices := changeEventStore(t)
	now := time.Date(2026, 10, 4, 11, 0, 0, 0, time.UTC)
	f := authorizationFixture{s: s, device: devices[0].ID, otherDevice: devices[1].ID, now: now}
	for _, target := range []*string{&f.requester, &f.approver} {
		*target = uuid.NewString()
		require.NoError(t, s.CreateUser(context.Background(), domain.User{ID: *target, Username: *target, Enabled: true, CreatedAt: now, UpdatedAt: now}))
		require.NoError(t, s.AssignRole(context.Background(), *target, "system_administrator"))
	}
	return f
}

func (f authorizationFixture) request(t *testing.T) domain.ChangeAuthorization {
	t.Helper()
	a := domain.ChangeAuthorization{ID: uuid.NewString(), Reference: strings.ToUpper("CHG-" + uuid.NewString()), DeviceID: f.device, ConfigType: "running", AllowedCategories: []string{"aaa", "interface"}, WindowStart: f.now, WindowEnd: f.now.Add(4 * time.Hour), Status: domain.ChangeAuthorizationPending, RequestedBy: f.requester, RequestedAt: f.now, CreatedAt: f.now, UpdatedAt: f.now}
	require.NoError(t, f.s.CreateChangeAuthorization(context.Background(), a))
	return a
}

func authorizationEvent(t *testing.T, s *Store, deviceID string) domain.ChangeEvent {
	t.Helper()
	ctx := context.Background()
	now := time.Date(2026, 10, 4, 13, 0, 0, 0, time.UTC)
	from := eventBackup(deviceID, "running", now.Add(-time.Hour))
	to := eventBackup(deviceID, "running", now)
	require.NoError(t, s.CreateBackup(ctx, from))
	e := eventBetween(from, to)
	e.AnalysisVersion = 1
	e.AnalyzedAt = &now
	e.ItemCount = 2
	items := []domain.ChangeEventItem{
		{ID: uuid.NewString(), ChangeEventID: e.ID, Ordinal: 0, Operation: "remove", ContextPath: []string{"interface Gi1"}, Command: "description old", CreatedAt: now},
		{ID: uuid.NewString(), ChangeEventID: e.ID, Ordinal: 1, Operation: "add", ContextPath: []string{}, Command: "enable secret <redacted>", Sensitive: true, CreatedAt: now},
	}
	for i := range items {
		c := changes.Classify(items[i].ContextPath, items[i].Command)
		items[i].ClassificationVersion = 1
		items[i].Category = c.Category
		items[i].Subcategory = c.Subcategory
		items[i].ClassificationRule = c.RuleID
		items[i].ClassifiedAt = &now
	}
	require.NoError(t, s.CreateBackupWithChangeAnalysis(ctx, to, &e, items))
	return e
}

func (f authorizationFixture) approve(t *testing.T, a domain.ChangeAuthorization) domain.ChangeAuthorization {
	t.Helper()
	got, err := f.s.TransitionChangeAuthorization(context.Background(), f.device, a.ID, f.approver, "approved", f.now.Add(time.Hour))
	require.NoError(t, err)
	return got
}
func (f authorizationFixture) link(a domain.ChangeAuthorization, e domain.ChangeEvent) domain.ChangeEventAuthorization {
	return domain.ChangeEventAuthorization{EventID: e.ID, AuthorizationID: a.ID, LinkedBy: f.approver, LinkedAt: f.now.Add(3 * time.Hour), Attestation: domain.ChangeAuthorizationAttestation}
}

func TestAuthorizationLifecycleAndHistoricalRevocation(t *testing.T) {
	f := authorizationStore(t)
	ctx := context.Background()
	a := f.request(t)
	_, err := f.s.TransitionChangeAuthorization(ctx, f.device, a.ID, f.requester, "approved", f.now.Add(time.Hour))
	require.ErrorIs(t, err, domain.ErrForbidden)
	_, err = f.s.TransitionChangeAuthorization(ctx, f.device, a.ID, f.approver, "revoked", f.now.Add(time.Hour))
	require.ErrorIs(t, err, domain.ErrConflict)
	a = f.approve(t, a)
	require.Equal(t, f.approver, a.ApprovedBy)
	require.Equal(t, f.now.Add(time.Hour), *a.ApprovedAt)
	for _, action := range []string{"approved", "rejected"} {
		_, err = f.s.TransitionChangeAuthorization(ctx, f.device, a.ID, f.approver, action, f.now.Add(2*time.Hour))
		require.ErrorIs(t, err, domain.ErrConflict)
	}
	e := authorizationEvent(t, f.s, f.device)
	link := f.link(a, e)
	require.NoError(t, f.s.LinkChangeEventAuthorization(ctx, f.device, link))
	loaded, err := f.s.GetChangeEvent(ctx, e.ID)
	require.NoError(t, err)
	require.Equal(t, "authorized", loaded.AuthorizationStatus)
	require.Equal(t, e.PolicyStatus, loaded.PolicyStatus)
	require.Equal(t, link.LinkedAt, loaded.UpdatedAt)
	require.ErrorIs(t, f.s.LinkChangeEventAuthorization(ctx, f.device, link), domain.ErrConflict)
	_, err = f.s.TransitionChangeAuthorization(ctx, f.device, a.ID, f.approver, "revoked", f.now.Add(3*time.Hour))
	require.NoError(t, err)
	_, err = f.s.TransitionChangeAuthorization(ctx, f.device, a.ID, f.approver, "approved", f.now.Add(3*time.Hour))
	require.ErrorIs(t, err, domain.ErrConflict)
	loaded, err = f.s.GetChangeEvent(ctx, e.ID)
	require.NoError(t, err)
	require.Equal(t, "authorized", loaded.AuthorizationStatus)
	got, err := f.s.GetChangeEventAuthorization(ctx, f.device, e.ID)
	require.NoError(t, err)
	require.Equal(t, link, got)
	history, total, err := f.s.ListChangeAuthorizationHistory(ctx, f.device, a.ID, repositories.Page{Limit: 200})
	require.NoError(t, err)
	require.Equal(t, 4, total)
	actions := []string{}
	for _, h := range history {
		actions = append(actions, h.Action)
		require.Equal(t, a.ID, h.AuthorizationID)
	}
	require.Equal(t, []string{"requested", "approved", "linked", "revoked"}, actions)
	page, total, err := f.s.ListChangeAuthorizationHistory(ctx, f.device, a.ID, repositories.Page{Limit: 1, Offset: 50})
	require.NoError(t, err)
	require.Empty(t, page)
	require.Equal(t, 4, total)
	other := f.request(t)
	_, err = f.s.TransitionChangeAuthorization(ctx, f.device, other.ID, f.approver, "rejected", f.now.Add(time.Hour))
	require.NoError(t, err)
	_, err = f.s.TransitionChangeAuthorization(ctx, f.device, other.ID, f.approver, "approved", f.now.Add(2*time.Hour))
	require.ErrorIs(t, err, domain.ErrConflict)
	expired := f.request(t)
	_, err = f.s.TransitionChangeAuthorization(ctx, f.device, expired.ID, f.approver, "approved", expired.WindowEnd)
	require.ErrorIs(t, err, domain.ErrConflict)
}

func TestAuthorizationReconciliationEligibility(t *testing.T) {
	for _, test := range []struct {
		name string
		edit func(*testing.T, authorizationFixture, domain.ChangeAuthorization, domain.ChangeEvent, *domain.ChangeEventAuthorization)
		want error
	}{
		{"pending", func(t *testing.T, f authorizationFixture, a domain.ChangeAuthorization, e domain.ChangeEvent, l *domain.ChangeEventAuthorization) {
			_, err := f.s.db.Exec(`UPDATE change_authorizations SET status='pending', approved_by=NULL, approved_at=NULL WHERE id=?`, a.ID)
			require.NoError(t, err)
		}, domain.ErrConflict},
		{"rejected", func(t *testing.T, f authorizationFixture, a domain.ChangeAuthorization, e domain.ChangeEvent, l *domain.ChangeEventAuthorization) {
			_, err := f.s.db.Exec(`UPDATE change_authorizations SET status='rejected', approved_by=NULL, approved_at=NULL, rejected_by=?, rejected_at=? WHERE id=?`, f.approver, timestamp(f.now), a.ID)
			require.NoError(t, err)
		}, domain.ErrConflict},
		{"revoked", func(t *testing.T, f authorizationFixture, a domain.ChangeAuthorization, e domain.ChangeEvent, l *domain.ChangeEventAuthorization) {
			_, err := f.s.TransitionChangeAuthorization(context.Background(), f.device, a.ID, f.approver, "revoked", f.now.Add(2*time.Hour))
			require.NoError(t, err)
		}, domain.ErrConflict},
		{"missing request", func(t *testing.T, f authorizationFixture, a domain.ChangeAuthorization, e domain.ChangeEvent, l *domain.ChangeEventAuthorization) {
			l.AuthorizationID = uuid.NewString()
		}, domain.ErrNotFound},
		{"missing event", func(t *testing.T, f authorizationFixture, a domain.ChangeAuthorization, e domain.ChangeEvent, l *domain.ChangeEventAuthorization) {
			l.EventID = uuid.NewString()
		}, domain.ErrNotFound},
		{"cross device", func(t *testing.T, f authorizationFixture, a domain.ChangeAuthorization, e domain.ChangeEvent, l *domain.ChangeEventAuthorization) {
			other := authorizationEvent(t, f.s, f.otherDevice)
			l.EventID = other.ID
		}, domain.ErrNotFound},
		{"different config", func(t *testing.T, f authorizationFixture, a domain.ChangeAuthorization, e domain.ChangeEvent, l *domain.ChangeEventAuthorization) {
			_, err := f.s.db.Exec(`UPDATE change_authorizations SET config_type='startup' WHERE id=?`, a.ID)
			require.NoError(t, err)
		}, domain.ErrValidation},
		{"before window", func(t *testing.T, f authorizationFixture, a domain.ChangeAuthorization, e domain.ChangeEvent, l *domain.ChangeEventAuthorization) {
			_, err := f.s.db.Exec(`UPDATE change_events SET detected_at=? WHERE id=?`, timestamp(a.WindowStart.Add(-time.Nanosecond)), e.ID)
			require.NoError(t, err)
		}, domain.ErrValidation},
		{"exclusive end", func(t *testing.T, f authorizationFixture, a domain.ChangeAuthorization, e domain.ChangeEvent, l *domain.ChangeEventAuthorization) {
			_, err := f.s.db.Exec(`UPDATE change_events SET detected_at=? WHERE id=?`, timestamp(a.WindowEnd), e.ID)
			require.NoError(t, err)
		}, domain.ErrValidation},
		{"late approval", func(t *testing.T, f authorizationFixture, a domain.ChangeAuthorization, e domain.ChangeEvent, l *domain.ChangeEventAuthorization) {
			_, err := f.s.db.Exec(`UPDATE change_authorizations SET approved_at=? WHERE id=?`, timestamp(e.DetectedAt.Add(time.Nanosecond)), a.ID)
			require.NoError(t, err)
		}, domain.ErrValidation},
		{"mixed category scope", func(t *testing.T, f authorizationFixture, a domain.ChangeAuthorization, e domain.ChangeEvent, l *domain.ChangeEventAuthorization) {
			_, err := f.s.db.Exec(`UPDATE change_authorizations SET allowed_categories_json='["interface"]' WHERE id=?`, a.ID)
			require.NoError(t, err)
		}, domain.ErrValidation},
		{"wrong classification", func(t *testing.T, f authorizationFixture, a domain.ChangeAuthorization, e domain.ChangeEvent, l *domain.ChangeEventAuthorization) {
			_, err := f.s.db.Exec(`UPDATE change_event_items SET classification_rule='spoofed' WHERE change_event_id=?`, e.ID)
			require.NoError(t, err)
		}, domain.ErrValidation},
		{"legacy classification", func(t *testing.T, f authorizationFixture, a domain.ChangeAuthorization, e domain.ChangeEvent, l *domain.ChangeEventAuthorization) {
			_, err := f.s.db.Exec(`UPDATE change_event_items SET classification_version=0,category='unclassified',subcategory='unclassified',classification_rule='',classified_at=NULL WHERE change_event_id=?`, e.ID)
			require.NoError(t, err)
		}, domain.ErrValidation},
		{"legacy analysis", func(t *testing.T, f authorizationFixture, a domain.ChangeAuthorization, e domain.ChangeEvent, l *domain.ChangeEventAuthorization) {
			_, err := f.s.db.Exec(`UPDATE change_events SET analysis_version=0, analyzed_at=NULL, item_count=0 WHERE id=?`, e.ID)
			require.NoError(t, err)
		}, domain.ErrValidation},
		{"zero items", func(t *testing.T, f authorizationFixture, a domain.ChangeAuthorization, e domain.ChangeEvent, l *domain.ChangeEventAuthorization) {
			_, err := f.s.db.Exec(`DELETE FROM change_event_items WHERE change_event_id=?`, e.ID)
			require.NoError(t, err)
			_, err = f.s.db.Exec(`UPDATE change_events SET item_count=0 WHERE id=?`, e.ID)
			require.NoError(t, err)
		}, domain.ErrValidation},
		{"count mismatch", func(t *testing.T, f authorizationFixture, a domain.ChangeAuthorization, e domain.ChangeEvent, l *domain.ChangeEventAuthorization) {
			_, err := f.s.db.Exec(`UPDATE change_events SET item_count=3 WHERE id=?`, e.ID)
			require.NoError(t, err)
		}, domain.ErrValidation},
		{"missing attestation", func(t *testing.T, f authorizationFixture, a domain.ChangeAuthorization, e domain.ChangeEvent, l *domain.ChangeEventAuthorization) {
			l.Attestation = ""
		}, domain.ErrValidation},
		{"invalid attestation", func(t *testing.T, f authorizationFixture, a domain.ChangeAuthorization, e domain.ChangeEvent, l *domain.ChangeEventAuthorization) {
			l.Attestation = "automatic"
		}, domain.ErrValidation},
		{"requester reconciles", func(t *testing.T, f authorizationFixture, a domain.ChangeAuthorization, e domain.ChangeEvent, l *domain.ChangeEventAuthorization) {
			l.LinkedBy = f.requester
		}, domain.ErrForbidden},
		{"unknown actor", func(t *testing.T, f authorizationFixture, a domain.ChangeAuthorization, e domain.ChangeEvent, l *domain.ChangeEventAuthorization) {
			l.LinkedBy = uuid.NewString()
		}, domain.ErrForbidden},
		{"disabled actor", func(t *testing.T, f authorizationFixture, a domain.ChangeAuthorization, e domain.ChangeEvent, l *domain.ChangeEventAuthorization) {
			_, err := f.s.db.Exec(`UPDATE users SET enabled=0 WHERE id=?`, f.approver)
			require.NoError(t, err)
		}, domain.ErrForbidden},
		{"raw secret", func(t *testing.T, f authorizationFixture, a domain.ChangeAuthorization, e domain.ChangeEvent, l *domain.ChangeEventAuthorization) {
			_, err := f.s.db.Exec(`UPDATE change_event_items SET command='enable secret should-not-escape' WHERE change_event_id=? AND ordinal=1`, e.ID)
			require.NoError(t, err)
		}, domain.ErrValidation},
		{"invalid chain", func(t *testing.T, f authorizationFixture, a domain.ChangeAuthorization, e domain.ChangeEvent, l *domain.ChangeEventAuthorization) {
			_, err := f.s.db.Exec(`UPDATE config_backups SET created_at=? WHERE id=?`, timestamp(e.DetectedAt.Add(time.Hour)), e.FromBackupID)
			require.NoError(t, err)
		}, domain.ErrValidation},
		{"missing approval metadata", func(t *testing.T, f authorizationFixture, a domain.ChangeAuthorization, e domain.ChangeEvent, l *domain.ChangeEventAuthorization) {
			_, err := f.s.db.Exec(`PRAGMA ignore_check_constraints=ON`)
			require.NoError(t, err)
			_, err = f.s.db.Exec(`UPDATE change_authorizations SET approved_at=NULL WHERE id=?`, a.ID)
			require.NoError(t, err)
			_, err = f.s.db.Exec(`PRAGMA ignore_check_constraints=OFF`)
			require.NoError(t, err)
		}, domain.ErrConflict},
	} {
		t.Run(test.name, func(t *testing.T) {
			f := authorizationStore(t)
			a := f.approve(t, f.request(t))
			e := authorizationEvent(t, f.s, f.device)
			link := f.link(a, e)
			test.edit(t, f, a, e, &link)
			require.ErrorIs(t, f.s.LinkChangeEventAuthorization(context.Background(), f.device, link), test.want)
			loaded, err := f.s.GetChangeEvent(context.Background(), e.ID)
			require.NoError(t, err)
			require.Equal(t, "unaccounted", loaded.AuthorizationStatus)
			require.Equal(t, "not_evaluated", loaded.PolicyStatus)
			var links, history int
			require.NoError(t, f.s.db.QueryRow(`SELECT COUNT(*) FROM change_event_authorizations`).Scan(&links))
			require.Zero(t, links)
			require.NoError(t, f.s.db.QueryRow(`SELECT COUNT(*) FROM change_authorization_history WHERE action='linked'`).Scan(&history))
			require.Zero(t, history)
		})
	}
}

func TestAuthorizationSingleUseAndInclusiveStart(t *testing.T) {
	f := authorizationStore(t)
	ctx := context.Background()
	a := f.approve(t, f.request(t))
	e := authorizationEvent(t, f.s, f.device)
	_, err := f.s.db.Exec(`UPDATE change_authorizations SET window_start=?,approved_at=? WHERE id=?`, authorizationTimestamp(e.DetectedAt), authorizationTimestamp(e.DetectedAt), a.ID)
	require.NoError(t, err)
	require.NoError(t, f.s.LinkChangeEventAuthorization(ctx, f.device, f.link(a, e)))
	second := authorizationEvent(t, f.s, f.device)
	require.ErrorIs(t, f.s.LinkChangeEventAuthorization(ctx, f.device, f.link(a, second)), domain.ErrConflict)
	other := f.approve(t, f.request(t))
	require.ErrorIs(t, f.s.LinkChangeEventAuthorization(ctx, f.device, f.link(other, e)), domain.ErrConflict)
	_, err = f.s.GetChangeAuthorization(ctx, f.otherDevice, a.ID)
	require.ErrorIs(t, err, domain.ErrNotFound)
	_, err = f.s.GetChangeEventAuthorization(ctx, f.otherDevice, e.ID)
	require.ErrorIs(t, err, domain.ErrNotFound)
	_, _, err = f.s.ListChangeAuthorizationHistory(ctx, f.otherDevice, a.ID, repositories.Page{Limit: 10})
	require.ErrorIs(t, err, domain.ErrNotFound)
	page, total, err := f.s.ListChangeAuthorizations(ctx, f.device, repositories.ChangeAuthorizationFilter{Status: "approved", ConfigType: "running"}, repositories.Page{Limit: 1, Offset: 100})
	require.NoError(t, err)
	require.Empty(t, page)
	require.Equal(t, 2, total)
}

func TestAuthorizationOtherMustBeExplicit(t *testing.T) {
	for _, include := range []bool{false, true} {
		t.Run(map[bool]string{false: "other excluded", true: "other explicit"}[include], func(t *testing.T) {
			f := authorizationStore(t)
			a := f.approve(t, f.request(t))
			e := authorizationEvent(t, f.s, f.device)
			c := changes.Classify(nil, "unknown-syntax")
			require.Equal(t, "other", c.Category)
			_, err := f.s.db.Exec(`UPDATE change_event_items SET command='unknown-syntax',context_path_json='[]',category=?,subcategory=?,classification_rule=? WHERE change_event_id=?`, c.Category, c.Subcategory, c.RuleID, e.ID)
			require.NoError(t, err)
			if include {
				_, err = f.s.db.Exec(`UPDATE change_authorizations SET allowed_categories_json='["other"]' WHERE id=?`, a.ID)
				require.NoError(t, err)
			}
			err = f.s.LinkChangeEventAuthorization(context.Background(), f.device, f.link(a, e))
			if include {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, domain.ErrValidation)
			}
		})
	}
}

func TestAuthorizationTransactionsRollbackOnInjectedFailures(t *testing.T) {
	for _, action := range []string{"requested", "approved", "rejected", "revoked", "linked", "event-update", "link-insert"} {
		t.Run(action, func(t *testing.T) {
			f := authorizationStore(t)
			ctx := context.Background()
			var a domain.ChangeAuthorization
			var e domain.ChangeEvent
			if action != "requested" {
				a = f.request(t)
			}
			if action == "revoked" || action == "linked" || action == "event-update" || action == "link-insert" {
				a = f.approve(t, a)
			}
			if action == "linked" || action == "event-update" || action == "link-insert" {
				e = authorizationEvent(t, f.s, f.device)
			}
			var before int
			require.NoError(t, f.s.db.QueryRow(`SELECT COUNT(*) FROM change_authorization_history`).Scan(&before))
			query := `CREATE TRIGGER inject_failure BEFORE INSERT ON change_authorization_history WHEN NEW.action = '` + action + `' BEGIN SELECT RAISE(ABORT,'injected'); END`
			if action == "event-update" {
				query = `CREATE TRIGGER inject_failure BEFORE UPDATE ON change_events BEGIN SELECT RAISE(ABORT,'injected'); END`
			}
			if action == "link-insert" {
				query = `CREATE TRIGGER inject_failure BEFORE INSERT ON change_event_authorizations BEGIN SELECT RAISE(ABORT,'injected'); END`
			}
			_, err := f.s.db.Exec(query)
			require.NoError(t, err)
			switch action {
			case "requested":
				a = domain.ChangeAuthorization{ID: uuid.NewString(), Reference: "CHG-ROLLBACK", DeviceID: f.device, ConfigType: "running", AllowedCategories: []string{"system"}, WindowStart: f.now, WindowEnd: f.now.Add(time.Hour), Status: "pending", RequestedBy: f.requester, RequestedAt: f.now, CreatedAt: f.now, UpdatedAt: f.now}
				err = f.s.CreateChangeAuthorization(ctx, a)
			case "linked", "event-update", "link-insert":
				err = f.s.LinkChangeEventAuthorization(ctx, f.device, f.link(a, e))
			default:
				_, err = f.s.TransitionChangeAuthorization(ctx, f.device, a.ID, f.approver, action, f.now.Add(2*time.Hour))
			}
			require.Error(t, err)
			var after int
			require.NoError(t, f.s.db.QueryRow(`SELECT COUNT(*) FROM change_authorization_history`).Scan(&after))
			require.Equal(t, before, after)
			if action == "requested" {
				_, err = f.s.GetChangeAuthorization(ctx, f.device, a.ID)
				require.ErrorIs(t, err, domain.ErrNotFound)
			} else {
				got, err := f.s.GetChangeAuthorization(ctx, f.device, a.ID)
				require.NoError(t, err)
				require.Equal(t, a, got)
			}
			if e.ID != "" {
				got, err := f.s.GetChangeEvent(ctx, e.ID)
				require.NoError(t, err)
				require.Equal(t, e, got)
				_, err = f.s.GetChangeEventAuthorization(ctx, f.device, e.ID)
				require.ErrorIs(t, err, domain.ErrNotFound)
			}
		})
	}
}

func TestAuthorizationConcurrentDecisionsAndReconciliation(t *testing.T) {
	for _, operation := range []string{"approve-reject", "link", "revoke-link"} {
		t.Run(operation, func(t *testing.T) {
			f := authorizationStore(t)
			a := f.request(t)
			e := authorizationEvent(t, f.s, f.device)
			if operation != "approve-reject" {
				a = f.approve(t, a)
			}
			start := make(chan struct{})
			results := make(chan error, 12)
			var wg sync.WaitGroup
			for i := 0; i < 12; i++ {
				wg.Add(1)
				go func(index int) {
					defer wg.Done()
					<-start
					if operation == "link" || operation == "revoke-link" && index%2 == 0 {
						results <- f.s.LinkChangeEventAuthorization(context.Background(), f.device, f.link(a, e))
						return
					}
					action := "approved"
					if index%2 == 0 {
						action = "rejected"
					}
					if operation == "revoke-link" {
						action = "revoked"
					}
					_, err := f.s.TransitionChangeAuthorization(context.Background(), f.device, a.ID, f.approver, action, f.now.Add(3*time.Hour))
					results <- err
				}(i)
			}
			close(start)
			wg.Wait()
			close(results)
			success := 0
			for err := range results {
				if err == nil {
					success++
				} else {
					require.True(t, errors.Is(err, domain.ErrConflict), err)
				}
			}
			if operation != "revoke-link" {
				require.Equal(t, 1, success)
			} else {
				require.True(t, success == 1 || success == 2)
			}
			var links, linkedHistory int
			require.NoError(t, f.s.db.QueryRow(`SELECT COUNT(*) FROM change_event_authorizations`).Scan(&links))
			require.NoError(t, f.s.db.QueryRow(`SELECT COUNT(*) FROM change_authorization_history WHERE action='linked'`).Scan(&linkedHistory))
			require.Equal(t, links, linkedHistory)
			require.LessOrEqual(t, links, 1)
			got, err := f.s.GetChangeEvent(context.Background(), e.ID)
			require.NoError(t, err)
			require.Equal(t, links == 1, got.AuthorizationStatus == "authorized")
			history, total, err := f.s.ListChangeAuthorizationHistory(context.Background(), f.device, a.ID, repositories.Page{Limit: 200})
			require.NoError(t, err)
			require.Len(t, history, total)
			if operation == "approve-reject" {
				require.Equal(t, 2, total)
			}
		})
	}
}

func TestAuthorizationMigrationUpgradeAndConstraints(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "task-three.db")
	db, err := sql.Open("sqlite", path)
	require.NoError(t, err)
	db.SetMaxOpenConns(1)
	s := &Store{db: db}
	for _, name := range []string{"001_initial.sql", "002_seed_rbac.sql", "003_change_events.sql", "004_change_event_items.sql", "005_change_item_classification.sql"} {
		body, err := migrations.Files.ReadFile(name)
		require.NoError(t, err)
		_, err = db.Exec(string(body))
		require.NoError(t, err)
		_, err = db.Exec(`INSERT INTO schema_migrations VALUES (?,?)`, name, timestamp(time.Now()))
		require.NoError(t, err)
	}
	devices := changeEventDevices(t, s)
	from := eventBackup(devices[0].ID, "startup", time.Now().UTC())
	to := eventBackup(devices[0].ID, "startup", from.CreatedAt.Add(time.Hour))
	legacy := eventBetween(from, to)
	require.NoError(t, s.CreateBackup(ctx, from))
	require.NoError(t, s.CreateBackupWithChangeEvent(ctx, to, &legacy))
	next, oldEvent, oldItems := analysisFixture(t, s, devices[1].ID)
	require.NoError(t, s.CreateBackupWithChangeAnalysis(ctx, next, &oldEvent, oldItems))
	classified := authorizationEvent(t, s, devices[0].ID)
	var oldRoot int
	require.NoError(t, db.QueryRow(`SELECT rootpage FROM sqlite_master WHERE name='change_events'`).Scan(&oldRoot))
	require.NoError(t, s.Close())
	s, err = Open(ctx, path)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, s.Close()) })
	require.NoError(t, s.Migrate(ctx))
	for _, e := range []domain.ChangeEvent{legacy, oldEvent, classified} {
		got, err := s.GetChangeEvent(ctx, e.ID)
		require.NoError(t, err)
		require.Equal(t, e, got)
		require.Equal(t, "unaccounted", got.AuthorizationStatus)
	}
	items, _, err := s.ListChangeEventItems(ctx, oldEvent.ID, repositories.Page{Limit: 10})
	require.NoError(t, err)
	require.Equal(t, oldItems, items)
	var root, evidence int
	require.NoError(t, s.db.QueryRow(`SELECT rootpage FROM sqlite_master WHERE name='change_events'`).Scan(&root))
	require.Equal(t, oldRoot, root)
	assertApprovedTaskSixMigrations(t, s.db)
	require.NoError(t, s.db.QueryRow(`SELECT COUNT(*) FROM change_event_authorizations`).Scan(&evidence))
	require.Zero(t, evidence)
	rows, err := s.db.Query(`PRAGMA foreign_key_check`)
	require.NoError(t, err)
	require.False(t, rows.Next())
	require.NoError(t, rows.Close())
	f := authorizationStore(t)
	a := f.request(t)
	duplicate := a
	duplicate.ID = uuid.NewString()
	require.ErrorIs(t, f.s.CreateChangeAuthorization(ctx, duplicate), domain.ErrConflict)
	_, err = f.s.db.Exec(`UPDATE change_authorizations SET reference=lower(reference) WHERE id=?`, a.ID)
	require.Error(t, err)
	for _, query := range []string{`UPDATE change_authorizations SET status='expired' WHERE id=?`, `UPDATE change_authorizations SET config_type='candidate' WHERE id=?`, `UPDATE change_authorizations SET window_end=window_start WHERE id=?`, `UPDATE change_authorizations SET allowed_categories_json='[]' WHERE id=?`, `UPDATE change_authorizations SET device_id='missing' WHERE id=?`, `UPDATE change_authorizations SET requested_by='missing' WHERE id=?`, `DELETE FROM users WHERE id=?`} {
		id := a.ID
		if query == `DELETE FROM users WHERE id=?` {
			id = f.requester
		}
		_, err = f.s.db.Exec(query, id)
		require.Error(t, err)
	}
	_, err = f.s.db.Exec(`UPDATE change_authorization_history SET action='invalid' WHERE authorization_id=?`, a.ID)
	require.Error(t, err)
	_, err = f.s.db.Exec(`DELETE FROM change_authorizations WHERE id=?`, a.ID)
	require.Error(t, err)
	a = f.approve(t, a)
	e := authorizationEvent(t, f.s, f.device)
	link := f.link(a, e)
	require.NoError(t, f.s.LinkChangeEventAuthorization(ctx, f.device, link))
	_, err = f.s.db.Exec(`INSERT INTO change_event_authorizations VALUES (?,?,?,?,?)`, e.ID, a.ID, f.approver, timestamp(link.LinkedAt), link.Attestation)
	require.Error(t, err)
	_, err = f.s.db.Exec(`DELETE FROM change_events WHERE id=?`, e.ID)
	require.Error(t, err)
}

func TestAuthorizationRepositoryRechecksPermissionsAndGroups(t *testing.T) {
	f := authorizationStore(t)
	ctx := context.Background()
	a := f.request(t)
	operator := uuid.NewString()
	require.NoError(t, f.s.CreateUser(ctx, domain.User{ID: operator, Username: operator, Enabled: true, CreatedAt: f.now, UpdatedAt: f.now}))
	require.NoError(t, f.s.AssignRole(ctx, operator, "network_operator"))
	_, err := f.s.TransitionChangeAuthorization(ctx, f.device, a.ID, operator, "approved", f.now.Add(time.Hour))
	require.ErrorIs(t, err, domain.ErrForbidden)
	_, err = f.s.db.Exec(`UPDATE devices SET device_group='group-A' WHERE id=?`, f.device)
	require.NoError(t, err)
	_, err = f.s.db.Exec(`INSERT INTO user_device_groups VALUES (?,?)`, operator, "group-B")
	require.NoError(t, err)
	other := a
	other.ID = uuid.NewString()
	other.Reference = "CHG-GROUP-REPO"
	other.RequestedBy = operator
	require.ErrorIs(t, f.s.CreateChangeAuthorization(ctx, other), domain.ErrForbidden)
	_, err = f.s.db.Exec(`UPDATE users SET enabled=0 WHERE id=?`, f.requester)
	require.NoError(t, err)
	other.RequestedBy = f.requester
	require.ErrorIs(t, f.s.CreateChangeAuthorization(ctx, other), domain.ErrForbidden)
	other.RequestedBy = uuid.NewString()
	require.ErrorIs(t, f.s.CreateChangeAuthorization(ctx, other), domain.ErrForbidden)
	other.RequestedBy = f.approver
	other.DeviceID = uuid.NewString()
	require.ErrorIs(t, f.s.CreateChangeAuthorization(ctx, other), domain.ErrNotFound)
	_, err = f.s.TransitionChangeAuthorization(ctx, f.device, uuid.NewString(), f.approver, "approved", f.now.Add(time.Hour))
	require.ErrorIs(t, err, domain.ErrNotFound)
	_, err = f.s.db.Exec(`UPDATE users SET enabled=0 WHERE id=?`, f.approver)
	require.NoError(t, err)
	_, err = f.s.TransitionChangeAuthorization(ctx, f.device, a.ID, f.approver, "approved", f.now.Add(time.Hour))
	require.ErrorIs(t, err, domain.ErrForbidden)
}

func TestAuthorizationSeedGrants(t *testing.T) {
	f := authorizationStore(t)
	ctx := context.Background()
	for _, test := range []struct {
		role     string
		expected []string
	}{
		{"viewer", nil},
		{"network_operator", []string{domain.PermissionAuthorizationRead, domain.PermissionAuthorizationRequest}},
		{"network_administrator", []string{domain.PermissionAuthorizationRead, domain.PermissionAuthorizationReconcile, domain.PermissionAuthorizationRequest}},
		{"system_administrator", []string{domain.PermissionAuthorizationApprove, domain.PermissionAuthorizationRead, domain.PermissionAuthorizationReconcile, domain.PermissionAuthorizationRequest}},
	} {
		t.Run(test.role, func(t *testing.T) {
			id := uuid.NewString()
			require.NoError(t, f.s.CreateUser(ctx, domain.User{ID: id, Username: id, Enabled: true, CreatedAt: f.now, UpdatedAt: f.now}))
			require.NoError(t, f.s.AssignRole(ctx, id, test.role))
			p, err := f.s.GetPrincipal(ctx, id)
			require.NoError(t, err)
			var grants []string
			for _, permission := range p.Permissions {
				if strings.HasPrefix(permission, "changes.authorization.") {
					grants = append(grants, permission)
				}
			}
			require.Equal(t, test.expected, grants)
		})
	}
}

func TestAuthorizationConcurrentSingleUseAcrossIndependentStores(t *testing.T) {
	for _, mode := range []string{"same ticket", "same event"} {
		t.Run(mode, func(t *testing.T) {
			f := authorizationStore(t)
			ctx := context.Background()
			a := f.approve(t, f.request(t))
			b := f.approve(t, f.request(t))
			e := authorizationEvent(t, f.s, f.device)
			other := authorizationEvent(t, f.s, f.device)
			var path string
			require.NoError(t, f.s.db.QueryRow(`SELECT file FROM pragma_database_list WHERE name='main'`).Scan(&path))
			second, err := Open(ctx, path)
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, second.Close()) })
			links := []domain.ChangeEventAuthorization{f.link(a, e), f.link(b, e)}
			if mode == "same ticket" {
				links[1] = f.link(a, other)
			}
			start := make(chan struct{})
			results := make(chan error, 2)
			var wg sync.WaitGroup
			for index, store := range []*Store{f.s, second} {
				wg.Add(1)
				go func(i int, s *Store) {
					defer wg.Done()
					<-start
					results <- s.LinkChangeEventAuthorization(ctx, f.device, links[i])
				}(index, store)
			}
			close(start)
			wg.Wait()
			close(results)
			success := 0
			for err := range results {
				if err == nil {
					success++
				} else {
					require.ErrorIs(t, err, domain.ErrConflict)
				}
			}
			require.Equal(t, 1, success)
			var count, history int
			require.NoError(t, f.s.db.QueryRow(`SELECT COUNT(*) FROM change_event_authorizations`).Scan(&count))
			require.NoError(t, f.s.db.QueryRow(`SELECT COUNT(*) FROM change_authorization_history WHERE action='linked'`).Scan(&history))
			require.Equal(t, 1, count)
			require.Equal(t, count, history)
		})
	}
}

func TestAuthorizationInvalidClassificationAndApprovalEvidence(t *testing.T) {
	for _, test := range []struct{ name, query string }{
		{"missing classification time", `UPDATE change_event_items SET classified_at=NULL WHERE change_event_id=?`},
		{"unknown classification version", `UPDATE change_event_items SET classification_version=2 WHERE change_event_id=?`},
		{"forged subcategory wildcard", `UPDATE change_event_items SET subcategory='unclassified' WHERE change_event_id=?`},
		{"forged category", `UPDATE change_event_items SET category='system' WHERE change_event_id=?`},
		{"raw context secret", `UPDATE change_event_items SET context_path_json='["radius server LAB", "key 0 forbidden-material"]' WHERE change_event_id=?`},
	} {
		t.Run(test.name, func(t *testing.T) {
			f := authorizationStore(t)
			a := f.approve(t, f.request(t))
			e := authorizationEvent(t, f.s, f.device)
			_, err := f.s.db.Exec(test.query, e.ID)
			require.NoError(t, err)
			require.ErrorIs(t, f.s.LinkChangeEventAuthorization(context.Background(), f.device, f.link(a, e)), domain.ErrValidation)
		})
	}
}

func TestAuthorizationNeverChangesPolicyStatus(t *testing.T) {
	for _, policy := range []string{domain.PolicyStatusNotEvaluated, domain.PolicyStatusPass, domain.PolicyStatusViolation} {
		t.Run(policy, func(t *testing.T) {
			f := authorizationStore(t)
			a := f.approve(t, f.request(t))
			e := authorizationEvent(t, f.s, f.device)
			_, err := f.s.db.Exec(`UPDATE change_events SET policy_status=? WHERE id=?`, policy, e.ID)
			require.NoError(t, err)
			require.NoError(t, f.s.LinkChangeEventAuthorization(context.Background(), f.device, f.link(a, e)))
			got, err := f.s.GetChangeEvent(context.Background(), e.ID)
			require.NoError(t, err)
			require.Equal(t, policy, got.PolicyStatus)
		})
	}
}

func TestAuthorizationWindowOrderingPreservesNanosecondPrecision(t *testing.T) {
	f := authorizationStore(t)
	a := f.request(t)
	a.ID = uuid.NewString()
	a.Reference = "CHG-NANOSECOND-WINDOW"
	a.WindowStart = f.now.Add(time.Hour)
	a.WindowEnd = a.WindowStart.Add(time.Nanosecond)
	require.NoError(t, f.s.CreateChangeAuthorization(context.Background(), a))
	got, err := f.s.GetChangeAuthorization(context.Background(), f.device, a.ID)
	require.NoError(t, err)
	require.Equal(t, time.Nanosecond, got.WindowEnd.Sub(got.WindowStart))
}

func TestAuthorizationTimestampBoundaries(t *testing.T) {
	// Literal expectations exercise persisted representations that sort
	// differently from their chronology. Do not derive eligibility from the
	// production comparator or formatter, and keep all other evidence valid.
	for _, test := range []struct {
		name, start, end, approved, detected string
		allowed                              bool
	}{
		{"inclusive start", "2026-10-04T12:00:00Z", "2026-10-04T14:00:00Z", "2026-10-04T11:30:00Z", "2026-10-04T12:00:00Z", true},
		{"exclusive end", "2026-10-04T12:00:00Z", "2026-10-04T14:00:00Z", "2026-10-04T11:30:00Z", "2026-10-04T14:00:00Z", false},
		{"nanosecond before start", "2026-10-04T12:00:00Z", "2026-10-04T14:00:00Z", "2026-10-04T11:30:00Z", "2026-10-04T11:59:59.999999999Z", false},
		{"nanosecond before end", "2026-10-04T12:00:00Z", "2026-10-04T14:00:00Z", "2026-10-04T11:30:00Z", "2026-10-04T13:59:59.999999999Z", true},
		{"same second after start", "2026-10-04T12:00:00Z", "2026-10-04T14:00:00Z", "2026-10-04T12:00:00Z", "2026-10-04T12:00:00.500000000Z", true},
		{"same second before fractional start", "2026-10-04T12:00:00.5Z", "2026-10-04T14:00:00Z", "2026-10-04T11:30:00Z", "2026-10-04T12:00:00Z", false},
		{"same second before fractional end", "2026-10-04T12:00:00Z", "2026-10-04T14:00:00.5Z", "2026-10-04T11:30:00Z", "2026-10-04T14:00:00Z", true},
		{"equal approval different precision", "2026-10-04T12:00:00Z", "2026-10-04T14:00:00Z", "2026-10-04T13:00:00.000000000Z", "2026-10-04T13:00:00Z", true},
		{"equal approval fractional precision", "2026-10-04T12:00:00Z", "2026-10-04T14:00:00Z", "2026-10-04T13:00:00.5Z", "2026-10-04T13:00:00.500000000Z", true},
		{"approval nanosecond after detection", "2026-10-04T12:00:00Z", "2026-10-04T14:00:00Z", "2026-10-04T13:00:00.000000001Z", "2026-10-04T13:00:00Z", false},
		{"equal instants different offsets", "2026-10-04T15:30:00+03:30", "2026-10-04T17:30:00+03:30", "2026-10-04T16:30:00.500+03:30", "2026-10-04T09:00:00.500000000-04:00", true},
		{"exclusive end different offsets", "2026-10-04T15:30:00+03:30", "2026-10-04T17:30:00+03:30", "2026-10-04T16:30:00+03:30", "2026-10-04T10:00:00-04:00", false},
		{"midnight crossing", "2026-10-04T23:59:59.9Z", "2026-10-05T00:00:00.1Z", "2026-10-04T23:59:59.900Z", "2026-10-05T00:00:00Z", true},
		{"short fractions within window", "2026-10-04T13:00:00.1Z", "2026-10-04T13:00:00.123456790Z", "2026-10-04T13:00:00.123Z", "2026-10-04T13:00:00.123456789Z", true},
		{"short fractions outside window", "2026-10-04T13:00:00.1Z", "2026-10-04T13:00:00.123456790Z", "2026-10-04T13:00:00.123Z", "2026-10-04T13:00:00.123456790Z", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			f := authorizationStore(t)
			ctx := context.Background()
			parse := func(value string) time.Time {
				parsed, err := time.Parse(time.RFC3339Nano, value)
				require.NoError(t, err)
				return parsed
			}
			a := f.request(t)
			// The new request follows the actual create/approve methods. Only
			// approval serialization is varied afterward, not its instant.
			a.ID, a.Reference = uuid.NewString(), "CHG-TIME-BOUNDARY"
			a.WindowStart, a.WindowEnd = parse(test.start), parse(test.end)
			require.NoError(t, f.s.CreateChangeAuthorization(ctx, a))
			_, err := f.s.TransitionChangeAuthorization(ctx, f.device, a.ID, f.approver, "approved", parse(test.approved))
			require.NoError(t, err)
			_, err = f.s.db.Exec(`UPDATE change_authorizations SET approved_at=? WHERE id=?`, test.approved, a.ID)
			require.NoError(t, err)
			e := authorizationEvent(t, f.s, f.device)
			detected := parse(test.detected)
			_, err = f.s.db.Exec(`UPDATE config_backups SET created_at=? WHERE id=?`, detected.Add(-time.Second).UTC().Format(time.RFC3339Nano), e.FromBackupID)
			require.NoError(t, err)
			_, err = f.s.db.Exec(`UPDATE config_backups SET created_at=? WHERE id=?`, test.detected, e.ToBackupID)
			require.NoError(t, err)
			_, err = f.s.db.Exec(`UPDATE change_events SET detected_at=?,created_at=?,updated_at=?,analyzed_at=? WHERE id=?`, test.detected, test.detected, test.detected, test.detected, e.ID)
			require.NoError(t, err)
			_, err = f.s.db.Exec(`UPDATE change_event_items SET created_at=?,classified_at=? WHERE change_event_id=?`, test.detected, test.detected, e.ID)
			require.NoError(t, err)
			before, err := f.s.GetChangeEvent(ctx, e.ID)
			require.NoError(t, err)
			link := f.link(a, e)
			link.LinkedAt = a.WindowEnd.Add(time.Hour)
			err = f.s.LinkChangeEventAuthorization(ctx, f.device, link)
			if test.allowed {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, domain.ErrValidation)
			}
			got, err := f.s.GetChangeEvent(ctx, e.ID)
			require.NoError(t, err)
			var links, histories int
			require.NoError(t, f.s.db.QueryRow(`SELECT COUNT(*) FROM change_event_authorizations WHERE event_id=?`, e.ID).Scan(&links))
			require.NoError(t, f.s.db.QueryRow(`SELECT COUNT(*) FROM change_authorization_history WHERE authorization_id=? AND action='linked'`, a.ID).Scan(&histories))
			require.Equal(t, links, histories)
			if test.allowed {
				require.Equal(t, 1, links)
				require.Equal(t, domain.AuthorizationStatusAuthorized, got.AuthorizationStatus)
				require.True(t, got.UpdatedAt.Equal(link.LinkedAt))
			} else {
				require.Zero(t, links)
				require.Equal(t, before, got, "rejected boundary must not change any event state")
			}
			require.Equal(t, before.PolicyStatus, got.PolicyStatus)
		})
	}
}

func TestAuthorizationApprovalExpirationBoundaries(t *testing.T) {
	for _, test := range []struct {
		name, end, now string
		allowed        bool
	}{
		{"nanosecond before expiry", "2026-10-04T12:00:00Z", "2026-10-04T11:59:59.999999999Z", true},
		{"exact expiry", "2026-10-04T12:00:00Z", "2026-10-04T12:00:00Z", false},
		{"same second before fractional expiry", "2026-10-04T12:00:00.5Z", "2026-10-04T12:00:00Z", true},
		{"nanosecond after expiry", "2026-10-04T12:00:00Z", "2026-10-04T12:00:00.000000001Z", false},
		{"equal expiry different offset", "2026-10-04T12:00:00.5Z", "2026-10-04T15:30:00.500000000+03:30", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			f := authorizationStore(t)
			a := f.request(t)
			end, err := time.Parse(time.RFC3339Nano, test.end)
			require.NoError(t, err)
			_, err = f.s.db.Exec(`UPDATE change_authorizations SET window_end=? WHERE id=?`, end.UTC().Format("2006-01-02T15:04:05.000000000Z"), a.ID)
			require.NoError(t, err)
			now, err := time.Parse(time.RFC3339Nano, test.now)
			require.NoError(t, err)
			_, err = f.s.TransitionChangeAuthorization(context.Background(), f.device, a.ID, f.approver, "approved", now)
			if test.allowed {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, domain.ErrConflict)
			}
			got, err := f.s.GetChangeAuthorization(context.Background(), f.device, a.ID)
			require.NoError(t, err)
			var approvals int
			require.NoError(t, f.s.db.QueryRow(`SELECT COUNT(*) FROM change_authorization_history WHERE authorization_id=? AND action='approved'`, a.ID).Scan(&approvals))
			if test.allowed {
				require.Equal(t, 1, approvals)
				require.Equal(t, domain.ChangeAuthorizationApproved, got.Status)
			} else {
				require.Zero(t, approvals)
				require.Equal(t, domain.ChangeAuthorizationPending, got.Status)
				require.Nil(t, got.ApprovedAt)
			}
		})
	}
}
