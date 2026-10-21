package privatefs

import (
	"path/filepath"
	"testing"

	"golang.org/x/sys/windows"
)

func TestWindowsPrivateAccessControl(t *testing.T) {
	root := filepath.Join(t.TempDir(), "private")
	if err := EnsureDir(root); err != nil {
		t.Fatal(err)
	}
	if err := WriteAtomic(root, "data", []byte("synthetic")); err != nil {
		t.Fatal(err)
	}
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{root, filepath.Join(root, "data")} {
		sd, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION)
		if err != nil {
			t.Fatal(err)
		}
		owner, _, err := sd.Owner()
		if err != nil || owner == nil || !owner.Equals(user.User.Sid) {
			t.Fatal("private path owner differs")
		}
		acl, _, err := sd.DACL()
		if err != nil || acl == nil || acl.AceCount != 1 {
			t.Fatal("private path allows more than its owner")
		}
		control, _, err := sd.Control()
		if err != nil || control&windows.SE_DACL_PROTECTED == 0 {
			t.Fatal("private path inherits access grants")
		}
	}
	if data, err := ReadFile(filepath.Join(root, "data")); err != nil || string(data) != "synthetic" {
		t.Fatal("current user cannot read private file")
	}
}
