package tray

import (
	"bytes"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"unsafe"
)

func TestIcons(t *testing.T) {
	icons := map[string][]byte{
		"green":  IconGreen,
		"yellow": IconYellow,
		"red":    IconRed,
		"gray":   IconGray,
	}

	for name, b := range icons {
		if len(b) < 22 {
			t.Fatalf("icon %s is truncated: %d bytes", name, len(b))
		}
		if b[0] != 0 || b[1] != 0 || b[2] != 1 || b[3] != 0 {
			t.Fatalf("icon %s has invalid ICO header", name)
		}
	}

	if bytes.Equal(IconGreen, IconYellow) || bytes.Equal(IconGreen, IconRed) || bytes.Equal(IconGreen, IconGray) {
		t.Fatalf("icons must be distinct")
	}

	if !bytes.Equal(GetIconBytes("green"), IconGreen) {
		t.Fatalf("expected IconGreen for green")
	}
	if !bytes.Equal(GetIconBytes("yellow"), IconYellow) {
		t.Fatalf("expected IconYellow for yellow")
	}
	if !bytes.Equal(GetIconBytes("red"), IconRed) {
		t.Fatalf("expected IconRed for red")
	}
	if !bytes.Equal(GetIconBytes("gray"), IconGray) {
		t.Fatalf("expected IconGray for gray")
	}
	if !bytes.Equal(GetIconBytes("unknown"), IconBytes) {
		t.Fatalf("expected default IconBytes for unknown")
	}

	user32 := syscall.NewLazyDLL("user32.dll")
	loadImageW := user32.NewProc("LoadImageW")
	destroyIcon := user32.NewProc("DestroyIcon")

	tmpDir := t.TempDir()
	for name, b := range icons {
		tmpPath := filepath.Join(tmpDir, name+".ico")
		if err := os.WriteFile(tmpPath, b, 0644); err != nil {
			t.Fatalf("failed to write tmp ico: %v", err)
		}
		ptr, err := syscall.UTF16PtrFromString(tmpPath)
		if err != nil {
			t.Fatalf("failed to convert path to utf16: %v", err)
		}
		h, _, _ := loadImageW.Call(0, uintptr(unsafe.Pointer(ptr)), 1, 0, 0, 0x10|0x40)
		if h == 0 {
			t.Fatalf("Windows LoadImageW failed to load %s icon", name)
		}
		destroyIcon.Call(h)
	}
}
