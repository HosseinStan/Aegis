package main

import (
	"errors"
	"os"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Protect the directory at creation time. Restart only accepts the same private
// ACL; inherited broad access is never silently repaired.
func privateDir(path string, fresh bool) error {
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return err
	}
	sid := user.User.Sid
	system, err := windows.CreateWellKnownSid(windows.WinLocalSystemSid)
	if err != nil || sid.Equals(system) {
		return errors.New("run Lab as a regular Windows user")
	}
	pathPtr, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	if fresh {
		descriptor, err := windows.SecurityDescriptorFromString("O:" + sid.String() + "D:P(A;OICI;FA;;;SY)(A;OICI;FA;;;" + sid.String() + ")")
		if err != nil {
			return err
		}
		attributes := windows.SecurityAttributes{SecurityDescriptor: descriptor}
		attributes.Length = uint32(unsafe.Sizeof(attributes))
		if err := windows.CreateDirectory(pathPtr, &attributes); err != nil {
			return err
		}
	}
	attr, err := windows.GetFileAttributes(pathPtr)
	info, statErr := os.Lstat(path)
	if err != nil || statErr != nil || !info.IsDir() || attr&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
		return errors.New("Lab data directories must be ordinary local directories")
	}
	descriptor, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		return err
	}
	control, _, controlErr := descriptor.Control()
	owner, _, ownerErr := descriptor.Owner()
	acl, _, aclErr := descriptor.DACL()
	if controlErr != nil || control&windows.SE_DACL_PROTECTED == 0 || ownerErr != nil || owner == nil || !owner.Equals(sid) || aclErr != nil || acl == nil || acl.AceCount != 2 {
		return errors.New("Lab data directory must retain its private owner/SYSTEM ACL")
	}
	var ownerRule, systemRule bool
	for index := uint32(0); index < 2; index++ {
		var ace *windows.ACCESS_ALLOWED_ACE
		if err := windows.GetAce(acl, index, &ace); err != nil || ace == nil ||
			ace.Header.AceType != windows.ACCESS_ALLOWED_ACE_TYPE ||
			ace.Header.AceFlags != windows.OBJECT_INHERIT_ACE|windows.CONTAINER_INHERIT_ACE ||
			ace.Mask != 0x1f01ff { // FILE_ALL_ACCESS in the resolved DACL.
			return errors.New("unexpected Lab directory access rule")
		}
		ruleSID := (*windows.SID)(unsafe.Pointer(&ace.SidStart))
		ownerRule = ownerRule || ruleSID.Equals(sid)
		systemRule = systemRule || ruleSID.Equals(system)
	}
	if !ownerRule || !systemRule {
		return errors.New("unexpected Lab directory principals")
	}
	return nil
}

func lockInstance(file *os.File) error {
	var overlapped windows.Overlapped
	return windows.LockFileEx(windows.Handle(file.Fd()), windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY, 0, 1, 0, &overlapped)
}

func protectKey(value []byte, encrypt bool) ([]byte, error) {
	const uiForbidden = 1 // CRYPTPROTECT_UI_FORBIDDEN
	input := windows.DataBlob{Size: uint32(len(value)), Data: &value[0]}
	var output windows.DataBlob
	var err error
	if encrypt {
		err = windows.CryptProtectData(&input, nil, nil, 0, nil, uiForbidden, &output)
	} else {
		err = windows.CryptUnprotectData(&input, nil, nil, 0, nil, uiForbidden, &output)
	}
	if err != nil {
		return nil, err
	}
	bytes := unsafe.Slice(output.Data, output.Size)
	result := append([]byte(nil), bytes...)
	clear(bytes)
	_, _ = windows.LocalFree(windows.Handle(uintptr(unsafe.Pointer(output.Data))))
	return result, nil
}
