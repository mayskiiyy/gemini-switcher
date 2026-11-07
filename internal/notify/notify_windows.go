package notify

import (
	"fmt"
	"os"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

type notifyIconData struct {
	Size                       uint32
	Window                     windows.Handle
	ID, Flags, CallbackMessage uint32
	Icon                       windows.Handle
	Tip                        [128]uint16
	State, StateMask           uint32
	Info                       [256]uint16
	Timeout                    uint32
	InfoTitle                  [64]uint16
	InfoFlags                  uint32
	GUID                       windows.GUID
	BalloonIcon                windows.Handle
}

func SendNotification(title, message string) error {
	user32 := windows.NewLazySystemDLL("user32.dll")
	enumWindows := user32.NewProc("EnumWindows")
	getPID := user32.NewProc("GetWindowThreadProcessId")
	getClass := user32.NewProc("GetClassNameW")
	var window uintptr
	callback := syscall.NewCallback(func(hwnd, _ uintptr) uintptr {
		var pid uint32
		getPID.Call(hwnd, uintptr(unsafe.Pointer(&pid)))
		if pid != uint32(os.Getpid()) {
			return 1
		}
		var class [128]uint16
		getClass.Call(hwnd, uintptr(unsafe.Pointer(&class[0])), uintptr(len(class)))
		if windows.UTF16ToString(class[:]) == "SystrayClass" {
			window = hwnd
			return 0
		}
		return 1
	})
	enumWindows.Call(callback, 0)
	if window == 0 {
		return fmt.Errorf("tray window not found")
	}
	data := notifyIconData{Window: windows.Handle(window), ID: 100, Flags: 0x10, InfoFlags: 1}
	data.Size = uint32(unsafe.Sizeof(data))
	copy(data.InfoTitle[:len(data.InfoTitle)-1], syscall.StringToUTF16(title))
	copy(data.Info[:len(data.Info)-1], syscall.StringToUTF16(message))
	result, _, err := windows.NewLazySystemDLL("shell32.dll").NewProc("Shell_NotifyIconW").Call(1, uintptr(unsafe.Pointer(&data)))
	if result == 0 {
		return fmt.Errorf("show tray notification: %w", err)
	}
	return nil
}
