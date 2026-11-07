//go:build windows

package gui

import (
	"gemini-switcher/internal/model"
	"gemini-switcher/internal/storage"
	"gemini-switcher/internal/switcher"
	"github.com/lxn/walk"
	"github.com/lxn/win"
	"os"
	"runtime"
	"strings"
	"testing"
	"time"
)

// Local desktop regression check; opt in because it opens a real window.
func TestNativeLayout(t *testing.T) {
	if os.Getenv("GEMINI_UI_CHECK") == "" {
		t.Skip("desktop check is opt-in")
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	st, err := storage.NewStorage(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	accounts := []*model.AccountProfile{
		{Email: "personal@example.com", Quota5H: model.QuotaBucket{RemainingFraction: .92, ResetTime: time.Now().Add(3 * time.Hour)}, QuotaWeekly: model.QuotaBucket{RemainingFraction: .78, ResetTime: time.Now().Add(48 * time.Hour)}},
		{Email: strings.Repeat("long-address", 8) + "@example.com", Alias: strings.Repeat("Long account name ", 12), Paused: true},
		{Email: "work@example.com", Quota5H: model.QuotaBucket{RemainingFraction: 1}, QuotaWeekly: model.QuotaBucket{RemainingFraction: 1}},
	}
	if err := st.SaveAccounts(accounts); err != nil {
		t.Fatal(err)
	}
	a := &nativeUI{st: st, sw: switcher.NewSwitcher(st)}
	if err := a.create(); err != nil {
		t.Fatal(err)
	}
	defer a.window.Dispose()
	if a.lang != model.ResolveLanguage("auto") {
		t.Errorf("first launch language = %q, expected system UI language", a.lang)
	}
	for _, b := range []*walk.PushButton{a.capture, a.refresh, a.settings, a.switchBtn, a.pause, a.alias, a.remove} {
		if b.Background() != walk.NullBrush() {
			t.Fatal("rounded button corners must inherit the dark parent background")
		}
		if b.GraphicsEffects().Len() != 0 {
			t.Fatal("native graphics effects must not paint borders over custom buttons")
		}
	}
	a.window.SetTitle("Gemini Switcher — UI check")
	go func() {
		time.Sleep(1000 * time.Millisecond)
		var initial walk.Rectangle
		var listWidth, panelWidth int
		a.window.Synchronize(func() {
			initial = a.window.Bounds()
			listWidth = a.list.Parent().Bounds().Width
			panelWidth = a.detail.Parent().Bounds().Width
		})
		for n := 0; n < 15; n++ {
			a.window.Synchronize(func() { _ = a.list.SetCurrentIndex(n % len(accounts)); a.applyLanguage(languages[n%len(languages)]) })
			time.Sleep(120 * time.Millisecond)
			a.window.Synchronize(func() {
				if a.window.Bounds() != initial || a.list.Parent().Bounds().Width != listWidth || a.detail.Parent().Bounds().Width != panelWidth {
					t.Errorf("selection %d changed layout: window %v -> %v; list %d -> %d; detail %d -> %d", n, initial, a.window.Bounds(), listWidth, a.list.Parent().Bounds().Width, panelWidth, a.detail.Parent().Bounds().Width)
				}
			})
		}
		a.window.Synchronize(func() {
			a.applyLanguage("ru")
			_ = a.list.SetCurrentIndex(0)
			a.window.SetSize(walk.Size{Width: 920, Height: 600})
		})
		time.Sleep(150 * time.Millisecond)
		a.window.Synchronize(func() {
			for _, w := range []walk.Widget{a.detail, a.state, a.quota5, a.quotaWeek, a.switchBtn, a.pause, a.alias, a.remove} {
				r := w.Bounds()
				parent := w.Parent().ClientBounds()
				if r.X < 0 || r.Y < 0 || r.X+r.Width > parent.Width || r.Y+r.Height > parent.Height {
					t.Errorf("widget %T clipped: %v within %v", w, r, parent)
				}
			}
			modelBefore := &a.list.Model().([]string)[0]
			isDirty := func(hwnd win.HWND) bool {
				dirty, _, _ := user32.NewProc("GetUpdateRect").Call(uintptr(hwnd), 0, 0)
				return dirty != 0
			}
			h := a.capture.Handle()
			win.SendMessage(h, win.WM_MOUSEMOVE, 0, 5|(5<<16))
			if !buttonHovered(h) {
				t.Fatal("hover did not activate")
			}
			win.UpdateWindow(h)
			for n := 0; n < 100; n++ {
				win.SendMessage(h, win.WM_MOUSEMOVE, 0, 6|(6<<16))
			}
			if isDirty(h) {
				t.Fatal("mouse movement repeatedly repainted an unchanged button")
			}
			win.SendMessage(h, win.WM_MOUSELEAVE, 0, 0)
			if buttonHovered(h) {
				t.Fatal("hover did not clear")
			}
			win.UpdateWindow(a.list.Handle())
			for n := 0; n < 100; n++ {
				win.SendMessage(a.list.Handle(), win.WM_MOUSEMOVE, 0, 20|(20<<16))
			}
			if isDirty(a.list.Handle()) {
				t.Fatal("mouse movement repainted rows with no hover state")
			}
			a.reload("")
			if &a.list.Model().([]string)[0] != modelBefore {
				t.Fatal("unchanged account list was reset")
			}
			a.accounts[0].Quota5H.RemainingFraction = .42
			if err := st.SaveAccounts(a.accounts); err != nil {
				t.Fatal(err)
			}
			a.reload("")
			if &a.list.Model().([]string)[0] != modelBefore || a.value5 != 42 {
				t.Fatal("quota refresh reset the list or failed to update details")
			}
			if os.Getenv("GEMINI_UI_PERF") == "1" {
				start := time.Now()
				for n := 0; n < 30; n++ {
					_ = a.list.SetCurrentIndex(n % len(accounts))
					win.RedrawWindow(a.window.Handle(), nil, 0, win.RDW_INVALIDATE|win.RDW_ALLCHILDREN|win.RDW_UPDATENOW)
				}
				t.Logf("30 selections with synchronous full repaint: %s", time.Since(start))
			}
			if os.Getenv("GEMINI_UI_SETTINGS") == "1" {
				a.openSettings()
			}
			if os.Getenv("GEMINI_UI_PREVIEW") == "" {
				a.window.Close()
			}
		})
	}()
	a.window.Run()
}

func TestTranslationFallback(t *testing.T) {
	a := &nativeUI{lang: "unsupported"}
	if got := a.t("Настройки"); got != "Settings" {
		t.Fatalf("missing translation = %q, want English", got)
	}
	a.lang = "ru"
	if got := a.t("Настройки"); got != "Настройки" {
		t.Fatalf("Russian source translation changed: %q", got)
	}
}
