//go:build windows

package secret

import (
	"fmt"
	"os"
	"runtime"
	"unsafe"

	"golang.org/x/sys/windows"
)

func openSecretFileExclusive(path string) (*os.File, error) {
	descriptor, err := currentUserSecretDescriptor()
	if err != nil {
		return nil, err
	}
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	attributes := &windows.SecurityAttributes{
		Length:             uint32(unsafe.Sizeof(windows.SecurityAttributes{})),
		SecurityDescriptor: descriptor,
	}
	handle, err := windows.CreateFile(
		name,
		windows.GENERIC_READ|windows.GENERIC_WRITE,
		0,
		attributes,
		windows.CREATE_NEW,
		windows.FILE_ATTRIBUTE_NORMAL,
		0,
	)
	runtime.KeepAlive(descriptor)
	if err != nil {
		return nil, err
	}
	return os.NewFile(uintptr(handle), path), nil
}

func applySecretFileProtection(*os.File) error {
	// 安全描述符已随 CREATE_NEW 原子应用，不能改用继承 ACL 后补设的窗口。
	return nil
}

func applySecretPathProtection(path string) error {
	descriptor, err := currentUserSecretDescriptor()
	if err != nil {
		return err
	}
	dacl, _, err := descriptor.DACL()
	if err != nil {
		return err
	}
	owner, _, err := descriptor.Owner()
	if err != nil {
		return err
	}
	// ReplaceFileW 可能从旧 target 保留 DACL；replacement 已提交后必须重设
	// protected DACL。此后失败由上层准确报告 committed，不能伪造 rollback。
	err = windows.SetNamedSecurityInfo(
		path,
		windows.SE_FILE_OBJECT,
		windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION,
		owner,
		nil,
		dacl,
		nil,
	)
	runtime.KeepAlive(descriptor)
	return err
}

func currentUserSecretDescriptor() (*windows.SECURITY_DESCRIPTOR, error) {
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return nil, err
	}
	// D:P 禁止继承 parent DACL；除当前用户外仅保留 Windows 运维所需的
	// LocalSystem 与 builtin Administrators，三者均获得完整文件控制权。
	sddl := fmt.Sprintf("O:%sD:P(A;;FA;;;%s)(A;;FA;;;SY)(A;;FA;;;BA)", user.User.Sid.String(), user.User.Sid.String())
	return windows.SecurityDescriptorFromString(sddl)
}
