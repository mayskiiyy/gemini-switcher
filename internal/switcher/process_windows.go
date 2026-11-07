package switcher

import (
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

func RestartAntigravityLanguageServer() error {
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return err
	}
	defer windows.CloseHandle(snapshot)

	var entry windows.ProcessEntry32
	entry.Size = uint32(unsafe.Sizeof(entry))

	for err = windows.Process32First(snapshot, &entry); err == nil; err = windows.Process32Next(snapshot, &entry) {
		name := windows.UTF16ToString(entry.ExeFile[:])
		if strings.EqualFold(name, "language_server.exe") {
			hProc, openErr := windows.OpenProcess(windows.PROCESS_TERMINATE, false, entry.ProcessID)
			if openErr == nil {
				_ = windows.TerminateProcess(hProc, 1)
				_ = windows.CloseHandle(hProc)
			}
		}
	}
	return nil
}
