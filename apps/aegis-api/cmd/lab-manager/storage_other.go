//go:build !windows

package main

import (
	"errors"
	"os"
)

func privateDir(string, bool) error {
	return errors.New("the L1 Lab storage profile requires Windows DPAPI and NTFS ACLs")
}

func lockInstance(*os.File) error {
	return errors.New("the Lab requires Windows file locking")
}

func protectKey([]byte, bool) ([]byte, error) {
	return nil, errors.New("the Lab requires Windows DPAPI")
}
