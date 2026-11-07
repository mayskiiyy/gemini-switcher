package gui

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"gemini-switcher/internal/autostart"
	"gemini-switcher/internal/credmgr"
	"gemini-switcher/internal/model"
	"gemini-switcher/internal/notify"
	"gemini-switcher/internal/quota"
	"gemini-switcher/internal/storage"
	"gemini-switcher/internal/switcher"
	"gemini-switcher/internal/updater"
	"log"
	"runtime"
	"slices"
	"sync"
	"time"

	"github.com/lxn/walk"
	. "github.com/lxn/walk/declarative"
	"github.com/lxn/win"
	"golang.org/x/sys/windows"
)

//go:embed translations.json
var translationJSON []byte
var translations map[string]map[string]string
var languages = []string{"ru", "en", "de", "pl", "uk"}
var nativeMu sync.Mutex
var nativeStorage *storage.Storage
var nativeSwitcher *switcher.Switcher
var nativeWindow *walk.MainWindow
var opening bool
var exiting bool

func init()                       { _ = json.Unmarshal(translationJSON, &translations) }
func setGlobalServer(string, int) {}
func Configure(st *storage.Storage, sw *switcher.Switcher) {
	nativeMu.Lock()
	nativeStorage, nativeSwitcher = st, sw
	nativeMu.Unlock()
}
func (s *Server) OpenWindow() error { Configure(s.storage, s.sw); return OpenWindow() }

func PrepareExit() {
	nativeMu.Lock()
	exiting = true
	win := nativeWindow
	nativeMu.Unlock()
	if win != nil {
		win.Synchronize(func() {
			win.Close()
		})
	}
}

func OpenWindow() error {
	nativeMu.Lock()
	if nativeWindow != nil {
		w := nativeWindow
		nativeMu.Unlock()
		w.Synchronize(func() {
			w.SetVisible(true)
			hwnd := w.Handle()
			win.ShowWindow(hwnd, win.SW_RESTORE)
			win.SetForegroundWindow(hwnd)
		})
		return nil
	}
	if opening {
		nativeMu.Unlock()
		return nil
	}
	if nativeStorage == nil || nativeSwitcher == nil {
		nativeMu.Unlock()
		return fmt.Errorf("window not configured")
	}
	opening = true
	st, sw := nativeStorage, nativeSwitcher
	nativeMu.Unlock()
	ready := make(chan error, 1)
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		app := &nativeUI{st: st, sw: sw}
		err := app.create()
		nativeMu.Lock()
		opening = false
		if err == nil {
			nativeWindow = app.window
		}
		nativeMu.Unlock()
		ready <- err
		if err != nil {
			return
		}
		done := make(chan struct{})
		go func() {
			ticker := time.NewTicker(10 * time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-done:
					return
				case <-ticker.C:
					app.window.Synchronize(func() {
						if app.window != nil && !app.busy {
							app.reload("")
						}
					})
				}
			}
		}()
		app.window.Run()
		close(done)
		nativeMu.Lock()
		if nativeWindow == app.window {
			nativeWindow = nil
		}
		isExit := exiting
		nativeMu.Unlock()
		app.window.Dispose()
		if !isExit {
			cfg, _ := st.LoadConfig()
			if cfg == nil || (cfg.NotifyOnSwitch && cfg.NotifyMode != "none") {
				go notify.SendNotification("Gemini Switcher", app.t("Приложение работает в системном трее"))
			}
		}
	}()
	return <-ready
}

type nativeUI struct {
	st                                                                  *storage.Storage
	sw                                                                  *switcher.Switcher
	window                                                              *walk.MainWindow
	accounts                                                            []*model.AccountProfile
	listNames                                                           []string
	active, lang                                                        string
	list                                                                *walk.ListBox
	capture, refresh, settings, switchBtn, pause, remove, alias         *walk.PushButton
	title, accountsLabel, activeLabel, detail, state, quota5, quotaWeek *walk.Label
	bar5, barWeek                                                       *walk.CustomWidget
	value5, valueWeek                                                   int
	changing, busy                                                      bool
}

func (a *nativeUI) t(s string) string {
	if a.lang == "ru" {
		return s
	}
	if v := translations[s][a.lang]; v != "" {
		return v
	}
	if v := translations[s]["en"]; v != "" {
		return v
	}
	return s
}
func (a *nativeUI) create() error {
	cfg, err := a.st.LoadConfig()
	if err != nil {
		return err
	}
	a.lang = model.ResolveLanguage(cfg.Language)
	a.changing = true
	err = (MainWindow{
		AssignTo: &a.window, Title: "Gemini Switcher", Size: Size{960, 680}, MinSize: Size{880, 600},
		Background: SolidColorBrush{Color: canvasColor}, Font: Font{Family: "Segoe UI", PointSize: 10},
		Layout: VBox{Margins: Margins{Left: 24, Top: 20, Right: 24, Bottom: 20}, Spacing: 14},
		Children: []Widget{
			Composite{Layout: HBox{MarginsZero: true, Spacing: 10}, Children: []Widget{
				Composite{MinSize: Size{260, 0}, Layout: VBox{MarginsZero: true, Spacing: 4}, Children: []Widget{
					Label{AssignTo: &a.title, Text: "Gemini Switcher", TextColor: textColor, Font: Font{Family: "Segoe UI", PointSize: 18, Bold: true}, EllipsisMode: EllipsisEnd},
					Label{AssignTo: &a.activeLabel, EllipsisMode: EllipsisEnd, TextColor: mutedColor, Font: Font{Family: "Segoe UI", PointSize: 9}},
				}},
				HSpacer{},
				PushButton{AssignTo: &a.capture, MaxSize: Size{168, 0}, MinSize: Size{168, 36}, Text: a.t("Захватить сессию"), OnClicked: func() {
					a.runTask(a.t("Захват..."), a.t("Захват..."), a.capture, func() error { _, e := a.sw.CaptureCurrentAccount(); return e })
				}},
				PushButton{AssignTo: &a.refresh, MaxSize: Size{140, 0}, MinSize: Size{120, 36}, Text: a.t("Обновить"), OnClicked: func() {
					a.runTask(a.t("Обновление..."), a.t("Обновление..."), a.refresh, a.refreshQuota)
				}},
				PushButton{AssignTo: &a.settings, MaxSize: Size{110, 0}, MinSize: Size{110, 36}, Text: a.t("Настройки"), OnClicked: a.openSettings},
			}},
			Composite{
				MinSize: Size{0, 140}, MaxSize: Size{0, 140},
				Background: SolidColorBrush{Color: panelColor},
				Layout: VBox{Margins: Margins{Left: 22, Top: 16, Right: 22, Bottom: 16}, Spacing: 12},
				Children: []Widget{
					Composite{Layout: HBox{MarginsZero: true, Spacing: 12}, Children: []Widget{
						Label{AssignTo: &a.detail, EllipsisMode: EllipsisEnd, TextColor: textColor, Font: Font{Family: "Segoe UI", PointSize: 14, Bold: true}, StretchFactor: 1},
						Label{AssignTo: &a.state, EllipsisMode: EllipsisEnd, TextColor: successColor, TextAlignment: AlignFar, Font: Font{Family: "Segoe UI", PointSize: 9, Bold: true}},
					}},
					Composite{Layout: HBox{MarginsZero: true, Spacing: 28}, Children: []Widget{
						Composite{StretchFactor: 1, MinSize: Size{320, 0}, Layout: VBox{MarginsZero: true, Spacing: 6}, Children: []Widget{
							Label{AssignTo: &a.quota5, EllipsisMode: EllipsisEnd, TextColor: mutedColor, Font: Font{Family: "Segoe UI", PointSize: 9}},
							a.meter(&a.bar5, func() int { return a.value5 }),
						}},
						Composite{StretchFactor: 1, MinSize: Size{320, 0}, Layout: VBox{MarginsZero: true, Spacing: 6}, Children: []Widget{
							Label{AssignTo: &a.quotaWeek, EllipsisMode: EllipsisEnd, TextColor: mutedColor, Font: Font{Family: "Segoe UI", PointSize: 9}},
							a.meter(&a.barWeek, func() int { return a.valueWeek }),
						}},
					}},
				},
			},
			Composite{StretchFactor: 1, Layout: VBox{MarginsZero: true, Spacing: 8}, Children: []Widget{
				Label{AssignTo: &a.accountsLabel, Text: a.t("Аккаунты"), TextColor: mutedColor, Font: Font{Family: "Segoe UI", PointSize: 9, Bold: true}},
				accountList{ListBox{AssignTo: &a.list, MinSize: Size{0, 220}, Background: SolidColorBrush{Color: canvasColor}, ItemStyler: accountStyler{ui: a}, Model: []string{}, OnCurrentIndexChanged: a.updateDetails}},
			}},
			Composite{Layout: HBox{MarginsZero: true, Spacing: 10}, Children: []Widget{
				PushButton{AssignTo: &a.switchBtn, MinSize: Size{160, 36}, Text: a.t("Переключить"), OnClicked: func() {
					if acc := a.selected(); acc != nil {
						a.run(func() error { return a.sw.SwitchTo(acc) })
					}
				}},
				PushButton{AssignTo: &a.pause, MinSize: Size{100, 36}, Text: a.t("Пауза"), OnClicked: func() {
					if acc := a.selected(); acc != nil {
						a.run(func() error { return a.sw.UpdateAccount(acc.Email, acc.Alias, !acc.Paused, acc.Priority) })
					}
				}},
				PushButton{AssignTo: &a.alias, MinSize: Size{130, 36}, Text: a.t("Имя аккаунта"), OnClicked: a.editAlias},
				HSpacer{},
				PushButton{AssignTo: &a.remove, MinSize: Size{150, 36}, Text: a.t("Удалить из пула"), OnClicked: a.deleteAccount},
			}},
		},
	}).Create()
	if err != nil {
		return err
	}
	a.window.Closing().Attach(func(canceled *bool, reason walk.CloseReason) {
		nativeMu.Lock()
		isExit := exiting
		nativeMu.Unlock()
		if !isExit {
			*canceled = true
			a.window.SetVisible(false)
			cfg, _ := a.st.LoadConfig()
			if cfg == nil || (cfg.NotifyOnSwitch && cfg.NotifyMode != "none") {
				go notify.SendNotification("Gemini Switcher", a.t("Приложение работает в системном трее"))
			}
		}
	})
	if icon, e := walk.NewIconFromResourceId(2); e == nil {
		_ = a.window.SetIcon(icon)
	}
	setSubclass.Call(uintptr(win.GetParent(a.list.Handle())), listProc, 2, uintptr(a.list.Handle()))
	styleButtons(a.capture, a.capture, a.refresh, a.settings)
	styleButtons(a.switchBtn, a.switchBtn, a.pause, a.alias, a.remove)
	styleDarkControls(a.window, a.list)
	a.changing = false
	a.reload("")
	return nil
}
func (a *nativeUI) selected() *model.AccountProfile {
	if a.list == nil {
		return nil
	}
	i := a.list.CurrentIndex()
	if i < 0 || i >= len(a.accounts) {
		return nil
	}
	return a.accounts[i]
}
func (a *nativeUI) activeAccount() *model.AccountProfile {
	if a.active == "" {
		return nil
	}
	for _, acc := range a.accounts {
		if acc.Email == a.active {
			return acc
		}
	}
	return nil
}
func (a *nativeUI) reload(preferred string) {
	if acc := a.selected(); preferred == "" && acc != nil {
		preferred = acc.Email
	}
	accounts, err := a.st.LoadAccounts()
	if err != nil {
		walk.MsgBox(a.window, "Gemini Switcher", err.Error(), walk.MsgBoxIconError)
		return
	}
	suspended := a.window.Suspended()
	a.window.SetSuspended(true)
	defer a.window.SetSuspended(suspended)
	a.accounts = accounts
	_ = a.accountsLabel.SetText(fmt.Sprintf("%s · %d", a.t("Пул аккаунтов"), len(accounts)))
	a.active = ""
	if blob, e := credmgr.ReadAntigravityCredential(); e == nil {
		a.active, _ = credmgr.ParseSessionEmail(blob)
	}
	cfg, _ := a.st.LoadConfig()
	if cfg != nil && cfg.AutoSwitchEnabled {
		_ = a.activeLabel.SetText("● " + a.t("Автопереключение включено"))
		a.activeLabel.SetTextColor(successColor)
	} else {
		_ = a.activeLabel.SetText("⏸ " + a.t("Автопереключение выключено"))
		a.activeLabel.SetTextColor(warningColor)
	}
	names := make([]string, 0, len(accounts))
	index := -1
	for i, acc := range accounts {
		name := acc.Email
		if acc.Alias != "" {
			name = acc.Alias + "  —  " + name
		}
		if acc.Email == a.active {
			name = "✓  " + name
		}
		if acc.Paused {
			name += "  (" + a.t("пауза") + ")"
		}
		names = append(names, name)
		if acc.Email == preferred || preferred == "" && acc.Email == a.active {
			index = i
		}
	}
	a.changing = true
	if !slices.Equal(names, a.listNames) {
		_ = a.list.SetModel(names)
		a.listNames = names
	}
	if index < 0 && len(names) > 0 {
		index = 0
	}
	_ = a.list.SetCurrentIndex(index)
	a.changing = false
	a.updateDetails()
	_ = a.list.Invalidate()
}
func percent(v float64) int {
	return min(100, max(0, int(v*100+0.5)))
}
func (a *nativeUI) updateDetails() {
	if a.changing || a.detail == nil {
		return
	}
	suspended := a.window.Suspended()
	a.window.SetSuspended(true)
	defer a.window.SetSuspended(suspended)
	acc := a.selected()
	enabled := acc != nil
	a.switchBtn.SetEnabled(enabled && !a.busy && acc.Email != a.active)
	a.pause.SetEnabled(enabled && !a.busy)
	a.remove.SetEnabled(enabled && !a.busy)
	a.alias.SetEnabled(enabled && !a.busy)
	if !enabled {
		_ = a.detail.SetText(a.t("Аккаунты"))
		_ = a.state.SetText(a.t("Захватить сессию"))
		a.state.SetTextColor(mutedColor)
		_ = a.detail.SetToolTipText("")

		_ = a.quota5.SetText("")
		_ = a.quotaWeek.SetText("")
		a.value5, a.valueWeek = 0, 0
		_ = a.bar5.Invalidate()
		_ = a.barWeek.Invalidate()
		return
	}
	title := acc.Email
	if acc.Alias != "" {
		title = fmt.Sprintf("%s (%s)", acc.Alias, acc.Email)
	}
	_ = a.detail.SetText(title)
	_ = a.detail.SetToolTipText(title)
	label := a.t("Готов к переключению")
	labelColor := accentColor
	if acc.Email == a.active {
		label = a.t("● активен")
		labelColor = successColor
	}
	if acc.Paused {
		label = a.t("пауза")
		labelColor = warningColor
	}
	a.state.SetTextColor(labelColor)
	_ = a.state.SetText(label)
	_ = a.state.SetToolTipText(label)
	_ = a.quota5.SetText(fmt.Sprintf("%s: %d%%  %s", a.t("5ч лимит"), percent(acc.Quota5H.RemainingFraction), countdown(acc.Quota5H.ResetTime)))
	_ = a.quotaWeek.SetText(fmt.Sprintf("%s: %d%%  %s", a.t("Недельный лимит"), percent(acc.QuotaWeekly.RemainingFraction), countdown(acc.QuotaWeekly.ResetTime)))
	a.value5, a.valueWeek = percent(acc.Quota5H.RemainingFraction), percent(acc.QuotaWeekly.RemainingFraction)
	_ = a.bar5.Invalidate()
	_ = a.barWeek.Invalidate()
	if acc.Paused {
		_ = a.pause.SetText(a.t("Включить"))
	} else {
		_ = a.pause.SetText(a.t("Пауза"))
	}
}
func countdown(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	d := time.Until(t)
	if d <= 0 {
		return ""
	}
	if d >= 24*time.Hour {
		return fmt.Sprintf("~%dd (%s)", int(d.Hours()/24), t.Local().Format("02.01 15:04"))
	}
	return fmt.Sprintf("~%dh %dm (%s)", int(d.Hours()), int(d.Minutes())%60, t.Local().Format("15:04"))
}
func (a *nativeUI) run(fn func() error) {
	a.runTask("", "", nil, fn)
}

func (a *nativeUI) runTask(statusText, buttonText string, btn *walk.PushButton, fn func() error) {
	a.busy = true
	a.settings.SetEnabled(false)
	a.capture.SetEnabled(false)
	a.refresh.SetEnabled(false)
	a.switchBtn.SetEnabled(false)
	a.pause.SetEnabled(false)
	a.remove.SetEnabled(false)
	a.alias.SetEnabled(false)
	if btn != nil && buttonText != "" {
		_ = btn.SetText(buttonText)
	}
	if statusText != "" {
		_ = a.state.SetText(statusText)
	}
	go func() {
		err := fn()
		a.window.Synchronize(func() {
			a.busy = false
			a.settings.SetEnabled(true)
			a.capture.SetEnabled(true)
			a.refresh.SetEnabled(true)
			_ = a.capture.SetText(a.t("Захватить сессию"))
			_ = a.refresh.SetText(a.t("Обновить"))
			if err != nil {
				walk.MsgBox(a.window, "Gemini Switcher", err.Error(), walk.MsgBoxIconError)
			}
			a.reload("")
		})
	}()
}
func (a *nativeUI) refreshQuota() error {
	blob, err := credmgr.ReadAntigravityCredential()
	if err != nil {
		return err
	}
	email, err := credmgr.ParseSessionEmail(blob)
	if err != nil {
		return err
	}
	q, err := quota.FetchActiveQuota()
	if err != nil {
		return err
	}
	_, err = a.sw.RotateIfNeeded(email, q)
	return err
}
func (a *nativeUI) deleteAccount() {
	acc := a.selected()
	if acc == nil {
		return
	}
	if walk.MsgBox(a.window, "Gemini Switcher", a.t("Удалить аккаунт ")+acc.Email+a.t(" из пула?"), walk.MsgBoxYesNo|walk.MsgBoxIconQuestion) == walk.DlgCmdYes {
		a.run(func() error { return a.sw.DeleteAccount(acc.Email) })
	}
}
func (a *nativeUI) editAlias() {
	acc := a.selected()
	if acc == nil {
		return
	}
	var dialog *walk.Dialog
	var name *walk.LineEdit
	var value string
	var cancel, apply *walk.PushButton
	err := (Dialog{AssignTo: &dialog, CancelButton: &cancel, DefaultButton: &apply, Title: a.t("Имя аккаунта"), Size: Size{460, 200}, MinSize: Size{460, 200}, Background: SolidColorBrush{Color: canvasColor}, Font: Font{Family: "Segoe UI", PointSize: 10}, Layout: VBox{Margins: Margins{Left: 24, Top: 20, Right: 24, Bottom: 20}, Spacing: 14}, Children: []Widget{Label{Text: acc.Email, EllipsisMode: EllipsisEnd, TextColor: textColor}, LineEdit{AssignTo: &name, Background: SolidColorBrush{Color: cardColor}, TextColor: textColor, Text: acc.Alias}, Composite{Layout: HBox{}, Children: []Widget{HSpacer{}, PushButton{AssignTo: &cancel, MinSize: Size{100, 34}, Text: a.t("Отмена"), OnClicked: func() { dialog.Cancel() }}, PushButton{AssignTo: &apply, MinSize: Size{120, 34}, Text: a.t("Применить"), OnClicked: func() { value = name.Text(); dialog.Accept() }}}}}}).Create(a.window)
	if err != nil {
		walk.MsgBox(a.window, "Gemini Switcher", err.Error(), walk.MsgBoxIconError)
		return
	}
	defer dialog.Dispose()
	styleButtons(apply, cancel, apply)
	styleDarkControls(dialog, name)
	dialog.Run()
	if dialog.Result() == walk.DlgCmdOK {
		a.run(func() error { return a.sw.UpdateAccount(acc.Email, value, acc.Paused, acc.Priority) })
	}
}
func (a *nativeUI) applyLanguage(lang string) {
	a.lang = model.ResolveLanguage(lang)
	_ = a.title.SetText("Gemini Switcher")
	_ = a.capture.SetText(a.t("Захватить сессию"))
	_ = a.refresh.SetText(a.t("Обновить"))
	_ = a.settings.SetText(a.t("Настройки"))
	_ = a.accountsLabel.SetText(a.t("Аккаунты"))
	_ = a.switchBtn.SetText(a.t("Переключить"))
	_ = a.remove.SetText(a.t("Удалить из пула"))
	_ = a.alias.SetText(a.t("Имя аккаунта"))
	a.reload("")
}
func (a *nativeUI) openSettings() {
	a.busy = true
	defer func() { a.busy = false; a.updateDetails() }()
	cfg, err := a.st.LoadConfig()
	if err != nil {
		walk.MsgBox(a.window, "Gemini Switcher", err.Error(), walk.MsgBoxIconError)
		return
	}
	auto, _ := autostart.IsEnabled()
	var dlg *walk.Dialog
	var start, rotate *walk.CheckBox
	var five, week, mode, language *walk.ComboBox
	var cancel, apply, checkBtn, dlBtn *walk.PushButton
	var statusLbl *walk.Label
	var latestURL string
	languageOptions := append([]string{"auto"}, languages...)
	thresholds := []float64{0.01, 0.05, 0.10}
	modes := []string{"all", "switch_only", "none"}
	selectedLang := max(0, slices.Index(languageOptions, cfg.Language))
	thresholdIndex := max(0, slices.Index(thresholds, cfg.Threshold5H))
	thresholdWeeklyIndex := max(0, slices.Index(thresholds, cfg.ThresholdWeekly))
	mi := max(0, slices.Index(modes, cfg.NotifyMode))
	err = (Dialog{AssignTo: &dlg, CancelButton: &cancel, DefaultButton: &apply, Title: a.t("Настройки"), Size: Size{480, 640}, MinSize: Size{480, 640}, FixedSize: true, Background: SolidColorBrush{Color: canvasColor}, Font: Font{Family: "Segoe UI", PointSize: 10}, Layout: VBox{Margins: Margins{Left: 24, Top: 22, Right: 24, Bottom: 22}, Spacing: 14}, Children: []Widget{
		Label{Text: a.t("Язык"), TextColor: textColor},
		ComboBox{AssignTo: &language, Background: SolidColorBrush{Color: cardColor}, Model: []string{a.t("Системный язык"), "Русский", "English", "Deutsch", "Polski", "Українська"}, CurrentIndex: selectedLang},
		Composite{Layout: HBox{MarginsZero: true}, Children: []Widget{CheckBox{AssignTo: &start, Text: a.t("Автозапуск с Windows"), Checked: auto}, HSpacer{}}},
		Composite{Layout: HBox{MarginsZero: true}, Children: []Widget{CheckBox{AssignTo: &rotate, Text: a.t("Автопереключение"), Checked: cfg.AutoSwitchEnabled}, HSpacer{}}},
		Label{Text: a.t("Порог 5ч окна"), TextColor: textColor}, ComboBox{AssignTo: &five, Background: SolidColorBrush{Color: cardColor}, Model: []string{"1%", "5%", "10%"}, CurrentIndex: thresholdIndex},
		Label{Text: a.t("Порог недельного окна"), TextColor: textColor}, ComboBox{AssignTo: &week, Background: SolidColorBrush{Color: cardColor}, Model: []string{"1%", "5%", "10%"}, CurrentIndex: thresholdWeeklyIndex},
		Label{Text: a.t("Уведомления Windows"), TextColor: textColor}, ComboBox{AssignTo: &mode, Background: SolidColorBrush{Color: cardColor}, Model: []string{a.t("Все события"), a.t("Только смена аккаунта"), a.t("Выключены")}, CurrentIndex: mi},
		Label{Text: a.t("Обновления"), TextColor: textColor},
		Composite{Layout: HBox{MarginsZero: true, Spacing: 8}, Children: []Widget{
			Label{Text: a.t("Текущая версия: ") + updater.CurrentVersion, TextColor: mutedColor},
			HSpacer{},
			PushButton{AssignTo: &checkBtn, MinSize: Size{150, 28}, Text: a.t("Проверить обновление"), OnClicked: func() {
				checkBtn.SetEnabled(false)
				_ = statusLbl.SetText(a.t("Проверка..."))
				go func() {
					info, checkErr := updater.CheckForUpdate(context.Background())
					dlg.Synchronize(func() {
						checkBtn.SetEnabled(true)
						if checkErr != nil {
							_ = statusLbl.SetText(a.t("Ошибка проверки: ") + checkErr.Error())
							return
						}
						if info.HasUpdate {
							_ = statusLbl.SetText(a.t("Доступно обновление: ") + info.LatestVersion)
							latestURL = info.ReleaseURL
							dlBtn.SetVisible(true)
						} else {
							_ = statusLbl.SetText(a.t("У вас установлена последняя версия"))
							dlBtn.SetVisible(false)
						}
					})
				}()
			}},
			PushButton{AssignTo: &dlBtn, MinSize: Size{90, 28}, Text: a.t("Скачать"), Visible: false, OnClicked: func() {
				if latestURL != "" {
					_ = windows.ShellExecute(0, nil, windows.StringToUTF16Ptr(latestURL), nil, nil, windows.SW_SHOWNORMAL)
				}
			}},
		}},
		Label{AssignTo: &statusLbl, TextColor: mutedColor, EllipsisMode: EllipsisEnd},
		Composite{Layout: HBox{}, Children: []Widget{HSpacer{}, PushButton{AssignTo: &cancel, MinSize: Size{100, 34}, Text: a.t("Отмена"), OnClicked: func() { dlg.Cancel() }}, PushButton{AssignTo: &apply, MinSize: Size{120, 34}, Text: a.t("Применить"), OnClicked: func() {
			if five.CurrentIndex() < 0 || week.CurrentIndex() < 0 || mode.CurrentIndex() < 0 || language.CurrentIndex() < 0 {
				return
			}
			cfg.Language = languageOptions[language.CurrentIndex()]
			cfg.AutoSwitchEnabled = rotate.Checked()
			cfg.Threshold5H = thresholds[five.CurrentIndex()]
			cfg.ThresholdWeekly = thresholds[week.CurrentIndex()]
			cfg.NotifyMode = modes[mode.CurrentIndex()]
			cfg.NotifyOnSwitch = cfg.NotifyMode != "none"
			if start.Checked() != auto {
				if e := autostart.SetEnabled(start.Checked()); e != nil {
					walk.MsgBox(dlg, "Gemini Switcher", e.Error(), walk.MsgBoxIconError)
					return
				}
			}
			if e := a.st.SaveConfig(cfg); e != nil {
				walk.MsgBox(dlg, "Gemini Switcher", e.Error(), walk.MsgBoxIconError)
				return
			}
			dlg.Accept()
		}}}},
	}}).Create(a.window)
	if err == nil {
		styleButtons(apply, cancel, apply, checkBtn, dlBtn)
		styleDarkControls(dlg, language, start, rotate, five, week, mode)
		dlg.Run()
	}
	if dlg != nil {
		defer dlg.Dispose()
		if dlg.Result() == walk.DlgCmdOK {
			a.applyLanguage(cfg.Language)
		}
	}
	if err != nil {
		log.Printf("settings: %v", err)
		walk.MsgBox(a.window, "Gemini Switcher", err.Error(), walk.MsgBoxIconError)
	}
}
