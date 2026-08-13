//go:build windows

package secret_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"unsafe"

	secret "github.com/FlanChanXwO/pixiv-cli/internal/storage/file/secret"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/windows"
)

func TestWriteSecretFileAppliesExplicitPrivateWindowsSecurity(t *testing.T) {
	for _, force := range []bool{false, true} {
		t.Run(map[bool]string{false: "exclusive create", true: "force replace"}[force], func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "auth-export.json")
			if force {
				require.NoError(t, os.WriteFile(path, []byte("old"), 0o600))
				makeWindowsFileWorldAccessible(t, path)
			}

			require.NoError(t, secret.WriteSecretFile(path, []byte("secret"), force))
			assertPrivateWindowsFileSecurity(t, path)
		})
	}
}

func TestWriteSecretFileForceReplacesDifferentWindowsOwner(t *testing.T) {
	path := filepath.Join(t.TempDir(), "auth-export.json")
	require.NoError(t, os.WriteFile(path, []byte("old"), 0o600))
	makeWindowsFileWorldAccessible(t, path)
	administrators, err := windows.CreateWellKnownSid(windows.WinBuiltinAdministratorsSid)
	require.NoError(t, err)
	err = windows.SetNamedSecurityInfo(
		path,
		windows.SE_FILE_OBJECT,
		windows.OWNER_SECURITY_INFORMATION,
		administrators,
		nil,
		nil,
		nil,
	)
	if errors.Is(err, windows.ERROR_PRIVILEGE_NOT_HELD) || errors.Is(err, windows.ERROR_ACCESS_DENIED) {
		t.Skip("Windows runner cannot assign the different-owner fixture without additional privilege")
	}
	require.NoError(t, err)
	descriptor, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION)
	require.NoError(t, err)
	owner, _, err := descriptor.Owner()
	require.NoError(t, err)
	currentUser, err := windows.GetCurrentProcessToken().GetTokenUser()
	require.NoError(t, err)
	require.NotEqual(t, currentUser.User.Sid.String(), owner.String())

	require.NoError(t, secret.WriteSecretFile(path, []byte("secret"), true))
	assertPrivateWindowsFileSecurity(t, path)
}

func makeWindowsFileWorldAccessible(t *testing.T, path string) {
	t.Helper()
	descriptor, err := windows.SecurityDescriptorFromString("D:(A;;FA;;;WD)")
	require.NoError(t, err)
	dacl, _, err := descriptor.DACL()
	require.NoError(t, err)
	require.NoError(t, windows.SetNamedSecurityInfo(
		path,
		windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION,
		nil,
		nil,
		dacl,
		nil,
	))
}

func assertPrivateWindowsFileSecurity(t *testing.T, path string) {
	t.Helper()
	descriptor, err := windows.GetNamedSecurityInfo(
		path,
		windows.SE_FILE_OBJECT,
		windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION,
	)
	require.NoError(t, err)
	control, _, err := descriptor.Control()
	require.NoError(t, err)
	assert.NotZero(t, control&windows.SE_DACL_PROTECTED)

	currentUser, err := windows.GetCurrentProcessToken().GetTokenUser()
	require.NoError(t, err)
	owner, _, err := descriptor.Owner()
	require.NoError(t, err)
	assert.Equal(t, currentUser.User.Sid.String(), owner.String())
	wantSIDs := map[string]bool{
		currentUser.User.Sid.String(): false,
		"S-1-5-18":                    false,
		"S-1-5-32-544":                false,
	}
	dacl, _, err := descriptor.DACL()
	require.NoError(t, err)
	for index := uint32(0); ; index++ {
		var ace *windows.ACCESS_ALLOWED_ACE
		err := windows.GetAce(dacl, index, &ace)
		if err != nil {
			require.True(t, errors.Is(err, windows.ERROR_INVALID_PARAMETER), "GetAce(%d): %v", index, err)
			break
		}
		require.Equal(t, uint8(windows.ACCESS_ALLOWED_ACE_TYPE), ace.Header.AceType)
		assert.Zero(t, ace.Header.AceFlags&windows.INHERITED_ACE)
		sid := (*windows.SID)(unsafe.Pointer(&ace.SidStart))
		_, allowed := wantSIDs[sid.String()]
		require.True(t, allowed, "unexpected ACL principal %s", sid.String())
		wantSIDs[sid.String()] = true
		// FILE_ALL_ACCESS 未由 x/sys/windows 导出；该值由 Windows SDK
		// STANDARD_RIGHTS_REQUIRED | SYNCHRONIZE | 0x1ff 定义。
		const fileAllAccess windows.ACCESS_MASK = 0x1f01ff
		assert.Equal(t, fileAllAccess, ace.Mask)
	}
	for sid, found := range wantSIDs {
		assert.True(t, found, "missing ACL principal %s", sid)
	}
}
