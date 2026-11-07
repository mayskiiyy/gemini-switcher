package tray

import (
	"fmt"
	"gemini-switcher/internal/autostart"
	"gemini-switcher/internal/credmgr"
	"gemini-switcher/internal/gui"
	"gemini-switcher/internal/model"
	"gemini-switcher/internal/notify"
	"gemini-switcher/internal/quota"
	"gemini-switcher/internal/storage"
	"gemini-switcher/internal/switcher"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/getlantern/systray"
)

type TrayApp struct {
	storage  *storage.Storage
	switcher *switcher.Switcher
	mu       sync.Mutex

	mOpenWindow  *systray.MenuItem
	mStatus      *systray.MenuItem
	mRefresh     *systray.MenuItem
	mCapture     *systray.MenuItem
	mAutoSwitch  *systray.MenuItem
	mAutostart   *systray.MenuItem
	mOpenFolder  *systray.MenuItem
	mQuit        *systray.MenuItem
	accountItems []*accountMenuItem
}

type accountMenuItem struct {
	mu      sync.Mutex
	item    *systray.MenuItem
	account *model.AccountProfile
}

func Run(st *storage.Storage, sw *switcher.Switcher) {
	app := &TrayApp{
		storage:  st,
		switcher: sw,
	}
	systray.Run(app.onReady, app.onExit)
}

func (app *TrayApp) onReady() {
	systray.SetIcon(IconBytes)
	systray.SetTitle("Gemini Switcher")
	systray.SetTooltip("Gemini Switcher: инициализация...")

	app.mOpenWindow = systray.AddMenuItem("🖥 Панель управления", "Открыть графическое окно управления")
	systray.AddSeparator()

	app.mStatus = systray.AddMenuItem("Инициализация...", "")
	app.mStatus.Disable()

	systray.AddSeparator()

	app.mRefresh = systray.AddMenuItem("🔄 Обновить квоту сейчас", "Запросить актуальные квоты из Antigravity")
	app.mCapture = systray.AddMenuItem("➕ Захватить текущую сессию в пул", "Сохранить текущий аккаунт Antigravity в пул")

	systray.AddSeparator()

	app.accountItems = make([]*accountMenuItem, 25)
	for i := 0; i < 25; i++ {
		item := systray.AddMenuItem("", "")
		item.Hide()
		slot := &accountMenuItem{
			item: item,
		}
		app.accountItems[i] = slot
		go app.watchSlotClick(slot)
	}

	systray.AddSeparator()

	cfg, _ := app.storage.LoadConfig()
	app.mAutoSwitch = systray.AddMenuItemCheckbox("⚙ Автопереключение", "Включить/выключить автопереключение", cfg.AutoSwitchEnabled)

	isAuto, _ := autostart.IsEnabled()
	app.mAutostart = systray.AddMenuItemCheckbox("🚀 Запуск с Windows", "Запускать утилиту при старте системы", isAuto)

	app.mOpenFolder = systray.AddMenuItem("📁 Открыть папку данных (.gemini-switcher)", "Открыть папку конфигурации в проводнике")

	systray.AddSeparator()
	app.mQuit = systray.AddMenuItem("❌ Выход", "Закрыть Gemini Switcher")

	app.refreshDisplay()

	go app.watchEvents()
	go app.pollingLoop()
}

func (app *TrayApp) onExit() {
}

func (app *TrayApp) sendNotify(category, title, msg string) {
	cfg, _ := app.storage.LoadConfig()
	if cfg == nil || !cfg.NotifyOnSwitch || cfg.NotifyMode == "none" {
		return
	}
	if cfg.NotifyMode == "switch_only" && category != "switch" && category != "rotate" {
		return
	}
	go notify.SendNotification(title, msg)
}

func (app *TrayApp) watchEvents() {
	go func() {
		for range app.mQuit.ClickedCh {
			gui.PrepareExit()
			systray.Quit()
			return
		}
	}()

	go func() {
		for range app.mOpenWindow.ClickedCh {
			_ = gui.OpenWindow()
		}
	}()

	go func() {
		for range app.mRefresh.ClickedCh {
			go app.refreshQuotasAndRotate(true)
		}
	}()

	go func() {
		for range app.mCapture.ClickedCh {
			go app.handleCapture()
		}
	}()

	go func() {
		for range app.mAutoSwitch.ClickedCh {
			cfg, _ := app.storage.LoadConfig()
			cfg.AutoSwitchEnabled = !cfg.AutoSwitchEnabled
			_ = app.storage.SaveConfig(cfg)
			if cfg.AutoSwitchEnabled {
				app.mAutoSwitch.Check()
				app.sendNotify("info", "Автопереключение", "Автопереключение включено")
			} else {
				app.mAutoSwitch.Uncheck()
				app.sendNotify("info", "Автопереключение", "Автопереключение выключено")
			}
			app.refreshDisplay()
		}
	}()

	go func() {
		for range app.mAutostart.ClickedCh {
			curr, _ := autostart.IsEnabled()
			next := !curr
			err := autostart.SetEnabled(next)
			if err == nil {
				if next {
					app.mAutostart.Check()
					app.sendNotify("info", "Автозапуск", "Автозапуск с Windows включен")
				} else {
					app.mAutostart.Uncheck()
					app.sendNotify("info", "Автозапуск", "Автозапуск с Windows отключен")
				}
			}
		}
	}()

	go func() {
		for range app.mOpenFolder.ClickedCh {
			dir := filepath.Dir(app.storage.AccountsFilePath())
			_ = os.MkdirAll(dir, 0700)
			cmd := exec.Command("explorer.exe", dir)
			cmd.SysProcAttr = &syscall.SysProcAttr{
				HideWindow:    true,
				CreationFlags: 0x08000000,
			}
			_ = cmd.Start()
		}
	}()
}

func (app *TrayApp) getActiveEmail() string {
	activeBlob, err := credmgr.ReadAntigravityCredential()
	if err != nil {
		return ""
	}
	email, _ := credmgr.ParseSessionEmail(activeBlob)
	return email
}

func (app *TrayApp) watchSlotClick(slot *accountMenuItem) {
	for range slot.item.ClickedCh {
		slot.mu.Lock()
		target := slot.account
		slot.mu.Unlock()

		if target != nil {
			go func(acc *model.AccountProfile) {
				err := app.switcher.SwitchTo(acc)
				if err != nil {
					app.sendNotify("error", "Ошибка", fmt.Sprintf("Ошибка переключения: %v", err))
				} else {
					app.sendNotify("switch", "Переключение аккаунта", fmt.Sprintf("Активен: %s", acc.Email))
					app.refreshDisplay()
					go app.refreshQuotasAndRotate(false)
				}
			}(target)
		}
	}
}

func (app *TrayApp) handleCapture() {
	acc, err := app.switcher.CaptureCurrentAccount()
	if err != nil {
		app.sendNotify("error", "Ошибка", fmt.Sprintf("Ошибка захвата: %v", err))
		return
	}

	app.refreshDisplay()
	msg := fmt.Sprintf("Аккаунт %s сохранен в пул (5ч: %.0f%%, Нед: %.0f%%)",
		acc.Email, acc.Quota5H.RemainingFraction*100, acc.QuotaWeekly.RemainingFraction*100)
	app.sendNotify("info", "Сессия сохранена", msg)
}

func (app *TrayApp) pollingLoop() {
	time.Sleep(3 * time.Second)
	app.refreshQuotasAndRotate(false)
	for {
		cfg, _ := app.storage.LoadConfig()
		interval := 60
		if cfg != nil && cfg.CheckIntervalSeconds > 0 {
			interval = cfg.CheckIntervalSeconds
		}
		time.Sleep(time.Duration(interval) * time.Second)
		app.refreshQuotasAndRotate(false)
	}
}

func (app *TrayApp) refreshQuotasAndRotate(userTriggered bool) {
	app.mu.Lock()
	defer app.mu.Unlock()

	activeEmail := app.getActiveEmail()
	qRes, qErr := quota.FetchActiveQuota()
	if qErr != nil && userTriggered {
		app.sendNotify("warning", "Предупреждение", fmt.Sprintf("Квоты: %v", qErr))
	}

	switched, rotErr := app.switcher.RotateIfNeeded(activeEmail, qRes)
	if rotErr != nil && userTriggered {
		app.sendNotify("warning", "Внимание", fmt.Sprintf("%v", rotErr))
	} else if switched != nil {
		app.sendNotify("rotate", "Ротация аккаунта", fmt.Sprintf("Лимит исчерпан. Переключено на %s (5ч: %.0f%%)",
			switched.Email, switched.Quota5H.RemainingFraction*100))
	}

	app.refreshDisplay()
}

func (app *TrayApp) refreshDisplay() {
	activeEmail := app.getActiveEmail()

	accs, _ := app.storage.LoadAccounts()
	var currentAcc *model.AccountProfile
	for _, a := range accs {
		if a.Email == activeEmail {
			currentAcc = a
			break
		}
	}

	cfg, _ := app.storage.LoadConfig()
	iconColor := "green"
	if cfg != nil && !cfg.AutoSwitchEnabled {
		iconColor = "gray"
	} else if currentAcc != nil {
		if currentAcc.Quota5H.RemainingFraction <= 0.05 || currentAcc.QuotaWeekly.RemainingFraction <= 0.05 {
			iconColor = "red"
		} else if currentAcc.Quota5H.RemainingFraction <= 0.20 {
			iconColor = "yellow"
		} else {
			iconColor = "green"
		}
	} else if activeEmail != "" {
		iconColor = "yellow"
	}
	systray.SetIcon(GetIconBytes(iconColor))

	statusText := "● Нет активного аккаунта"
	tooltip := "Gemini Switcher: нет активного аккаунта"
	if currentAcc != nil {
		rem5h := currentAcc.Quota5H.RemainingFraction * 100
		remWk := currentAcc.QuotaWeekly.RemainingFraction * 100
		name := currentAcc.Email
		if currentAcc.Alias != "" {
			name = fmt.Sprintf("%s (%s)", currentAcc.Alias, currentAcc.Email)
		}
		statusText = fmt.Sprintf("● %s [5ч: %.0f%% | Неделя: %.0f%%]", name, rem5h, remWk)
		tooltip = fmt.Sprintf("Gemini Switcher: %s (5ч: %.0f%% | Нед: %.0f%%)", name, rem5h, remWk)
	} else if activeEmail != "" {
		statusText = fmt.Sprintf("● %s (не сохранен в пуле)", activeEmail)
		tooltip = fmt.Sprintf("Gemini Switcher: %s (не в пуле)", activeEmail)
	}

	app.mStatus.SetTitle(statusText)
	systray.SetTooltip(tooltip)

	now := time.Now().UTC()
	for i, slot := range app.accountItems {
		if i < len(accs) {
			a := accs[i]
			slot.mu.Lock()
			slot.account = a
			slot.mu.Unlock()

			prefix := "○ "
			if a.Email == activeEmail {
				prefix = "✓ "
			}

			displayName := a.Email
			if a.Alias != "" {
				displayName = fmt.Sprintf("[%s] %s", a.Alias, a.Email)
			}
			if a.Paused {
				displayName = "[ПАУЗА] " + displayName
			}

			info := ""
			if a.Quota5H.RemainingFraction <= 0.05 && !a.Quota5H.ResetTime.IsZero() && now.Before(a.Quota5H.ResetTime) {
				diff := time.Until(a.Quota5H.ResetTime).Round(time.Minute)
				localTime := a.Quota5H.ResetTime.Local().Format("15:04")
				info = fmt.Sprintf("[5ч: сброс в %s (~%v)]", localTime, diff)
			} else if a.QuotaWeekly.RemainingFraction <= 0.05 && !a.QuotaWeekly.ResetTime.IsZero() && now.Before(a.QuotaWeekly.ResetTime) {
				diff := time.Until(a.QuotaWeekly.ResetTime).Round(time.Hour)
				localDate := a.QuotaWeekly.ResetTime.Local().Format("02.01 15:04")
				info = fmt.Sprintf("[Нед: сброс %s (~%v)]", localDate, diff)
			} else {
				info = fmt.Sprintf("[5ч: %.0f%%, Нед: %.0f%%]", a.Quota5H.RemainingFraction*100, a.QuotaWeekly.RemainingFraction*100)
			}

			title := fmt.Sprintf("%s%s %s", prefix, displayName, info)
			slot.item.SetTitle(title)
			slot.item.Show()
		} else {
			slot.mu.Lock()
			slot.account = nil
			slot.mu.Unlock()
			slot.item.Hide()
		}
	}
}
