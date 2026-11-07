package model

import "syscall"

func systemLanguage() string {
	id, _, _ := syscall.NewLazyDLL("kernel32.dll").NewProc("GetUserDefaultUILanguage").Call()
	// The primary language ID ignores regional variants of the display language.
	switch id & 0x3ff {
	case 0x19:
		return "ru"
	case 0x07:
		return "de"
	case 0x15:
		return "pl"
	case 0x22:
		return "uk"
	}
	return "en"
}
