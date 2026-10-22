package scan

import (
	"io/fs"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"
)

var getDriveType = syscall.NewLazyDLL("kernel32.dll").NewProc("GetDriveTypeW")

func localAbs(path string) (string, error) {
	path = strings.ReplaceAll(path, "/", `\`)
	// 拡張長のドライブパスは通常表記に直し、UNC・デバイス名は開かない。
	if strings.HasPrefix(path, `\\?\`) && len(path) >= 7 && path[5] == ':' && path[6] == '\\' {
		path = path[4:]
	}
	if strings.HasPrefix(path, `\\`) || strings.Contains(path, "://") {
		return "", errNonLocal
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	if len(abs) < 3 || abs[1] != ':' || strings.Contains(abs[2:], ":") || !filepath.IsAbs(abs) {
		return "", errNonLocal
	}
	for _, part := range strings.Split(abs[3:], `\`) {
		if part != "" && !filepath.IsLocal(part) {
			return "", errNonLocal
		}
	}
	root, err := syscall.UTF16PtrFromString(abs[:3])
	if err != nil {
		return "", err
	}
	driveType, _, _ := getDriveType.Call(uintptr(unsafe.Pointer(root)))
	if err := checkDriveType(driveType); err != nil {
		return "", err
	}
	return abs, nil
}

func checkDriveType(kind uintptr) error {
	switch kind {
	case 2, 3, 5, 6: // 取り外し可能・固定・光学・RAMのローカルドライブ。
		return nil
	case 1: // ドライブルートが存在しない。
		return fs.ErrNotExist
	default: // 4はネットワークドライブ。0や未知の値も開かない。
		return errNonLocal
	}
}

func pathKey(path string) string { return strings.ToLower(path) }

func isLink(info fs.FileInfo) bool {
	if info.Mode()&fs.ModeSymlink != 0 {
		return true
	}
	// Go 1.23以降のジャンクションはModeIrregularなので属性も確認する。
	data, ok := info.Sys().(*syscall.Win32FileAttributeData)
	return ok && info.Mode()&fs.ModeIrregular != 0 && data.FileAttributes&syscall.FILE_ATTRIBUTE_REPARSE_POINT != 0
}
