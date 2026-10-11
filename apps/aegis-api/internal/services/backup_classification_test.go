package services

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"company.local/aegis/apps/aegis-api/internal/changes"
	"company.local/aegis/apps/aegis-api/internal/domain"
	"company.local/aegis/apps/aegis-api/internal/repositories"
	"github.com/stretchr/testify/require"
)

func requireClassifiedItem(t *testing.T, item domain.ChangeEventItem) {
	t.Helper()
	result := changes.Classify(item.ContextPath, item.Command)
	require.Equal(t, domain.ChangeClassificationVersion, item.ClassificationVersion)
	require.Equal(t, result.Category, item.Category)
	require.Equal(t, result.Subcategory, item.Subcategory)
	require.Equal(t, result.RuleID, item.ClassificationRule)
	require.NotEqual(t, domain.ChangeCategoryUnclassified, item.Category)
	require.NotNil(t, item.ClassifiedAt)
	require.Equal(t, time.UTC, item.ClassifiedAt.Location())
	require.Equal(t, item.CreatedAt, *item.ClassifiedAt)
}

func TestBackupClassifiesSanitizedItemsAcrossTechnicalDomains(t *testing.T) {
	s, store, connector, principal, deviceID := backupFixture(t)
	ctx := context.Background()
	connector.SetRunning("hostname LAB\n")
	first, err := s.Create(ctx, principal, deviceID, "running")
	require.NoError(t, err)
	const secret = "SENTINEL_CLASSIFIER_SERVICE_PRIVATE_VALUE"
	connector.SetRunning("hostname LAB\ninterface Gi1/0/1\n description Finance\n switchport access vlan 20\n ip access-group USERS in\n service-policy input WAN\n no shutdown\n mls qos trust dscp\n!\nlogging host 192.0.2.10\nrouter bgp 65000\n address-family ipv4\n  neighbor 192.0.2.1 remote-as 65001\n!\nsnmp-server community " + secret + " RO\nusername admin secret " + secret + "\nunknown future syntax\nIP   ROUTE 192.0.2.0 255.255.255.0 198.51.100.1\n")
	second, err := s.Create(ctx, principal, deviceID, "running")
	require.NoError(t, err)
	require.True(t, second.Created)
	events, total, err := store.ListChangeEvents(ctx, repositories.ChangeEventFilter{DeviceID: deviceID}, repositories.Page{Limit: 10})
	require.NoError(t, err)
	require.Equal(t, 1, total)
	event := events[0]
	require.Equal(t, domain.ChangeAnalysisVersion, event.AnalysisVersion)
	require.Equal(t, domain.AuthorizationStatusUnaccounted, event.AuthorizationStatus)
	require.Equal(t, domain.PolicyStatusNotEvaluated, event.PolicyStatus)
	require.Equal(t, first.Backup.ID, event.FromBackupID)
	require.Equal(t, second.Backup.ID, event.ToBackupID)
	items, total, err := store.ListChangeEventItems(ctx, event.ID, repositories.Page{Limit: 100})
	require.NoError(t, err)
	require.Equal(t, 15, total)
	require.Equal(t, total, event.ItemCount)
	want := map[string]changes.Classification{
		"interface Gi1/0/1":                               {Category: "interface", Subcategory: "definition", RuleID: "interface.definition"},
		"description Finance":                             {Category: "interface", Subcategory: "description", RuleID: "interface.description"},
		"switchport access vlan 20":                       {Category: "switching", Subcategory: "switchport", RuleID: "switching.switchport"},
		"ip access-group USERS in":                        {Category: "access_control", Subcategory: "acl_binding", RuleID: "access_control.acl_binding"},
		"service-policy input WAN":                        {Category: "qos", Subcategory: "service_policy", RuleID: "qos.service_policy"},
		"no shutdown":                                     {Category: "interface", Subcategory: "admin_state", RuleID: "interface.admin_state"},
		"mls qos trust dscp":                              {Category: "qos", Subcategory: "generic", RuleID: "qos.generic"},
		"logging host 192.0.2.10":                         {Category: "monitoring", Subcategory: "logging", RuleID: "monitoring.logging"},
		"router bgp 65000":                                {Category: "routing", Subcategory: "bgp", RuleID: "routing.bgp"},
		"address-family ipv4":                             {Category: "routing", Subcategory: "bgp", RuleID: "routing.bgp.context"},
		"neighbor 192.0.2.1 remote-as 65001":              {Category: "routing", Subcategory: "bgp", RuleID: "routing.bgp.context"},
		"snmp-server community <redacted>":                {Category: "monitoring", Subcategory: "snmp", RuleID: "monitoring.snmp"},
		"username admin secret <redacted>":                {Category: "aaa", Subcategory: "local_user", RuleID: "aaa.local_user"},
		"unknown future syntax":                           {Category: "other", Subcategory: "unclassified", RuleID: "fallback.unclassified"},
		"IP   ROUTE 192.0.2.0 255.255.255.0 198.51.100.1": {Category: "routing", Subcategory: "static_route", RuleID: "routing.static_route"},
	}
	sensitive := 0
	for ordinal, item := range items {
		// Avoid dumping item values if redaction ever regresses.
		require.False(t, strings.Contains(item.Command, secret))
		for _, parent := range item.ContextPath {
			require.False(t, strings.Contains(parent, secret))
		}
		require.False(t, strings.Contains(item.ClassificationRule, secret))
		requireClassifiedItem(t, item)
		require.Equal(t, ordinal, item.Ordinal)
		require.Equal(t, domain.ChangeOperationAdd, item.Operation)
		require.Equal(t, event.ID, item.ChangeEventID)
		require.Equal(t, event.DetectedAt, *item.ClassifiedAt)
		expected, exists := want[item.Command]
		require.True(t, exists, "unexpected stored command")
		require.Equal(t, expected.Category, item.Category)
		require.Equal(t, expected.Subcategory, item.Subcategory)
		require.Equal(t, expected.RuleID, item.ClassificationRule)
		delete(want, item.Command)
		if item.Sensitive {
			sensitive++
		}
		if item.Command == "neighbor 192.0.2.1 remote-as 65001" {
			require.Equal(t, []string{"router bgp 65000", "address-family ipv4"}, item.ContextPath)
		}
	}
	require.Empty(t, want)
	require.Equal(t, 2, sensitive)
	diff, err := s.Diff(ctx, deviceID, first.Backup.ID, second.Backup.ID)
	require.NoError(t, err)
	require.Equal(t, event.AddedLines, diff.AddedLines)
	require.Equal(t, event.RemovedLines, diff.RemovedLines)
	files := backupFiles(t, s.root)
	duplicate, err := s.Create(ctx, principal, deviceID, "running")
	require.NoError(t, err)
	require.False(t, duplicate.Created)
	require.Equal(t, files, backupFiles(t, s.root))
	_, total, err = store.ListChangeEvents(ctx, repositories.ChangeEventFilter{DeviceID: deviceID}, repositories.Page{Limit: 10})
	require.NoError(t, err)
	require.Equal(t, 1, total)
	_, total, err = store.ListChangeEventItems(ctx, event.ID, repositories.Page{Limit: 100})
	require.NoError(t, err)
	require.Equal(t, 15, total)
}

func TestBackupUnknownCommandGetsVersionOneFallback(t *testing.T) {
	s, store, connector, principal, deviceID := backupFixture(t)
	ctx := context.Background()
	connector.SetRunning("hostname LAB\n")
	_, err := s.Create(ctx, principal, deviceID, "running")
	require.NoError(t, err)
	connector.SetRunning("hostname LAB\nunknown totally-new syntax\n")
	_, err = s.Create(ctx, principal, deviceID, "running")
	require.NoError(t, err)
	events, _, err := store.ListChangeEvents(ctx, repositories.ChangeEventFilter{}, repositories.Page{Limit: 10})
	require.NoError(t, err)
	items, total, err := store.ListChangeEventItems(ctx, events[0].ID, repositories.Page{Limit: 10})
	require.NoError(t, err)
	require.Equal(t, 1, total)
	item := items[0]
	requireClassifiedItem(t, item)
	require.Equal(t, "other", item.Category)
	require.Equal(t, "unclassified", item.Subcategory)
	require.Equal(t, "fallback.unclassified", item.ClassificationRule)
}

type invalidClassificationRepository struct {
	repositories.BackupRepository
	backup  domain.ConfigBackup
	eventID string
}

func (r *invalidClassificationRepository) CreateBackupWithChangeAnalysis(ctx context.Context, b domain.ConfigBackup, event *domain.ChangeEvent, items []domain.ChangeEventItem) error {
	r.backup, r.eventID = b, event.ID
	invalid := append([]domain.ChangeEventItem{}, items...)
	invalid[1].ClassificationRule = "spoofed.rule"
	return r.BackupRepository.CreateBackupWithChangeAnalysis(ctx, b, event, invalid)
}

func TestBackupClassificationFailurePreservesAtomicityAndFileCleanup(t *testing.T) {
	s, store, connector, principal, deviceID := backupFixture(t)
	ctx := context.Background()
	first, err := s.Create(ctx, principal, deviceID, "running")
	require.NoError(t, err)
	files := backupFiles(t, s.root)
	failing := &invalidClassificationRepository{BackupRepository: store}
	s.repository = failing
	connector.SetRunning("hostname CHANGED\n")
	_, err = s.Create(ctx, principal, deviceID, "running")
	require.ErrorIs(t, err, domain.ErrValidation)
	require.Equal(t, files, backupFiles(t, s.root))
	_, err = os.Stat(filepath.Join(s.root, failing.backup.StoragePath))
	require.ErrorIs(t, err, os.ErrNotExist)
	_, err = store.GetBackup(ctx, deviceID, failing.backup.ID)
	require.ErrorIs(t, err, domain.ErrNotFound)
	_, err = store.GetChangeEvent(ctx, failing.eventID)
	require.ErrorIs(t, err, domain.ErrNotFound)
	_, total, err := store.ListChangeEventItems(ctx, failing.eventID, repositories.Page{Limit: 100})
	require.NoError(t, err)
	require.Zero(t, total)
	_, total, err = store.ListBackups(ctx, deviceID, repositories.Page{Limit: 10})
	require.NoError(t, err)
	require.Equal(t, 1, total)
	_, total, err = store.ListChangeEvents(ctx, repositories.ChangeEventFilter{}, repositories.Page{Limit: 10})
	require.NoError(t, err)
	require.Zero(t, total)
	_, _, err = s.Content(ctx, deviceID, first.Backup.ID)
	require.NoError(t, err)
	s.repository = store
	_, err = s.Create(ctx, principal, deviceID, "running")
	require.NoError(t, err)
	require.Len(t, backupFiles(t, s.root), 2)
}
