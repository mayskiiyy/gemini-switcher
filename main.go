package main

import (
	"errors"
	"gemini-switcher/internal/autostart"
	"gemini-switcher/internal/gui"
	"gemini-switcher/internal/storage"
	"gemini-switcher/internal/switcher"
	"gemini-switcher/internal/tray"
	"log"
	"os"
	"path/filepath"

	"github.com/lxn/win"
	"golang.org/x/sys/windows"
)

func main() {
	mutexName, _ := windows.UTF16PtrFromString("Local\\GeminiSwitcherSingleInstanceMutex")
	eventName, _ := windows.UTF16PtrFromString("Local\\GeminiSwitcherOpenEvent")

	autostartMode := false
	for _, arg := range os.Args[1:] {
		if arg == "--autostart" || arg == "-autostart" {
			autostartMode = true
			break
		}
	}

	hMutex, err := windows.CreateMutex(nil, true, mutexName)
	if errors.Is(err, windows.ERROR_ALREADY_EXISTS) || errors.Is(err, windows.ERROR_ACCESS_DENIED) {
		if !autostartMode {
			if hEvent, err := windows.OpenEvent(windows.EVENT_MODIFY_STATE, false, eventName); err == nil {
				_ = windows.SetEvent(hEvent)
				_ = windows.CloseHandle(hEvent)
			}
			title, _ := windows.UTF16PtrFromString("Gemini Switcher")
			if hwnd := win.FindWindow(nil, title); hwnd != 0 {
				win.ShowWindow(hwnd, win.SW_SHOW)
				win.ShowWindow(hwnd, win.SW_RESTORE)
				win.SetForegroundWindow(hwnd)
			}
		}
		return
	}
	defer windows.CloseHandle(hMutex)

	if hEvent, err := windows.CreateEvent(nil, 0, 0, eventName); err == nil {
		go func() {
			defer windows.CloseHandle(hEvent)
			for {
				if r, _ := windows.WaitForSingleObject(hEvent, windows.INFINITE); r != windows.WAIT_OBJECT_0 {
					return
				}
				_ = gui.OpenWindow()
			}
		}()
	}

	_ = autostart.EnsureFlag()

	home, err := os.UserHomeDir()
	if err != nil {
		log.Fatalf("failed to get user home dir: %v", err)
	}

	dataDir := filepath.Join(home, ".gemini-switcher")
	st, err := storage.NewStorage(dataDir)
	if err != nil {
		log.Fatalf("failed to init storage: %v", err)
	}

	sw := switcher.NewSwitcher(st)

	gui.Configure(st, sw)
	if !autostartMode {
		if err := gui.OpenWindow(); err != nil {
			log.Printf("open control panel: %v", err)
		}
	}

	tray.Run(st, sw)
}
