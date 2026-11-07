package main

import (
	"bytes"
	"compress/gzip"
	_ "embed"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"github.com/lxn/walk"
	. "github.com/lxn/walk/declarative"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

//go:embed payload.dat
var payloadData []byte

var pendingCleanupDir string

func getDefaultInstallDir() string {
	localApp, err := windows.KnownFolderPath(windows.FOLDERID_LocalAppData, 0)
	if err != nil {
		home, _ := os.UserHomeDir()
		localApp = filepath.Join(home, "AppData", "Local")
	}
	return filepath.Join(localApp, "Programs", "Gemini Switcher")
}

func extractPayload(dst string) error {
	if len(payloadData) == 0 {
		return fmt.Errorf("embedded payload is empty")
	}
	if len(payloadData) > 2 && payloadData[0] == 0x1f && payloadData[1] == 0x8b {
		gz, err := gzip.NewReader(bytes.NewReader(payloadData))
		if err != nil {
			return err
		}
		defer gz.Close()

		f, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755)
		if err != nil {
			return err
		}
		defer f.Close()

		_, err = io.Copy(f, gz)
		return err
	}
	return os.WriteFile(dst, payloadData, 0755)
}

func killRunningApp() {
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err == nil {
		defer windows.CloseHandle(snapshot)
		var entry windows.ProcessEntry32
		entry.Size = uint32(unsafe.Sizeof(entry))
		for err = windows.Process32First(snapshot, &entry); err == nil; err = windows.Process32Next(snapshot, &entry) {
			if strings.EqualFold(windows.UTF16ToString(entry.ExeFile[:]), "gemini-switcher.exe") {
				hProc, openErr := windows.OpenProcess(windows.PROCESS_TERMINATE, false, entry.ProcessID)
				if openErr == nil {
					_ = windows.TerminateProcess(hProc, 1)
					_ = windows.CloseHandle(hProc)
				}
			}
		}
	}
	time.Sleep(200 * time.Millisecond)
}

func getInstalledDir() string {
	k, err := registry.OpenKey(registry.CURRENT_USER, `Software\Microsoft\Windows\CurrentVersion\Uninstall\GeminiSwitcher`, registry.QUERY_VALUE)
	if err == nil {
		defer k.Close()
		if loc, _, err := k.GetStringValue("InstallLocation"); err == nil && loc != "" {
			return filepath.Clean(loc)
		}
	}
	selfExe, _ := os.Executable()
	dir := filepath.Dir(selfExe)
	if strings.EqualFold(filepath.Base(dir), "Gemini Switcher") {
		return filepath.Clean(dir)
	}
	return filepath.Clean(getDefaultInstallDir())
}

func doInstall(destDir string, desktopShortcut, startMenuShortcut, autostartLaunch, runAfter bool, progressCb func(int, string)) error {
	progressCb(10, "Завершение работающих процессов...")
	killRunningApp()

	progressCb(25, "Создание папки установки...")
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return err
	}

	progressCb(45, "Копирование исполняемых файлов...")
	targetExe := filepath.Join(destDir, "gemini-switcher.exe")
	var lastErr error
	for i := 0; i < 5; i++ {
		lastErr = extractPayload(targetExe)
		if lastErr == nil {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	if lastErr != nil {
		return lastErr
	}

	selfExe, err := os.Executable()
	if err == nil {
		selfBytes, err := os.ReadFile(selfExe)
		if err == nil {
			_ = os.WriteFile(filepath.Join(destDir, "uninstall.exe"), selfBytes, 0755)
		}
	}

	progressCb(65, "Создание ярлыков...")
	if desktopShortcut {
		desktopDir, err := windows.KnownFolderPath(windows.FOLDERID_Desktop, 0)
		if err == nil {
			_ = createShortcut(filepath.Join(desktopDir, "Gemini Switcher.lnk"), targetExe, targetExe, destDir)
		}
	}

	if startMenuShortcut {
		programsDir, err := windows.KnownFolderPath(windows.FOLDERID_Programs, 0)
		if err == nil {
			_ = createShortcut(filepath.Join(programsDir, "Gemini Switcher.lnk"), targetExe, targetExe, destDir)
		}
	}

	progressCb(80, "Регистрация приложения в Windows...")
	if autostartLaunch {
		runKey, _, err := registry.CreateKey(registry.CURRENT_USER, `Software\Microsoft\Windows\CurrentVersion\Run`, registry.SET_VALUE)
		if err == nil {
			_ = runKey.SetStringValue("GeminiSwitcher", `"`+targetExe+`" --autostart`)
			_ = runKey.Close()
		}
	}

	appPathsKey, _, err := registry.CreateKey(registry.CURRENT_USER, `Software\Microsoft\Windows\CurrentVersion\App Paths\gemini-switcher.exe`, registry.SET_VALUE)
	if err == nil {
		_ = appPathsKey.SetStringValue("", targetExe)
		_ = appPathsKey.SetStringValue("Path", destDir)
		_ = appPathsKey.Close()
	}

	unKey, _, err := registry.CreateKey(registry.CURRENT_USER, `Software\Microsoft\Windows\CurrentVersion\Uninstall\GeminiSwitcher`, registry.SET_VALUE)
	if err == nil {
		unExe := filepath.Join(destDir, "uninstall.exe")
		_ = unKey.SetStringValue("DisplayName", "Gemini Switcher")
		_ = unKey.SetStringValue("DisplayVersion", "1.0.0")
		_ = unKey.SetStringValue("Publisher", "Gemini Switcher")
		_ = unKey.SetStringValue("DisplayIcon", targetExe+",0")
		_ = unKey.SetStringValue("InstallLocation", destDir)
		_ = unKey.SetStringValue("UninstallString", `"`+unExe+`"`)
		_ = unKey.SetStringValue("QuietUninstallString", `"`+unExe+`" --silent`)
		_ = unKey.SetDWordValue("EstimatedSize", uint32(len(payloadData)/1024))
		_ = unKey.SetDWordValue("NoModify", 1)
		_ = unKey.SetDWordValue("NoRepair", 1)
		_ = unKey.Close()
	}

	progressCb(100, "Установка успешно завершена!")

	if runAfter {
		cmd := exec.Command(targetExe)
		_ = cmd.Start()
	}

	return nil
}

func doUninstall(removeUserData bool, progressCb func(int, string)) error {
	progressCb(15, "Завершение работающих процессов...")
	killRunningApp()

	progressCb(35, "Удаление ярлыков...")
	desktopDir, err := windows.KnownFolderPath(windows.FOLDERID_Desktop, 0)
	if err == nil {
		_ = os.Remove(filepath.Join(desktopDir, "Gemini Switcher.lnk"))
	}
	programsDir, err := windows.KnownFolderPath(windows.FOLDERID_Programs, 0)
	if err == nil {
		_ = os.Remove(filepath.Join(programsDir, "Gemini Switcher.lnk"))
	}

	progressCb(55, "Удаление записей реестра...")
	runKey, err := registry.OpenKey(registry.CURRENT_USER, `Software\Microsoft\Windows\CurrentVersion\Run`, registry.SET_VALUE)
	if err == nil {
		_ = runKey.DeleteValue("GeminiSwitcher")
		_ = runKey.Close()
	}
	_ = registry.DeleteKey(registry.CURRENT_USER, `Software\Microsoft\Windows\CurrentVersion\Uninstall\GeminiSwitcher`)
	_ = registry.DeleteKey(registry.CURRENT_USER, `Software\Microsoft\Windows\CurrentVersion\App Paths\gemini-switcher.exe`)

	progressCb(75, "Удаление файлов приложения...")
	destDir := getInstalledDir()
	selfExe, _ := os.Executable()

	if strings.EqualFold(filepath.Base(destDir), "Gemini Switcher") {
		_ = os.Remove(filepath.Join(destDir, "gemini-switcher.exe"))
		if strings.EqualFold(filepath.Dir(selfExe), destDir) {
			pendingCleanupDir = destDir
		} else {
			_ = os.RemoveAll(destDir)
		}
	}

	if removeUserData {
		home, err := os.UserHomeDir()
		if err == nil {
			_ = os.RemoveAll(filepath.Join(home, ".gemini-switcher"))
		}
	}

	progressCb(100, "Gemini Switcher успешно удален!")
	return nil
}

func runInstallerUI(defaultDir string) {
	var mw *walk.MainWindow
	var pathEdit *walk.LineEdit
	var browseBtn, btnAction, btnCancel *walk.PushButton
	var cbDesktop, cbStartMenu, cbAutostart, cbLaunch *walk.CheckBox
	var progressBar *walk.ProgressBar
	var statusLabel *walk.Label
	var installing bool

	_ = MainWindow{
		AssignTo: &mw,
		Title:    "Установка Gemini Switcher",
		Size:     Size{Width: 520, Height: 440},
		MinSize:  Size{Width: 520, Height: 440},
		MaxSize:  Size{Width: 520, Height: 440},
		Layout:   VBox{Margins: Margins{Left: 22, Top: 18, Right: 22, Bottom: 18}, Spacing: 10},
		Children: []Widget{
			Composite{
				Layout: VBox{MarginsZero: true, Spacing: 3},
				Children: []Widget{
					Label{
						Text: "Установка Gemini Switcher",
						Font: Font{Family: "Segoe UI", PointSize: 13, Bold: true},
					},
					Label{
						Text: "Менеджер аккаунтов и квот Antigravity для Windows",
						Font: Font{Family: "Segoe UI", PointSize: 9},
					},
				},
			},
			VSpacer{Size: 4},
			Label{
				Text: "Папка установки:",
				Font: Font{Family: "Segoe UI", PointSize: 9, Bold: true},
			},
			Composite{
				Layout: HBox{MarginsZero: true, Spacing: 8},
				Children: []Widget{
					LineEdit{
						AssignTo: &pathEdit,
						Text:     defaultDir,
					},
					PushButton{
						AssignTo: &browseBtn,
						Text:     "Обзор...",
						OnClicked: func() {
							dlg := new(walk.FileDialog)
							dlg.Title = "Выберите папку для установки"
							dlg.FilePath = pathEdit.Text()
							if ok, _ := dlg.ShowBrowseFolder(mw); ok {
								pathEdit.SetText(dlg.FilePath)
							}
						},
					},
				},
			},
			VSpacer{Size: 4},
			Label{
				Text: "Параметры установки:",
				Font: Font{Family: "Segoe UI", PointSize: 9, Bold: true},
			},
			CheckBox{
				AssignTo: &cbDesktop,
				Text:     "Создать ярлык на рабочем столе",
				Checked:  true,
			},
			CheckBox{
				AssignTo: &cbStartMenu,
				Text:     "Создать ярлык в меню «Пуск»",
				Checked:  true,
			},
			CheckBox{
				AssignTo: &cbAutostart,
				Text:     "Запускать автоматически при входе в Windows",
				Checked:  true,
			},
			CheckBox{
				AssignTo: &cbLaunch,
				Text:     "Запустить Gemini Switcher после установки",
				Checked:  true,
			},
			VSpacer{Size: 4},
			ProgressBar{
				AssignTo: &progressBar,
				MinValue: 0,
				MaxValue: 100,
				Value:    0,
				Visible:  false,
			},
			Label{
				AssignTo:     &statusLabel,
				Text:         "Нажмите «Установить» для продолжения.",
				EllipsisMode: EllipsisEnd,
			},
			VSpacer{Size: 6},
			Composite{
				Layout: HBox{MarginsZero: true, Spacing: 10},
				Children: []Widget{
					HSpacer{},
					PushButton{
						AssignTo: &btnAction,
						Text:     "Установить",
						MinSize:  Size{Width: 120, Height: 32},
						OnClicked: func() {
							if btnAction.Text() == "Готово" {
								mw.Close()
								return
							}
							if installing {
								return
							}
							installing = true
							btnAction.SetEnabled(false)
							btnCancel.SetEnabled(false)
							pathEdit.SetEnabled(false)
							browseBtn.SetEnabled(false)
							cbDesktop.SetEnabled(false)
							cbStartMenu.SetEnabled(false)
							cbAutostart.SetEnabled(false)
							cbLaunch.SetEnabled(false)
							progressBar.SetVisible(true)

							dest := strings.TrimSpace(pathEdit.Text())
							desk := cbDesktop.Checked()
							start := cbStartMenu.Checked()
							auto := cbAutostart.Checked()
							launch := cbLaunch.Checked()

							go func() {
								err := doInstall(dest, desk, start, auto, launch, func(pct int, msg string) {
									mw.Synchronize(func() {
										progressBar.SetValue(pct)
										statusLabel.SetText(msg)
									})
								})
								mw.Synchronize(func() {
									installing = false
									if err != nil {
										statusLabel.SetText("Ошибка установки: " + err.Error())
										walk.MsgBox(mw, "Ошибка", err.Error(), walk.MsgBoxIconError)
										btnAction.SetText("Повторить")
										btnAction.SetEnabled(true)
										btnCancel.SetEnabled(true)
										return
									}
									statusLabel.SetText("Установка успешно завершена!")
									btnAction.SetText("Готово")
									btnAction.SetEnabled(true)
									btnCancel.SetVisible(false)
								})
							}()
						},
					},
					PushButton{
						AssignTo: &btnCancel,
						Text:     "Отмена",
						MinSize:  Size{Width: 90, Height: 32},
						OnClicked: func() {
							mw.Close()
						},
					},
				},
			},
		},
	}.Create()

	if mw != nil {
		mw.Run()
	}
}

func runUninstallerUI() {
	var mw *walk.MainWindow
	var btnUninstall, btnCancel *walk.PushButton
	var cbRemoveData *walk.CheckBox
	var progressBar *walk.ProgressBar
	var statusLabel *walk.Label
	var running bool

	_ = MainWindow{
		AssignTo: &mw,
		Title:    "Удаление Gemini Switcher",
		Size:     Size{Width: 480, Height: 300},
		MinSize:  Size{Width: 480, Height: 300},
		MaxSize:  Size{Width: 480, Height: 300},
		Layout:   VBox{Margins: Margins{Left: 22, Top: 18, Right: 22, Bottom: 18}, Spacing: 12},
		Children: []Widget{
			Composite{
				Layout: VBox{MarginsZero: true, Spacing: 3},
				Children: []Widget{
					Label{
						Text: "Удаление Gemini Switcher",
						Font: Font{Family: "Segoe UI", PointSize: 13, Bold: true},
					},
					Label{
						Text: "Вы действительно хотите удалить Gemini Switcher с компьютера?",
						Font: Font{Family: "Segoe UI", PointSize: 9},
					},
				},
			},
			VSpacer{Size: 4},
			CheckBox{
				AssignTo: &cbRemoveData,
				Text:     "Удалить сохраненные аккаунты и настройки (%USERPROFILE%\\.gemini-switcher)",
				Checked:  false,
			},
			VSpacer{Size: 4},
			ProgressBar{
				AssignTo: &progressBar,
				MinValue: 0,
				MaxValue: 100,
				Value:    0,
				Visible:  false,
			},
			Label{
				AssignTo:     &statusLabel,
				Text:         "Нажмите «Удалить» для начала процесса.",
				EllipsisMode: EllipsisEnd,
			},
			VSpacer{Size: 6},
			Composite{
				Layout: HBox{MarginsZero: true, Spacing: 10},
				Children: []Widget{
					HSpacer{},
					PushButton{
						AssignTo: &btnUninstall,
						Text:     "Удалить",
						MinSize:  Size{Width: 110, Height: 32},
						OnClicked: func() {
							if btnUninstall.Text() == "Закрыть" {
								mw.Close()
								return
							}
							if running {
								return
							}
							running = true
							btnUninstall.SetEnabled(false)
							btnCancel.SetEnabled(false)
							cbRemoveData.SetEnabled(false)
							progressBar.SetVisible(true)

							removeData := cbRemoveData.Checked()

							go func() {
								err := doUninstall(removeData, func(pct int, msg string) {
									mw.Synchronize(func() {
										progressBar.SetValue(pct)
										statusLabel.SetText(msg)
									})
								})
								mw.Synchronize(func() {
									running = false
									if err != nil {
										statusLabel.SetText("Ошибка удаления: " + err.Error())
										walk.MsgBox(mw, "Ошибка", err.Error(), walk.MsgBoxIconError)
										btnUninstall.SetText("Повторить")
										btnUninstall.SetEnabled(true)
										btnCancel.SetEnabled(true)
										return
									}
									statusLabel.SetText("Gemini Switcher успешно удален!")
									btnUninstall.SetText("Закрыть")
									btnUninstall.SetEnabled(true)
									btnCancel.SetVisible(false)
								})
							}()
						},
					},
					PushButton{
						AssignTo: &btnCancel,
						Text:     "Отмена",
						MinSize:  Size{Width: 90, Height: 32},
						OnClicked: func() {
							mw.Close()
						},
					},
				},
			},
		},
	}.Create()

	if mw != nil {
		mw.Run()
	}
}

func main() {
	defer func() {
		if pendingCleanupDir != "" && strings.EqualFold(filepath.Base(pendingCleanupDir), "Gemini Switcher") {
			cleanDir := filepath.Clean(pendingCleanupDir)
			cmd := exec.Command("cmd.exe", "/c", "ping -n 2 127.0.0.1 >nul & rd /s /q \""+cleanDir+"\"")
			cmd.Dir = os.TempDir()
			cmd.SysProcAttr = &syscall.SysProcAttr{
				HideWindow:    true,
				CreationFlags: 0x08000000,
			}
			_ = cmd.Start()
		}
	}()

	defaultDir := getDefaultInstallDir()
	isUninstall := false
	silentMode := false

	baseName := strings.ToLower(filepath.Base(os.Args[0]))
	if strings.Contains(baseName, "uninstall") {
		isUninstall = true
	}

	for _, arg := range os.Args[1:] {
		lower := strings.ToLower(arg)
		if lower == "--uninstall" || lower == "-uninstall" || lower == "/uninstall" {
			isUninstall = true
		} else if lower == "--silent" || lower == "-silent" || lower == "/silent" || lower == "/s" || lower == "-s" {
			silentMode = true
		} else if strings.HasPrefix(lower, "--dir=") {
			defaultDir = arg[6:]
		} else if strings.HasPrefix(lower, "-dir=") {
			defaultDir = arg[5:]
		}
	}

	if silentMode {
		if isUninstall {
			_ = doUninstall(false, func(int, string) {})
		} else {
			_ = doInstall(defaultDir, true, true, true, false, func(int, string) {})
		}
		return
	}

	if isUninstall {
		runUninstallerUI()
		return
	}

	runInstallerUI(defaultDir)
}
