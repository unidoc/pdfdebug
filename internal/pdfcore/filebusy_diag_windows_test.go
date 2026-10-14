//go:build windows

package pdfcore

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"unsafe"
)

var (
	diagKernel32                 = syscall.NewLazyDLL("kernel32.dll")
	diagGetVolumePathNameW       = diagKernel32.NewProc("GetVolumePathNameW")
	diagGetVolumeInformationW    = diagKernel32.NewProc("GetVolumeInformationW")
	diagGetFinalPathNameByHandle = diagKernel32.NewProc("GetFinalPathNameByHandleW")
)

func diagVolume(path string) string {
	p, _ := syscall.UTF16PtrFromString(path)
	vol := make([]uint16, 300)
	r, _, e := diagGetVolumePathNameW.Call(uintptr(unsafe.Pointer(p)), uintptr(unsafe.Pointer(&vol[0])), uintptr(len(vol)))
	if r == 0 {
		return fmt.Sprintf("GetVolumePathName failed: %v", e)
	}
	fs := make([]uint16, 64)
	var flags uint32
	r, _, e = diagGetVolumeInformationW.Call(uintptr(unsafe.Pointer(&vol[0])), 0, 0, 0, 0,
		uintptr(unsafe.Pointer(&flags)), uintptr(unsafe.Pointer(&fs[0])), uintptr(len(fs)))
	if r == 0 {
		return fmt.Sprintf("volume %s, GetVolumeInformation failed: %v", syscall.UTF16ToString(vol), e)
	}
	return fmt.Sprintf("volume %s fs %s flags 0x%x", syscall.UTF16ToString(vol), syscall.UTF16ToString(fs), flags)
}

func diagFinalPath(h syscall.Handle) string {
	buf := make([]uint16, 600)
	r, _, e := diagGetFinalPathNameByHandle.Call(uintptr(h), uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)), 0)
	if r == 0 {
		return fmt.Sprintf("GetFinalPathNameByHandle failed: %v", e)
	}
	return syscall.UTF16ToString(buf)
}

func diagLockAndOpen(t *testing.T, path string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "path %s\n  %s\n", path, diagVolume(path))
	p, _ := syscall.UTF16PtrFromString(path)
	h, err := syscall.CreateFile(p, syscall.GENERIC_READ, 0, nil, syscall.OPEN_EXISTING, syscall.FILE_ATTRIBUTE_NORMAL, 0)
	fmt.Fprintf(&b, "  lock CreateFile(share=0): handle=%v err=%v\n", h, err)
	if err != nil {
		return b.String()
	}
	defer func() { _ = syscall.CloseHandle(h) }()
	fmt.Fprintf(&b, "  lock handle final path: %s\n", diagFinalPath(h))

	f, err := os.Open(path)
	fmt.Fprintf(&b, "  os.Open: err=%v (%T)\n", err, err)
	if err == nil {
		fmt.Fprintf(&b, "  os.Open handle final path: %s\n", diagFinalPath(syscall.Handle(f.Fd())))
		_ = f.Close()
	}
	h2, err := syscall.CreateFile(p, syscall.GENERIC_READ, syscall.FILE_SHARE_READ|syscall.FILE_SHARE_WRITE, nil, syscall.OPEN_EXISTING, syscall.FILE_ATTRIBUTE_NORMAL, 0)
	fmt.Fprintf(&b, "  second CreateFile(GENERIC_READ, share=R|W): handle=%v err=%v\n", h2, err)
	if err == nil {
		_ = syscall.CloseHandle(h2)
	}
	h3, err := syscall.CreateFile(p, syscall.GENERIC_READ, 0, nil, syscall.OPEN_EXISTING, syscall.FILE_ATTRIBUTE_NORMAL, 0)
	fmt.Fprintf(&b, "  second CreateFile(GENERIC_READ, share=0): handle=%v err=%v\n", h3, err)
	if err == nil {
		_ = syscall.CloseHandle(h3)
	}
	h4, err := syscall.CreateFile(p, syscall.GENERIC_READ, syscall.FILE_SHARE_READ|syscall.FILE_SHARE_WRITE, nil, syscall.OPEN_EXISTING, syscall.FILE_ATTRIBUTE_NORMAL|syscall.FILE_FLAG_BACKUP_SEMANTICS, 0)
	fmt.Fprintf(&b, "  second CreateFile(GENERIC_READ, share=R|W, BACKUP_SEMANTICS): handle=%v err=%v\n", h4, err)
	if err == nil {
		_ = syscall.CloseHandle(h4)
	}
	_, err = NewInspector().Open("tab-diag", path)
	fmt.Fprintf(&b, "  Inspector.Open: err=%v\n", err)
	return b.String()
}

// Temporary: reports why a share-mode-0 lock does not block os.Open on the CI runner.
func TestFileBusyDiagnostic(t *testing.T) {
	report := "TEMP=" + os.Getenv("TEMP") + "\n"
	report += diagLockAndOpen(t, copyFixtureToTemp(t, "minimal.pdf"))

	wd, _ := os.Getwd()
	local := filepath.Join(wd, "filebusy-diag.pdf")
	data, err := os.ReadFile(filepath.Join(testdataDir(t), "minimal.pdf"))
	if err == nil {
		if err := os.WriteFile(local, data, 0o644); err == nil {
			defer func() { _ = os.Remove(local) }()
			report += diagLockAndOpen(t, local)
		} else {
			report += fmt.Sprintf("write %s: %v\n", local, err)
		}
	}
	out, err := exec.Command("whoami", "/priv").CombinedOutput()
	report += fmt.Sprintf("whoami /priv (err=%v):\n%s", err, out)
	t.Errorf("diagnostic report:\n%s", report)
}
