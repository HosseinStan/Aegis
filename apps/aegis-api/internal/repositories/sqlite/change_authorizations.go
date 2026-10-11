package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"time"

	"company.local/aegis/apps/aegis-api/internal/changes"
	"company.local/aegis/apps/aegis-api/internal/domain"
	"company.local/aegis/apps/aegis-api/internal/repositories"
	"github.com/google/uuid"
	sqlitedriver "modernc.org/sqlite"
)

var _ repositories.ChangeAuthorizationRepository = (*Store)(nil)

const authorizationColumns = `id, reference, device_id, config_type, allowed_categories_json,
    window_start, window_end, status, requested_by, requested_at, approved_by, approved_at,
    rejected_by, rejected_at, revoked_by, revoked_at, created_at, updated_at`

func authorizationTimestamp(t time.Time) string {
	return t.UTC().Format("2006-01-02T15:04:05.000000000Z")
}

func authorizationWriteError(err error) error {
	// A second SQLite writer can lose its WAL snapshot upgrade. No action has
	// committed in this transaction; report a retryable conflict, not a SQL leak.
	var sqliteError *sqlitedriver.Error
	if errors.As(err, &sqliteError) {
		// Primary SQLite result codes: BUSY=5, LOCKED=6. Mask extended
		// codes too, including BUSY_SNAPSHOT=517, which has no name in its message.
		if primary := sqliteError.Code() & 0xff; primary == 5 || primary == 6 {
			return domain.ErrConflict
		}
	}
	if err != nil && strings.Contains(strings.ToLower(err.Error()), "check constraint") {
		return domain.ErrValidation
	}
	return repositoryError(mapWriteError(err))
}

func (s *Store) authorizationTransaction(ctx context.Context, operation func(*sql.Tx) error) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return repositoryError(err)
	}
	defer tx.Rollback()
	if err := operation(tx); err != nil {
		return authorizationWriteError(err)
	}
	return authorizationWriteError(tx.Commit())
}

// Recheck the active actor, live permissions and current device group within
// the write transaction, rather than trusting an earlier middleware snapshot.
func authorizationActor(ctx context.Context, tx *sql.Tx, deviceID, actorID string, permissions ...string) error {
	var enabled bool
	if err := tx.QueryRowContext(ctx, `SELECT enabled FROM users WHERE id = ?`, actorID).Scan(&enabled); err != nil {
		if err == sql.ErrNoRows {
			return domain.ErrForbidden
		}
		return repositoryError(err)
	}
	if !enabled {
		return domain.ErrForbidden
	}
	var group sql.NullString
	if err := tx.QueryRowContext(ctx, `SELECT device_group FROM devices WHERE id = ?`, deviceID).Scan(&group); err != nil {
		return repositoryError(err)
	}
	p := domain.Principal{UserID: actorID}
	rows, err := tx.QueryContext(ctx, `SELECT DISTINCT p.name FROM permissions p
        JOIN role_permissions rp ON rp.permission_id = p.id
        JOIN user_roles ur ON ur.role_id = rp.role_id WHERE ur.user_id = ?`, actorID)
	if err != nil {
		return repositoryError(err)
	}
	for rows.Next() {
		var permission string
		if err := rows.Scan(&permission); err != nil {
			rows.Close()
			return err
		}
		p.Permissions = append(p.Permissions, permission)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return repositoryError(err)
	}
	for _, permission := range append([]string{"devices.read"}, permissions...) {
		if !p.HasPermission(permission) {
			return domain.ErrForbidden
		}
	}
	rows, err = tx.QueryContext(ctx, `SELECT device_group FROM user_device_groups WHERE user_id = ?`, actorID)
	if err != nil {
		return repositoryError(err)
	}
	for rows.Next() {
		var value string
		if err := rows.Scan(&value); err != nil {
			rows.Close()
			return err
		}
		p.DeviceGroups = append(p.DeviceGroups, value)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return repositoryError(err)
	}
	if !p.CanAccessGroup(group.String) {
		return domain.ErrForbidden
	}
	return nil
}

func insertAuthorizationHistory(ctx context.Context, tx *sql.Tx, authorizationID, eventID, action, actor string, at time.Time) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO change_authorization_history
        (id, authorization_id, event_id, action, actor_user_id, occurred_at) VALUES (?, ?, ?, ?, ?, ?)`,
		uuid.NewString(), authorizationID, nullable(eventID), action, actor, authorizationTimestamp(at))
	return authorizationWriteError(err)
}

func (s *Store) CreateChangeAuthorization(ctx context.Context, a domain.ChangeAuthorization) error {
	in, err := domain.NormalizeChangeAuthorizationInput(domain.ChangeAuthorizationInput{
		Reference: a.Reference, ConfigType: a.ConfigType, AllowedCategories: a.AllowedCategories,
		WindowStart: a.WindowStart, WindowEnd: a.WindowEnd}, a.RequestedAt)
	if err != nil || a.ID == "" || a.Status != domain.ChangeAuthorizationPending || a.Reference != in.Reference ||
		!slices.Equal(a.AllowedCategories, in.AllowedCategories) || !a.CreatedAt.Equal(a.RequestedAt) || !a.UpdatedAt.Equal(a.RequestedAt) ||
		a.ApprovedBy != "" || a.ApprovedAt != nil || a.RejectedBy != "" || a.RejectedAt != nil || a.RevokedBy != "" || a.RevokedAt != nil {
		return domain.ErrValidation
	}
	return s.authorizationTransaction(ctx, func(tx *sql.Tx) error {
		if err := authorizationActor(ctx, tx, a.DeviceID, a.RequestedBy, domain.PermissionAuthorizationRequest); err != nil {
			return err
		}
		categories, err := json.Marshal(a.AllowedCategories)
		if err != nil {
			return domain.ErrValidation
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO change_authorizations (`+authorizationColumns+`)
            VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, NULL, NULL, NULL, NULL, NULL, NULL, ?, ?)`,
			a.ID, a.Reference, a.DeviceID, a.ConfigType, string(categories), authorizationTimestamp(a.WindowStart), authorizationTimestamp(a.WindowEnd),
			a.Status, a.RequestedBy, authorizationTimestamp(a.RequestedAt), authorizationTimestamp(a.CreatedAt), authorizationTimestamp(a.UpdatedAt))
		if err != nil {
			return authorizationWriteError(err)
		}
		return insertAuthorizationHistory(ctx, tx, a.ID, "", domain.ChangeAuthorizationRequestedAction, a.RequestedBy, a.RequestedAt)
	})
}

func scanChangeAuthorization(scan scanFunc) (domain.ChangeAuthorization, error) {
	var a domain.ChangeAuthorization
	var categories, start, end, requested, created, updated string
	var approvedBy, approvedAt, rejectedBy, rejectedAt, revokedBy, revokedAt sql.NullString
	err := scan(&a.ID, &a.Reference, &a.DeviceID, &a.ConfigType, &categories, &start, &end, &a.Status, &a.RequestedBy, &requested,
		&approvedBy, &approvedAt, &rejectedBy, &rejectedAt, &revokedBy, &revokedAt, &created, &updated)
	if err != nil {
		return a, repositoryError(err)
	}
	if err = json.Unmarshal([]byte(categories), &a.AllowedCategories); err != nil {
		return a, domain.ErrValidation
	}
	for _, field := range []struct {
		value  string
		target *time.Time
	}{{start, &a.WindowStart}, {end, &a.WindowEnd}, {requested, &a.RequestedAt}, {created, &a.CreatedAt}, {updated, &a.UpdatedAt}} {
		if *field.target, err = parseTime(field.value); err != nil {
			return a, err
		}
	}
	a.ApprovedBy, a.RejectedBy, a.RevokedBy = approvedBy.String, rejectedBy.String, revokedBy.String
	if a.ApprovedAt, err = parseNullableTime(approvedAt); err != nil {
		return a, err
	}
	if a.RejectedAt, err = parseNullableTime(rejectedAt); err != nil {
		return a, err
	}
	a.RevokedAt, err = parseNullableTime(revokedAt)
	return a, err
}

func (s *Store) GetChangeAuthorization(ctx context.Context, deviceID, id string) (domain.ChangeAuthorization, error) {
	return scanChangeAuthorization(s.db.QueryRowContext(ctx, `SELECT `+authorizationColumns+` FROM change_authorizations WHERE device_id = ? AND id = ?`, deviceID, id).Scan)
}

func validAuthorizationPage(page repositories.Page) bool {
	return page.Limit >= 1 && page.Limit <= 200 && page.Offset >= 0
}

func (s *Store) ListChangeAuthorizations(ctx context.Context, deviceID string, filter repositories.ChangeAuthorizationFilter, page repositories.Page) ([]domain.ChangeAuthorization, int, error) {
	if !validAuthorizationPage(page) {
		return nil, 0, domain.ErrValidation
	}
	where := ` WHERE device_id = ?`
	args := []any{deviceID}
	if filter.Status != "" {
		where += ` AND status = ?`
		args = append(args, filter.Status)
	}
	if filter.ConfigType != "" {
		where += ` AND config_type = ?`
		args = append(args, filter.ConfigType)
	}
	var total int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM change_authorizations`+where, args...).Scan(&total); err != nil {
		return nil, 0, repositoryError(err)
	}
	rows, err := s.db.QueryContext(ctx, `SELECT `+authorizationColumns+` FROM change_authorizations`+where+` ORDER BY requested_at DESC, id DESC LIMIT ? OFFSET ?`, append(args, page.Limit, page.Offset)...)
	if err != nil {
		return nil, 0, repositoryError(err)
	}
	defer rows.Close()
	items := make([]domain.ChangeAuthorization, 0)
	for rows.Next() {
		a, err := scanChangeAuthorization(rows.Scan)
		if err != nil {
			return nil, 0, err
		}
		items = append(items, a)
	}
	return items, total, repositoryError(rows.Err())
}

func (s *Store) TransitionChangeAuthorization(ctx context.Context, deviceID, id, actorID, action string, now time.Time) (domain.ChangeAuthorization, error) {
	var result domain.ChangeAuthorization
	err := s.authorizationTransaction(ctx, func(tx *sql.Tx) error {
		if err := authorizationActor(ctx, tx, deviceID, actorID, domain.PermissionAuthorizationApprove); err != nil {
			return err
		}
		a, err := scanChangeAuthorization(tx.QueryRowContext(ctx, `SELECT `+authorizationColumns+` FROM change_authorizations WHERE device_id = ? AND id = ?`, deviceID, id).Scan)
		if err != nil {
			return err
		}
		if now.IsZero() || now.Before(a.UpdatedAt) {
			return domain.ErrValidation
		}
		var query, previous, next string
		switch action {
		case domain.ChangeAuthorizationApprovedAction:
			if a.RequestedBy == actorID {
				return domain.ErrForbidden
			}
			if !a.WindowEnd.After(now) {
				return domain.ErrConflict
			}
			previous, next = domain.ChangeAuthorizationPending, domain.ChangeAuthorizationApproved
			query = `UPDATE change_authorizations SET status = ?, approved_by = ?, approved_at = ?, updated_at = ? WHERE device_id = ? AND id = ? AND status = ?`
			a.ApprovedBy, a.ApprovedAt = actorID, timePointer(now)
		case domain.ChangeAuthorizationRejectedAction:
			previous, next = domain.ChangeAuthorizationPending, domain.ChangeAuthorizationRejected
			query = `UPDATE change_authorizations SET status = ?, rejected_by = ?, rejected_at = ?, updated_at = ? WHERE device_id = ? AND id = ? AND status = ?`
			a.RejectedBy, a.RejectedAt = actorID, timePointer(now)
		case domain.ChangeAuthorizationRevokedAction:
			previous, next = domain.ChangeAuthorizationApproved, domain.ChangeAuthorizationRevoked
			query = `UPDATE change_authorizations SET status = ?, revoked_by = ?, revoked_at = ?, updated_at = ? WHERE device_id = ? AND id = ? AND status = ?`
			a.RevokedBy, a.RevokedAt = actorID, timePointer(now)
		default:
			return domain.ErrValidation
		}
		if a.Status != previous {
			return domain.ErrConflict
		}
		written, err := tx.ExecContext(ctx, query, next, actorID, authorizationTimestamp(now), authorizationTimestamp(now), deviceID, id, previous)
		if err != nil {
			return authorizationWriteError(err)
		}
		count, err := written.RowsAffected()
		if err != nil {
			return err
		}
		if count != 1 {
			return domain.ErrConflict
		}
		if err := insertAuthorizationHistory(ctx, tx, id, "", action, actorID, now); err != nil {
			return err
		}
		a.Status, a.UpdatedAt = next, now.UTC()
		result = a
		return nil
	})
	if err != nil {
		return domain.ChangeAuthorization{}, err
	}
	return result, nil
}

func timePointer(t time.Time) *time.Time { t = t.UTC(); return &t }

func validateAuthorizationEvent(ctx context.Context, tx *sql.Tx, a domain.ChangeAuthorization, e domain.ChangeEvent, link domain.ChangeEventAuthorization) error {
	canonical, err := domain.NormalizeChangeAuthorizationInput(domain.ChangeAuthorizationInput{
		Reference: a.Reference, ConfigType: a.ConfigType, AllowedCategories: a.AllowedCategories,
		WindowStart: a.WindowStart, WindowEnd: a.WindowEnd}, a.RequestedAt)
	if err != nil || canonical.Reference != a.Reference || !slices.Equal(canonical.AllowedCategories, a.AllowedCategories) {
		return domain.ErrValidation
	}
	if a.Status != domain.ChangeAuthorizationApproved || a.ApprovedBy == "" || a.ApprovedAt == nil || a.ApprovedAt.IsZero() || a.ApprovedBy == a.RequestedBy {
		return domain.ErrConflict
	}
	if a.ApprovedAt.Before(a.RequestedAt) || link.LinkedAt.Before(a.UpdatedAt) || link.LinkedAt.Before(e.UpdatedAt) {
		return domain.ErrValidation
	}
	if link.LinkedBy == a.RequestedBy {
		return domain.ErrForbidden
	}
	if e.AuthorizationStatus != domain.AuthorizationStatusUnaccounted {
		return domain.ErrConflict
	}
	if e.DeviceID != a.DeviceID || e.ConfigType != a.ConfigType || e.FromBackupID == e.ToBackupID ||
		e.AnalysisVersion != domain.ChangeAnalysisVersion || e.AnalyzedAt == nil || e.ItemCount <= 0 {
		return domain.ErrValidation
	}
	if e.DetectedAt.Before(a.WindowStart) || !e.DetectedAt.Before(a.WindowEnd) || a.ApprovedAt.After(e.DetectedAt) || link.LinkedAt.Before(e.DetectedAt) {
		return domain.ErrValidation
	}
	var previous, next string
	for _, backup := range []struct {
		id     string
		target *string
	}{{e.FromBackupID, &previous}, {e.ToBackupID, &next}} {
		if err := tx.QueryRowContext(ctx, `SELECT created_at FROM config_backups WHERE id = ? AND device_id = ? AND config_type = ?`, backup.id, e.DeviceID, e.ConfigType).Scan(backup.target); err != nil {
			if err == sql.ErrNoRows {
				return domain.ErrValidation
			}
			return repositoryError(err)
		}
	}
	from, err := parseTime(previous)
	if err != nil {
		return domain.ErrValidation
	}
	to, err := parseTime(next)
	if err != nil || from.After(to) || e.DetectedAt.Before(to) {
		return domain.ErrValidation
	}
	rows, err := tx.QueryContext(ctx, `SELECT `+changeEventItemColumns+` FROM change_event_items WHERE change_event_id = ? ORDER BY ordinal ASC`, e.ID)
	if err != nil {
		return repositoryError(err)
	}
	defer rows.Close()
	count := 0
	for rows.Next() {
		item, err := scanAuthorizationItem(rows.Scan)
		if err != nil {
			return err
		}
		if item.Ordinal != count || item.ChangeEventID != e.ID || item.ClassificationVersion != domain.ChangeClassificationVersion ||
			item.ClassifiedAt == nil || item.ClassifiedAt.IsZero() || strings.TrimSpace(item.Command) == "" ||
			(item.Operation != domain.ChangeOperationAdd && item.Operation != domain.ChangeOperationRemove) ||
			!domain.IsChangeAuthorizationCategory(item.Category) || !slices.Contains(a.AllowedCategories, item.Category) {
			return domain.ErrValidation
		}
		command, path, sensitive := changes.Sanitize(item.Command, item.ContextPath)
		if command != item.Command || !slices.Equal(path, item.ContextPath) || (sensitive && !item.Sensitive) {
			return domain.ErrValidation
		}
		if err := validateItemClassification(item); err != nil {
			return err
		}
		count++
	}
	if err := rows.Err(); err != nil {
		return repositoryError(err)
	}
	if count != e.ItemCount {
		return domain.ErrValidation
	}
	return nil
}

func scanAuthorizationItem(scan scanFunc) (domain.ChangeEventItem, error) {
	var i domain.ChangeEventItem
	var path, created string
	var classified sql.NullString
	err := scan(&i.ID, &i.ChangeEventID, &i.Ordinal, &i.Operation, &path, &i.Command, &i.Sensitive, &created, &i.ClassificationVersion, &i.Category, &i.Subcategory, &i.ClassificationRule, &classified)
	if err != nil {
		return i, repositoryError(err)
	}
	if err = json.Unmarshal([]byte(path), &i.ContextPath); err != nil {
		return i, domain.ErrValidation
	}
	if i.CreatedAt, err = parseTime(created); err != nil {
		return i, err
	}
	i.ClassifiedAt, err = parseNullableTime(classified)
	return i, err
}

func (s *Store) LinkChangeEventAuthorization(ctx context.Context, deviceID string, link domain.ChangeEventAuthorization) error {
	if link.Attestation != domain.ChangeAuthorizationAttestation || link.LinkedAt.IsZero() {
		return domain.ErrValidation
	}
	return s.authorizationTransaction(ctx, func(tx *sql.Tx) error {
		if err := authorizationActor(ctx, tx, deviceID, link.LinkedBy, "backups.read", domain.PermissionAuthorizationReconcile); err != nil {
			return err
		}
		a, err := scanChangeAuthorization(tx.QueryRowContext(ctx, `SELECT `+authorizationColumns+` FROM change_authorizations WHERE device_id = ? AND id = ?`, deviceID, link.AuthorizationID).Scan)
		if err != nil {
			return err
		}
		e, err := scanChangeEvent(tx.QueryRowContext(ctx, `SELECT `+changeEventColumns+` FROM change_events WHERE device_id = ? AND id = ?`, deviceID, link.EventID).Scan)
		if err != nil {
			return err
		}
		var existing int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM change_event_authorizations WHERE event_id = ? OR authorization_id = ?`, e.ID, a.ID).Scan(&existing); err != nil {
			return repositoryError(err)
		}
		if existing != 0 {
			return domain.ErrConflict
		}
		if err := validateAuthorizationEvent(ctx, tx, a, e, link); err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO change_event_authorizations (event_id, authorization_id, linked_by, linked_at, attestation) VALUES (?, ?, ?, ?, ?)`,
			link.EventID, link.AuthorizationID, link.LinkedBy, authorizationTimestamp(link.LinkedAt), link.Attestation)
		if err != nil {
			return authorizationWriteError(err)
		}
		written, err := tx.ExecContext(ctx, `UPDATE change_events SET authorization_status = ?, updated_at = ? WHERE id = ? AND device_id = ? AND authorization_status = ?`,
			domain.AuthorizationStatusAuthorized, authorizationTimestamp(link.LinkedAt), e.ID, deviceID, domain.AuthorizationStatusUnaccounted)
		if err != nil {
			return authorizationWriteError(err)
		}
		count, err := written.RowsAffected()
		if err != nil {
			return err
		}
		if count != 1 {
			return domain.ErrConflict
		}
		return insertAuthorizationHistory(ctx, tx, a.ID, e.ID, domain.ChangeAuthorizationLinkedAction, link.LinkedBy, link.LinkedAt)
	})
}

func (s *Store) GetChangeEventAuthorization(ctx context.Context, deviceID, eventID string) (domain.ChangeEventAuthorization, error) {
	var link domain.ChangeEventAuthorization
	var at string
	err := s.db.QueryRowContext(ctx, `SELECT l.event_id, l.authorization_id, l.linked_by, l.linked_at, l.attestation
        FROM change_event_authorizations l JOIN change_events e ON e.id = l.event_id WHERE e.device_id = ? AND l.event_id = ?`, deviceID, eventID).
		Scan(&link.EventID, &link.AuthorizationID, &link.LinkedBy, &at, &link.Attestation)
	if err != nil {
		return link, repositoryError(err)
	}
	link.LinkedAt, err = parseTime(at)
	return link, err
}

func (s *Store) ListChangeAuthorizationHistory(ctx context.Context, deviceID, id string, page repositories.Page) ([]domain.ChangeAuthorizationHistory, int, error) {
	if !validAuthorizationPage(page) {
		return nil, 0, domain.ErrValidation
	}
	if _, err := s.GetChangeAuthorization(ctx, deviceID, id); err != nil {
		return nil, 0, err
	}
	var total int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM change_authorization_history WHERE authorization_id = ?`, id).Scan(&total); err != nil {
		return nil, 0, repositoryError(err)
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id, authorization_id, event_id, action, actor_user_id, occurred_at
        FROM change_authorization_history WHERE authorization_id = ? ORDER BY occurred_at ASC, rowid ASC LIMIT ? OFFSET ?`, id, page.Limit, page.Offset)
	if err != nil {
		return nil, 0, repositoryError(err)
	}
	defer rows.Close()
	items := make([]domain.ChangeAuthorizationHistory, 0)
	for rows.Next() {
		var h domain.ChangeAuthorizationHistory
		var event sql.NullString
		var at string
		if err := rows.Scan(&h.ID, &h.AuthorizationID, &event, &h.Action, &h.ActorUserID, &at); err != nil {
			return nil, 0, repositoryError(err)
		}
		h.EventID = event.String
		if h.OccurredAt, err = parseTime(at); err != nil {
			return nil, 0, err
		}
		items = append(items, h)
	}
	return items, total, repositoryError(rows.Err())
}
