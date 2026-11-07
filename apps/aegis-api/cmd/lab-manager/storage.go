package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"company.local/aegis/apps/aegis-api/internal/credentials"
	"company.local/aegis/apps/aegis-api/internal/repositories"
)

type labStorage struct {
	database string
	backups  string
	key      []byte
	lock     *os.File
}

func openLabStorage(path string) (labStorage, error) {
	const keyName = "credential-key.dpapi"
	const dbName = "aegis-lab.sqlite"
	var result labStorage
	path = filepath.Clean(path)
	parent := filepath.Dir(path)
	if !filepath.IsAbs(path) || parent == path || strings.ContainsAny(path, "?#") || strings.HasPrefix(path, `\\`) {
		return result, errors.New("the Lab data directory must be an absolute local path")
	}
	resolved, err := filepath.EvalSymlinks(parent)
	if err != nil || !strings.EqualFold(resolved, parent) {
		return result, errors.New("the Lab parent must already exist without symlinks or junctions")
	}
	for current := path; ; current = filepath.Dir(current) {
		if _, err := os.Lstat(filepath.Join(current, ".git")); !errors.Is(err, os.ErrNotExist) {
			return result, errors.New("the Lab storage must be outside Git checkouts")
		}
		if current == filepath.Dir(current) {
			break
		}
	}
	_, err = os.Lstat(path)
	fresh := errors.Is(err, os.ErrNotExist)
	if err := privateDir(path, fresh); err != nil {
		return result, err
	}
	root, err := os.OpenRoot(path)
	if err != nil {
		return result, err
	}
	defer root.Close()

	for _, name := range []string{dbName, dbName + "-wal", dbName + "-shm", dbName + "-journal", keyName, "instance.lock"} {
		info, err := root.Lstat(name)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil || !info.Mode().IsRegular() {
			return result, errors.New("unexpected file type in Lab storage")
		}
	}
	lock, err := root.OpenFile("instance.lock", os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return result, err
	}
	retained := false
	defer func() {
		if !retained {
			_ = lock.Close()
		}
	}()
	if err := lockInstance(lock); err != nil {
		return result, errors.New("the Lab data directory is already in use or cannot be locked")
	}
	backupPath := filepath.Join(path, "backups")
	if err := privateDir(backupPath, fresh); err != nil {
		return result, err
	}
	var protected []byte
	if fresh {
		key := randomBytes(32)
		defer clear(key)
		protected, err = protectKey(key, true)
		if err != nil {
			return result, errors.New("could not protect the Lab key with Windows DPAPI")
		}
		file, err := root.OpenFile(keyName, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err != nil {
			return result, err
		}
		_, writeErr := file.Write(protected)
		syncErr := file.Sync()
		if err := errors.Join(writeErr, syncErr, file.Close()); err != nil {
			return result, err
		}
	} else {
		info, err := root.Stat(keyName)
		if err != nil || info.Size() < 1 || info.Size() > 16<<10 {
			return result, errors.New("existing Lab storage must contain its original protected key")
		}
		protected, err = root.ReadFile(keyName)
		if err != nil {
			return result, err
		}
	}
	key, err := protectKey(protected, false)
	if err != nil || len(key) != 32 {
		clear(key)
		return result, errors.New("could not unlock this Lab credential key as the original Windows user")
	}
	retained = true
	return labStorage{database: filepath.Join(path, dbName), backups: backupPath, key: key, lock: lock}, nil
}

func checkVault(ctx context.Context, vault *credentials.Store) error {
	items, _, err := vault.List(ctx, repositories.Page{Limit: 1})
	if err != nil {
		return errors.New("could not read the Lab credential store")
	}
	if len(items) != 0 {
		if _, err := vault.Get(ctx, items[0].ID); err != nil {
			return errors.New("the Lab credential store cannot be decrypted")
		}
	}
	return nil
}
