package main

import (
	"fmt"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	ole32            = windows.NewLazySystemDLL("ole32.dll")
	coInitialize     = ole32.NewProc("CoInitialize")
	coUninitialize   = ole32.NewProc("CoUninitialize")
	coCreateInstance = ole32.NewProc("CoCreateInstance")

	clsidShellLink  = windows.GUID{Data1: 0x00021401, Data2: 0x0000, Data3: 0x0000, Data4: [8]byte{0xC0, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x46}}
	iidIShellLinkW  = windows.GUID{Data1: 0x000214F9, Data2: 0x0000, Data3: 0x0000, Data4: [8]byte{0xC0, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x46}}
	iidIPersistFile = windows.GUID{Data1: 0x0000010B, Data2: 0x0000, Data3: 0x0000, Data4: [8]byte{0xC0, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x46}}
)

type iUnknownVtbl struct {
	QueryInterface uintptr
	AddRef         uintptr
	Release        uintptr
}

type iShellLinkWVtbl struct {
	iUnknownVtbl
	GetPath             uintptr
	GetIDList           uintptr
	SetIDList           uintptr
	GetDescription      uintptr
	SetDescription      uintptr
	GetWorkingDirectory uintptr
	SetWorkingDirectory uintptr
	GetArguments        uintptr
	SetArguments        uintptr
	GetHotkey           uintptr
	SetHotkey           uintptr
	GetShowCmd          uintptr
	SetShowCmd          uintptr
	GetIconLocation     uintptr
	SetIconLocation     uintptr
	SetRelativePath     uintptr
	Resolve             uintptr
	SetPath             uintptr
}

type iShellLinkW struct {
	lpVtbl *iShellLinkWVtbl
}

type iPersistFileVtbl struct {
	iUnknownVtbl
	GetClassID    uintptr
	IsDirty       uintptr
	Load          uintptr
	Save          uintptr
	SaveCompleted uintptr
	GetCurFile    uintptr
}

type iPersistFile struct {
	lpVtbl *iPersistFileVtbl
}

func createShortcut(dst, target, icon, workDir string) error {
	hr, _, _ := coInitialize.Call(0)
	if int32(hr) < 0 && hr != 1 {
		return fmt.Errorf("CoInitialize failed: %x", hr)
	}
	defer coUninitialize.Call()

	var shellLink *iShellLinkW
	hr, _, _ = coCreateInstance.Call(
		uintptr(unsafe.Pointer(&clsidShellLink)),
		0,
		1,
		uintptr(unsafe.Pointer(&iidIShellLinkW)),
		uintptr(unsafe.Pointer(&shellLink)),
	)
	if int32(hr) < 0 {
		return fmt.Errorf("CoCreateInstance failed: %x", hr)
	}
	defer syscall.SyscallN(shellLink.lpVtbl.Release, uintptr(unsafe.Pointer(shellLink)))

	targetPtr, _ := windows.UTF16PtrFromString(target)
	hr, _, _ = syscall.SyscallN(shellLink.lpVtbl.SetPath, uintptr(unsafe.Pointer(shellLink)), uintptr(unsafe.Pointer(targetPtr)))
	if int32(hr) < 0 {
		return fmt.Errorf("SetPath failed: %x", hr)
	}

	if workDir != "" {
		workDirPtr, _ := windows.UTF16PtrFromString(workDir)
		syscall.SyscallN(shellLink.lpVtbl.SetWorkingDirectory, uintptr(unsafe.Pointer(shellLink)), uintptr(unsafe.Pointer(workDirPtr)))
	}

	if icon != "" {
		iconPtr, _ := windows.UTF16PtrFromString(icon)
		syscall.SyscallN(shellLink.lpVtbl.SetIconLocation, uintptr(unsafe.Pointer(shellLink)), uintptr(unsafe.Pointer(iconPtr)), 0)
	}

	var persistFile *iPersistFile
	hr, _, _ = syscall.SyscallN(shellLink.lpVtbl.QueryInterface, uintptr(unsafe.Pointer(shellLink)), uintptr(unsafe.Pointer(&iidIPersistFile)), uintptr(unsafe.Pointer(&persistFile)))
	if int32(hr) < 0 {
		return fmt.Errorf("QueryInterface IPersistFile failed: %x", hr)
	}
	defer syscall.SyscallN(persistFile.lpVtbl.Release, uintptr(unsafe.Pointer(persistFile)))

	dstPtr, _ := windows.UTF16PtrFromString(dst)
	hr, _, _ = syscall.SyscallN(persistFile.lpVtbl.Save, uintptr(unsafe.Pointer(persistFile)), uintptr(unsafe.Pointer(dstPtr)), 1)
	if int32(hr) < 0 {
		return fmt.Errorf("Save failed: %x", hr)
	}

	return nil
}
