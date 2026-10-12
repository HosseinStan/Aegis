package sqlite

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"company.local/aegis/apps/aegis-api/internal/domain"
	"company.local/aegis/apps/aegis-api/internal/policy"
	"company.local/aegis/apps/aegis-api/internal/repositories"
	"github.com/google/uuid"
	sqlitedriver "modernc.org/sqlite"
)

var _ repositories.ChangePolicyRepository = (*Store)(nil)

const policyColumns = `id, reference, device_id, config_type, status, engine_version, rules_json,
 definition_hash, created_by, created_at, activated_by, activated_at, retired_by, retired_at, updated_at`
const evaluationColumns = `id, event_id, policy_set_id, engine_version, policy_definition_hash,
 policy_definition_snapshot, result, total_items, allowed_count, denied_count, uncovered_count, evaluated_by, evaluated_at`

// Pin the connection through commit-error cleanup. SQLite can keep a deferred
// constraint transaction open after a failed COMMIT, while database/sql marks
// the Tx done. Roll back that same connection before returning it to the pool.
func (s *Store) policyTransaction(ctx context.Context, operation func(*sql.Tx) error) error {
	conn, err := s.db.Conn(ctx)
	if err != nil {
		return authorizationWriteError(err)
	}
	defer conn.Close()
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return authorizationWriteError(err)
	}
	defer tx.Rollback()
	if err := operation(tx); err != nil {
		return authorizationWriteError(err)
	}
	if err := tx.Commit(); err != nil {
		cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
		defer cancel()
		_, rollbackErr := conn.ExecContext(cleanup, "ROLLBACK")
		if rollbackErr != nil && !policyRollbackFinished(rollbackErr) {
			// Raw's ErrBadConn contract discards this physical connection rather
			// than returning a possibly unresolved transaction to the pool.
			discardErr := conn.Raw(func(any) error { return driver.ErrBadConn })
			failure := errors.Join(authorizationWriteError(err), fmt.Errorf("policy rollback cleanup failed: %w", rollbackErr))
			if !errors.Is(discardErr, driver.ErrBadConn) && !errors.Is(discardErr, sql.ErrConnDone) {
				return errors.Join(failure, fmt.Errorf("discard policy connection: %w", discardErr))
			}
			return failure
		}
		return authorizationWriteError(err)
	}
	return nil
}

func policyRollbackFinished(err error) bool {
	// modernc SQLite rolls back after a failed driver Commit. A redundant SQL
	// rollback then returns SQLITE_ERROR with this specific engine message.
	// sql.ErrTxDone alone is NOT proof that a backend transaction is resolved.
	var sqliteErr *sqlitedriver.Error
	return errors.As(err, &sqliteErr) && sqliteErr.Code() == 1 &&
		strings.Contains(sqliteErr.Error(), "cannot rollback - no transaction is active")
}

func validatePolicySet(p domain.PolicySet) error {
	in, err := domain.NormalizePolicyInput(domain.PolicySetInput{Reference: p.Reference, ConfigType: p.ConfigType, Rules: p.Rules})
	_, hash, hashErr := domain.CanonicalPolicyDefinition(p.Definition())
	if err != nil || hashErr != nil || p.ID == "" || p.Reference != in.Reference || !slices.Equal(p.Rules, in.Rules) ||
		p.DefinitionHash != hash || p.CreatedBy == "" || p.CreatedAt.IsZero() || p.UpdatedAt.Before(p.CreatedAt) {
		return domain.ErrValidation
	}
	switch p.Status {
	case domain.PolicyDraft:
		if p.ActivatedBy != "" || p.ActivatedAt != nil || p.RetiredBy != "" || p.RetiredAt != nil || !p.UpdatedAt.Equal(p.CreatedAt) {
			return domain.ErrValidation
		}
	case domain.PolicyActive, domain.PolicyRetired:
		if p.ActivatedBy == "" || p.ActivatedAt == nil || p.ActivatedAt.IsZero() || p.ActivatedAt.Before(p.CreatedAt) {
			return domain.ErrValidation
		}
		if p.Status == domain.PolicyActive {
			if p.RetiredBy != "" || p.RetiredAt != nil || !p.UpdatedAt.Equal(*p.ActivatedAt) {
				return domain.ErrValidation
			}
		} else if p.RetiredBy == "" || p.RetiredAt == nil || p.RetiredAt.Before(*p.ActivatedAt) || !p.UpdatedAt.Equal(*p.RetiredAt) {
			return domain.ErrValidation
		}
	default:
		return domain.ErrValidation
	}
	return nil
}

func scanPolicySet(scan scanFunc) (domain.PolicySet, error) {
	var p domain.PolicySet
	var rules, created, updated string
	var activatedBy, activatedAt, retiredBy, retiredAt sql.NullString
	err := scan(&p.ID, &p.Reference, &p.DeviceID, &p.ConfigType, &p.Status, &p.EngineVersion, &rules,
		&p.DefinitionHash, &p.CreatedBy, &created, &activatedBy, &activatedAt, &retiredBy, &retiredAt, &updated)
	if err != nil {
		return domain.PolicySet{}, repositoryError(err)
	}
	// Strict stored rule decoding rejects hidden/unsupported rule dimensions.
	d := json.NewDecoder(strings.NewReader(rules))
	d.DisallowUnknownFields()
	if err = d.Decode(&p.Rules); err != nil {
		return domain.PolicySet{}, domain.ErrValidation
	}
	encoded, _ := json.Marshal(p.Rules)
	if string(encoded) != rules {
		return domain.PolicySet{}, domain.ErrValidation
	}
	if p.CreatedAt, err = parseTime(created); err != nil {
		return domain.PolicySet{}, domain.ErrValidation
	}
	if p.UpdatedAt, err = parseTime(updated); err != nil {
		return domain.PolicySet{}, domain.ErrValidation
	}
	if p.ActivatedAt, err = parseNullableTime(activatedAt); err != nil {
		return domain.PolicySet{}, domain.ErrValidation
	}
	if p.RetiredAt, err = parseNullableTime(retiredAt); err != nil {
		return domain.PolicySet{}, domain.ErrValidation
	}
	p.ActivatedBy, p.RetiredBy = activatedBy.String, retiredBy.String
	if err := validatePolicySet(p); err != nil {
		return domain.PolicySet{}, err
	}
	return p, nil
}

func (s *Store) CreatePolicySet(ctx context.Context, p domain.PolicySet) error {
	if p.Status != domain.PolicyDraft {
		return domain.ErrValidation
	}
	if err := validatePolicySet(p); err != nil {
		return err
	}
	return s.policyTransaction(ctx, func(tx *sql.Tx) error {
		if err := authorizationActor(ctx, tx, p.DeviceID, p.CreatedBy, domain.PermissionPolicyManage); err != nil {
			return err
		}
		rules, _ := json.Marshal(p.Rules)
		_, err := tx.ExecContext(ctx, `INSERT INTO change_policy_sets (`+policyColumns+`) VALUES
 (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, NULL, NULL, NULL, NULL, ?)`, p.ID, p.Reference, p.DeviceID, p.ConfigType, p.Status,
			p.EngineVersion, string(rules), p.DefinitionHash, p.CreatedBy, authorizationTimestamp(p.CreatedAt), authorizationTimestamp(p.UpdatedAt))
		return err
	})
}

func (s *Store) GetPolicySet(ctx context.Context, deviceID, id string) (domain.PolicySet, error) {
	return scanPolicySet(s.db.QueryRowContext(ctx, `SELECT `+policyColumns+` FROM change_policy_sets WHERE device_id = ? AND id = ?`, deviceID, id).Scan)
}

func (s *Store) ListPolicySets(ctx context.Context, deviceID string, filter repositories.PolicySetFilter, page repositories.Page) ([]domain.PolicySet, int, error) {
	if !validAuthorizationPage(page) || !policyFilterValid(filter) {
		return nil, 0, domain.ErrValidation
	}
	where, args := ` WHERE device_id = ?`, []any{deviceID}
	if filter.Status != "" {
		where += ` AND status = ?`
		args = append(args, filter.Status)
	}
	if filter.ConfigType != "" {
		where += ` AND config_type = ?`
		args = append(args, filter.ConfigType)
	}
	var total int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM change_policy_sets`+where, args...).Scan(&total); err != nil {
		return nil, 0, repositoryError(err)
	}
	rows, err := s.db.QueryContext(ctx, `SELECT `+policyColumns+` FROM change_policy_sets`+where+` ORDER BY created_at DESC, id DESC LIMIT ? OFFSET ?`, append(args, page.Limit, page.Offset)...)
	if err != nil {
		return nil, 0, repositoryError(err)
	}
	defer rows.Close()
	result := []domain.PolicySet{}
	for rows.Next() {
		p, err := scanPolicySet(rows.Scan)
		if err != nil {
			return nil, 0, err
		}
		result = append(result, p)
	}
	return result, total, repositoryError(rows.Err())
}

func policyFilterValid(f repositories.PolicySetFilter) bool {
	return (f.Status == "" || f.Status == domain.PolicyDraft || f.Status == domain.PolicyActive || f.Status == domain.PolicyRetired) &&
		(f.ConfigType == "" || f.ConfigType == "running" || f.ConfigType == "startup")
}

func (s *Store) GetActivePolicySet(ctx context.Context, deviceID, configType string) (domain.PolicySet, error) {
	return scanPolicySet(s.db.QueryRowContext(ctx, `SELECT `+policyColumns+` FROM change_policy_sets WHERE device_id = ? AND config_type = ? AND status = 'active'`, deviceID, configType).Scan)
}

func (s *Store) ActivatePolicySet(ctx context.Context, deviceID, id, actor string, at time.Time) (domain.PolicySet, error) {
	return s.transitionPolicy(ctx, deviceID, id, actor, at, true)
}
func (s *Store) RetirePolicySet(ctx context.Context, deviceID, id, actor string, at time.Time) (domain.PolicySet, error) {
	return s.transitionPolicy(ctx, deviceID, id, actor, at, false)
}
func (s *Store) transitionPolicy(ctx context.Context, deviceID, id, actor string, at time.Time, activate bool) (domain.PolicySet, error) {
	var result domain.PolicySet
	if at.IsZero() {
		return result, domain.ErrValidation
	}
	err := s.policyTransaction(ctx, func(tx *sql.Tx) error {
		if err := authorizationActor(ctx, tx, deviceID, actor, domain.PermissionPolicyManage); err != nil {
			return err
		}
		p, err := scanPolicySet(tx.QueryRowContext(ctx, `SELECT `+policyColumns+` FROM change_policy_sets WHERE device_id = ? AND id = ?`, deviceID, id).Scan)
		if err != nil {
			return err
		}
		if at.Before(p.UpdatedAt) {
			return domain.ErrValidation
		}
		var write sql.Result
		if activate {
			if p.Status != domain.PolicyDraft {
				return domain.ErrConflict
			}
			write, err = tx.ExecContext(ctx, `UPDATE change_policy_sets SET status = 'active', activated_by = ?, activated_at = ?, updated_at = ? WHERE id = ? AND status = 'draft'`, actor, authorizationTimestamp(at), authorizationTimestamp(at), id)
			p.Status, p.ActivatedBy, p.ActivatedAt = domain.PolicyActive, actor, &at
		} else {
			if p.Status != domain.PolicyActive {
				return domain.ErrConflict
			}
			write, err = tx.ExecContext(ctx, `UPDATE change_policy_sets SET status = 'retired', retired_by = ?, retired_at = ?, updated_at = ? WHERE id = ? AND status = 'active'`, actor, authorizationTimestamp(at), authorizationTimestamp(at), id)
			p.Status, p.RetiredBy, p.RetiredAt = domain.PolicyRetired, actor, &at
		}
		if err != nil {
			return err
		}
		if n, err := write.RowsAffected(); err != nil {
			return err
		} else if n != 1 {
			return domain.ErrConflict
		}
		p.UpdatedAt = at
		result = p
		return nil
	})
	if err != nil {
		return domain.PolicySet{}, err
	}
	return result, nil
}

func scanPolicyEvaluation(scan scanFunc) (domain.ChangePolicyEvaluation, error) {
	var e domain.ChangePolicyEvaluation
	var at string
	if err := scan(&e.ID, &e.EventID, &e.PolicySetID, &e.EngineVersion, &e.PolicyDefinitionHash, &e.PolicyDefinitionSnapshot,
		&e.Result, &e.TotalItems, &e.AllowedCount, &e.DeniedCount, &e.UncoveredCount, &e.EvaluatedBy, &at); err != nil {
		return e, repositoryError(err)
	}
	var definition domain.PolicyDefinition
	d := json.NewDecoder(strings.NewReader(e.PolicyDefinitionSnapshot))
	d.DisallowUnknownFields()
	if err := d.Decode(&definition); err != nil {
		return domain.ChangePolicyEvaluation{}, domain.ErrValidation
	}
	canonical, hash, err := domain.CanonicalPolicyDefinition(definition)
	if err != nil || canonical != e.PolicyDefinitionSnapshot || hash != e.PolicyDefinitionHash || e.EngineVersion != definition.EngineVersion ||
		e.TotalItems <= 0 || e.AllowedCount < 0 || e.DeniedCount < 0 || e.UncoveredCount < 0 || e.TotalItems != e.AllowedCount+e.DeniedCount+e.UncoveredCount ||
		(e.Result != domain.PolicyStatusPass && e.Result != domain.PolicyStatusViolation) ||
		(e.Result == domain.PolicyStatusPass && (e.AllowedCount != e.TotalItems || e.DeniedCount != 0 || e.UncoveredCount != 0)) ||
		(e.Result == domain.PolicyStatusViolation && e.DeniedCount == 0) {
		return domain.ChangePolicyEvaluation{}, domain.ErrValidation
	}
	if e.EvaluatedAt, err = parseTime(at); err != nil || e.EvaluatedAt.IsZero() {
		return domain.ChangePolicyEvaluation{}, domain.ErrValidation
	}
	return e, nil
}

func (s *Store) GetPolicyEvaluation(ctx context.Context, deviceID, eventID string) (domain.ChangePolicyEvaluation, error) {
	// Scope the event first to avoid exposing a cross-device evaluation identity.
	var id string
	if err := s.db.QueryRowContext(ctx, `SELECT id FROM change_events WHERE device_id = ? AND id = ?`, deviceID, eventID).Scan(&id); err != nil {
		return domain.ChangePolicyEvaluation{}, repositoryError(err)
	}
	return scanPolicyEvaluation(s.db.QueryRowContext(ctx, `SELECT `+evaluationColumns+` FROM change_policy_evaluations WHERE event_id = ?`, eventID).Scan)
}

func (s *Store) ListPolicyEvaluationItems(ctx context.Context, deviceID, eventID string, page repositories.Page) ([]domain.ChangePolicyEvaluationItem, int, error) {
	if !validAuthorizationPage(page) {
		return nil, 0, domain.ErrValidation
	}
	evaluation, err := s.GetPolicyEvaluation(ctx, deviceID, eventID)
	if err != nil {
		return nil, 0, err
	}
	var total int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM change_policy_evaluation_items WHERE evaluation_id = ?`, evaluation.ID).Scan(&total); err != nil {
		return nil, 0, repositoryError(err)
	}
	if total != evaluation.TotalItems {
		return nil, 0, domain.ErrValidation
	}
	rows, err := s.db.QueryContext(ctx, `SELECT evaluation_id, event_id, change_event_item_id, ordinal, category, operation, decision, matched_rule_key
 FROM change_policy_evaluation_items WHERE evaluation_id = ? ORDER BY ordinal ASC LIMIT ? OFFSET ?`, evaluation.ID, page.Limit, page.Offset)
	if err != nil {
		return nil, 0, repositoryError(err)
	}
	defer rows.Close()
	items := []domain.ChangePolicyEvaluationItem{}
	for rows.Next() {
		var item domain.ChangePolicyEvaluationItem
		if err := rows.Scan(&item.EvaluationID, &item.EventID, &item.ChangeEventItemID, &item.Ordinal, &item.Category, &item.Operation, &item.Decision, &item.MatchedRuleKey); err != nil {
			return nil, 0, repositoryError(err)
		}
		items = append(items, item)
	}
	return items, total, repositoryError(rows.Err())
}

// loadPolicyPreview uses one SQLite snapshot for all evidence and policy reads.
// The same function is rerun inside final persistence, never trusting preview.
func loadPolicyPreview(ctx context.Context, tx *sql.Tx, deviceID, eventID string) (domain.PolicyEvaluationPreview, domain.ChangeEvent, domain.PolicySet, error) {
	preview := domain.PolicyEvaluationPreview{EventID: eventID, PolicyEvaluationResult: domain.PolicyEvaluationResult{
		Outcome: domain.PolicyStatusNotEvaluated, ItemResults: []domain.PolicyItemResult{}}}
	var selected domain.PolicySet
	e, err := scanChangeEvent(tx.QueryRowContext(ctx, `SELECT `+changeEventColumns+` FROM change_events WHERE device_id = ? AND id = ?`, deviceID, eventID).Scan)
	if err != nil {
		return preview, e, selected, err
	}
	existing, err := scanPolicyEvaluation(tx.QueryRowContext(ctx, `SELECT `+evaluationColumns+` FROM change_policy_evaluations WHERE event_id = ?`, eventID).Scan)
	if err == nil {
		preview.ExistingEvaluation = &existing
		if existing.Result != e.PolicyStatus {
			return preview, e, selected, domain.ErrValidation
		}
	} else if !errors.Is(err, domain.ErrNotFound) {
		return preview, e, selected, err
	} else if e.PolicyStatus != domain.PolicyStatusNotEvaluated {
		return preview, e, selected, domain.ErrValidation
	}
	rows, err := tx.QueryContext(ctx, `SELECT `+changeEventItemColumns+` FROM change_event_items WHERE change_event_id = ? ORDER BY ordinal ASC`, eventID)
	if err != nil {
		return preview, e, selected, repositoryError(err)
	}
	items := []domain.ChangeEventItem{}
	for rows.Next() {
		item, scanErr := scanAuthorizationItem(rows.Scan)
		if scanErr != nil {
			rows.Close()
			return preview, e, selected, domain.ErrValidation
		}
		items = append(items, item)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return preview, e, selected, repositoryError(err)
	}
	if e.DetectedAt.IsZero() || (e.ConfigType != "running" && e.ConfigType != "startup") || len(items) != e.ItemCount {
		return preview, e, selected, domain.ErrValidation
	}
	for i, item := range items {
		if item.ChangeEventID != e.ID || item.Ordinal != i {
			return preview, e, selected, domain.ErrValidation
		}
	}
	reason, err := policy.ValidateItems(items)
	if err != nil {
		return preview, e, selected, err
	}
	if e.AnalysisVersion == 0 && e.AnalyzedAt == nil && len(items) == 0 {
		preview.Reason = domain.PolicyReasonLegacy
		return preview, e, selected, nil
	}
	if e.AnalysisVersion != domain.ChangeAnalysisVersion || e.AnalyzedAt == nil || e.AnalyzedAt.IsZero() {
		return preview, e, selected, domain.ErrValidation
	}
	if reason != "" {
		preview.Reason = reason
		return preview, e, selected, nil
	}
	if len(items) == 0 {
		preview.Reason = domain.PolicyReasonInvalid
		return preview, e, selected, nil
	}
	selected, err = scanPolicySet(tx.QueryRowContext(ctx, `SELECT `+policyColumns+` FROM change_policy_sets WHERE device_id = ? AND config_type = ? AND status = 'active'`, deviceID, e.ConfigType).Scan)
	if errors.Is(err, domain.ErrNotFound) {
		preview.Reason = domain.PolicyReasonNoActive
		return preview, e, selected, nil
	}
	if err != nil {
		return preview, e, selected, err
	}
	// Parse absolute instants; never compare variable RFC3339 strings.
	if selected.ActivatedAt.After(e.DetectedAt) {
		preview.Reason = domain.PolicyReasonNoActive
		return preview, e, selected, nil
	}
	preview.PolicySetID, preview.PolicyDefinitionHash = selected.ID, selected.DefinitionHash
	preview.PolicyEvaluationResult, err = policy.Evaluate(selected.Definition(), items)
	return preview, e, selected, err
}

func (s *Store) PreviewPolicyEvaluation(ctx context.Context, deviceID, eventID, actor string) (domain.PolicyEvaluationPreview, error) {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return domain.PolicyEvaluationPreview{}, authorizationWriteError(err)
	}
	defer tx.Rollback()
	if err := authorizationActor(ctx, tx, deviceID, actor, "backups.read", domain.PermissionPolicyEvaluate); err != nil {
		return domain.PolicyEvaluationPreview{}, authorizationWriteError(err)
	}
	preview, _, _, err := loadPolicyPreview(ctx, tx, deviceID, eventID)
	if err != nil {
		return domain.PolicyEvaluationPreview{}, authorizationWriteError(err)
	}
	if err := tx.Commit(); err != nil {
		return domain.PolicyEvaluationPreview{}, authorizationWriteError(err)
	}
	return preview, nil
}

func (s *Store) PersistPolicyEvaluation(ctx context.Context, deviceID, eventID, actor string, at time.Time) (domain.ChangePolicyEvaluation, error) {
	var result domain.ChangePolicyEvaluation
	if at.IsZero() {
		return result, domain.ErrValidation
	}
	err := s.policyTransaction(ctx, func(tx *sql.Tx) error {
		if err := authorizationActor(ctx, tx, deviceID, actor, "backups.read", domain.PermissionPolicyEvaluate); err != nil {
			return err
		}
		// Repeated final writes always conflict, even after policy retirement.
		var status string
		if err := tx.QueryRowContext(ctx, `SELECT policy_status FROM change_events WHERE device_id = ? AND id = ?`, deviceID, eventID).Scan(&status); err != nil {
			return repositoryError(err)
		}
		var count int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM change_policy_evaluations WHERE event_id = ?`, eventID).Scan(&count); err != nil {
			return err
		}
		if status != domain.PolicyStatusNotEvaluated || count != 0 {
			return domain.ErrConflict
		}
		preview, event, selected, err := loadPolicyPreview(ctx, tx, deviceID, eventID)
		if err != nil {
			return err
		}
		if preview.Outcome == domain.PolicyStatusNotEvaluated {
			return domain.ErrValidation
		}
		if at.Before(event.DetectedAt) {
			return domain.ErrValidation
		}
		snapshot, hash, err := domain.CanonicalPolicyDefinition(selected.Definition())
		if err != nil || hash != selected.DefinitionHash {
			return domain.ErrValidation
		}
		result = domain.ChangePolicyEvaluation{ID: uuid.NewString(), EventID: eventID, PolicySetID: selected.ID,
			EngineVersion: domain.ChangePolicyEngineVersion, PolicyDefinitionHash: hash, PolicyDefinitionSnapshot: snapshot,
			Result: preview.Outcome, TotalItems: preview.TotalItems, AllowedCount: preview.AllowedCount, DeniedCount: preview.DeniedCount,
			UncoveredCount: preview.UncoveredCount, EvaluatedBy: actor, EvaluatedAt: at.UTC()}
		_, err = tx.ExecContext(ctx, `INSERT INTO change_policy_evaluations (`+evaluationColumns+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			result.ID, eventID, selected.ID, result.EngineVersion, hash, snapshot, result.Result, result.TotalItems, result.AllowedCount,
			result.DeniedCount, result.UncoveredCount, actor, authorizationTimestamp(at))
		if err != nil {
			return err
		}
		for _, item := range preview.ItemResults {
			_, err = tx.ExecContext(ctx, `INSERT INTO change_policy_evaluation_items
 (evaluation_id, event_id, change_event_item_id, ordinal, category, operation, decision, matched_rule_key) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
				result.ID, eventID, item.ItemID, item.Ordinal, item.Category, item.Operation, item.Decision, item.MatchedRuleKey)
			if err != nil {
				return err
			}
		}
		write, err := tx.ExecContext(ctx, `UPDATE change_events SET policy_status = ?, updated_at = ? WHERE id = ? AND device_id = ? AND policy_status = 'not_evaluated'`, result.Result, authorizationTimestamp(at), eventID, deviceID)
		if err != nil {
			return err
		}
		if n, err := write.RowsAffected(); err != nil {
			return err
		} else if n != 1 {
			return domain.ErrConflict
		}
		return nil
	})
	if err != nil {
		return domain.ChangePolicyEvaluation{}, err
	}
	return result, nil
}
