package main

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"company.local/aegis/apps/aegis-api/internal/domain"
	"company.local/aegis/apps/aegis-api/internal/repositories/sqlite"
	"company.local/aegis/apps/aegis-api/internal/services"
	"github.com/google/uuid"
)

// This connection is used only during initialization of the brand-new demo
// database. There is no production role-management API or migration change.
func grantDemoBackupRead(ctx context.Context, databasePath string, store *sqlite.Store, userID string) error {
	db, err := sql.Open("sqlite", databasePath+"?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)")
	if err != nil {
		return err
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	const roleID = "role-isolated-demo-backup-reader"
	const roleName = "isolated_demo_backup_reader"
	if _, err = tx.ExecContext(ctx, "INSERT INTO roles(id,name,description) VALUES(?,?,?)", roleID, roleName, "Disposable demo: backup reads only"); err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, "INSERT INTO role_permissions(role_id,permission_id) SELECT ?,id FROM permissions WHERE name = ?", roleID, "backups.read")
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows != 1 {
		return errors.New("the existing backups.read permission is required")
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	return store.AssignRole(ctx, userID, roleName)
}

// CreateBackup is intentionally not used: it fetches device configurations.
// Seed fixed synthetic text into the same relative-path/hash conventions, then
// persist metadata through the existing repository and verify via BackupService.
func seedDemoSnapshots(ctx context.Context, store *sqlite.Store, service *services.BackupService, root, deviceID, hostname string, now time.Time) (int, error) {
	for revision := 1; revision <= 3; revision++ {
		id := uuid.NewString()
		captured := now.Add(-time.Duration(3-revision) * 24 * time.Hour)
		content := fmt.Sprintf("! AEGIS SYNTHETIC DEMO — DISPOSABLE BACKUP SNAPSHOT\n! Not retrieved from a device. Never apply.\nhostname %s\n! synthetic revision %d\ninterface DEMO-ONLY\n description Synthetic snapshot %02d\nend\n", hostname, revision, revision)
		relative := filepath.Join(deviceID, captured.Format("2006"), captured.Format("01"), captured.Format("20060102T150405.000000000Z")+"_running_"+id+".txt")
		filename := filepath.Join(root, relative)
		if err := os.MkdirAll(filepath.Dir(filename), 0750); err != nil {
			return 0, err
		}
		file, err := os.OpenFile(filename, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil {
			return 0, err
		}
		_, writeErr := io.WriteString(file, content)
		closeErr := file.Close()
		if writeErr != nil {
			return 0, writeErr
		}
		if closeErr != nil {
			return 0, closeErr
		}
		backup := domain.ConfigBackup{ID: id, DeviceID: deviceID, ConfigType: "running", StoragePath: relative,
			ContentHash: fmt.Sprintf("%x", sha256.Sum256([]byte(content))), SizeBytes: int64(len(content)),
			TriggerType: "demo_seed", RetrievedAt: captured, CreatedAt: captured}
		if err := store.CreateBackup(ctx, backup); err != nil {
			return 0, err
		}
		actual, text, err := service.Content(ctx, deviceID, id)
		if err != nil {
			return 0, err
		}
		if text != content || actual.ContentHash != backup.ContentHash || actual.SizeBytes != int64(len(content)) {
			return 0, errors.New("synthetic backup initialization verification failed")
		}
	}
	return 3, nil
}
