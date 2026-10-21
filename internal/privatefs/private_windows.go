package privatefs

import (
	"errors"
	"os"
	"path/filepath"
	"unsafe"

	"golang.org/x/sys/windows"
)

func privateAttributes() (*windows.SecurityAttributes, error) {
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return nil, err
	}
	sid := user.User.Sid.String()
	// Elevated tokens can default new objects to Administrators ownership.
	sd, err := windows.SecurityDescriptorFromString("O:" + sid + "D:P(A;OICI;FA;;;" + sid + ")")
	if err != nil {
		return nil, err
	}
	return &windows.SecurityAttributes{
		Length:             uint32(unsafe.Sizeof(windows.SecurityAttributes{})),
		SecurityDescriptor: sd,
	}, nil
}

func mkdirNative(path string) error {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	attributes, err := privateAttributes()
	if err != nil {
		return err
	}
	err = windows.CreateDirectory(p, attributes)
	if errors.Is(err, windows.ERROR_PATH_NOT_FOUND) {
		parent := filepath.Dir(path)
		if parent != path {
			if err := mkdirNative(parent); err != nil {
				return err
			}
			err = windows.CreateDirectory(p, attributes)
		}
	}
	if errors.Is(err, windows.ERROR_ALREADY_EXISTS) {
		return nil
	}
	if err != nil {
		return &os.PathError{Op: "mkdir", Path: path, Err: err}
	}
	return nil
}

func openNative(path string, flags int, directory bool) (*os.File, error) {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	access := uint32(windows.GENERIC_READ)
	if flags&(os.O_WRONLY|os.O_RDWR) != 0 {
		access |= windows.GENERIC_WRITE
	}
	disposition := uint32(windows.OPEN_EXISTING)
	if flags&os.O_CREATE != 0 {
		disposition = windows.OPEN_ALWAYS
	}
	if flags&(os.O_CREATE|os.O_EXCL) == os.O_CREATE|os.O_EXCL {
		disposition = windows.CREATE_NEW
	}
	attrs := uint32(windows.FILE_FLAG_OPEN_REPARSE_POINT)
	if directory {
		attrs |= windows.FILE_FLAG_BACKUP_SEMANTICS
	}
	var attributes *windows.SecurityAttributes
	if flags&os.O_CREATE != 0 {
		attributes, err = privateAttributes()
		if err != nil {
			return nil, err
		}
	}
	h, err := windows.CreateFile(
		p,
		access|windows.READ_CONTROL|windows.WRITE_DAC,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		attributes,
		disposition,
		attrs,
		0,
	)
	if err != nil {
		return nil, &os.PathError{Op: "open", Path: path, Err: err}
	}
	var info windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(h, &info); err != nil {
		return nil, errors.Join(err, windows.CloseHandle(h))
	}
	if info.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
		return nil, errors.Join(
			errors.New("private path cannot be a reparse point"),
			windows.CloseHandle(h),
		)
	}
	return os.NewFile(uintptr(h), path), nil
}

func protect(f *os.File, mode os.FileMode) error {
	attributes, err := privateAttributes()
	if err != nil {
		return err
	}
	user, _, err := attributes.SecurityDescriptor.Owner()
	if err != nil {
		return err
	}
	h := windows.Handle(f.Fd())
	sd, err := windows.GetSecurityInfo(h, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION)
	if err != nil {
		return err
	}
	owner, _, err := sd.Owner()
	if err != nil {
		return err
	}
	if owner == nil || user == nil || !owner.Equals(user) {
		return errors.New("private path is not owned by current user")
	}
	dacl, _, err := attributes.SecurityDescriptor.DACL()
	if err != nil {
		return err
	}
	return windows.SetSecurityInfo(
		h,
		windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION,
		nil,
		nil,
		dacl,
		nil,
	)
}
