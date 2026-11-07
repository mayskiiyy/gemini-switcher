[🇷🇺 Читать на русском языке](README.ru.md)

# Gemini Switcher

[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![Platform: Windows 10 / 11](https://img.shields.io/badge/Platform-Windows%2010%20%7C%2011-0078D6.svg?logo=windows)](https://microsoft.com/windows)
[![Go Version](https://img.shields.io/badge/Go-1.22+-00ADD8.svg?logo=go)](https://golang.org)
[![Architecture: x64](https://img.shields.io/badge/Arch-x64-informational.svg)]()
[![Release](https://img.shields.io/github/v/release/mayskiiyy/gemini-switcher?color=success&include_prereleases)](https://github.com/mayskiiyy/gemini-switcher/releases)

A lightweight, silent, and seamless multi-account session manager and quota monitor for **Google Antigravity** on Windows.

---

## Overview

When developing and agentic pair-programming with Google Antigravity, developers frequently encounter rate limits on the 5-hour rolling prompt window or weekly usage allowances.

**Gemini Switcher** runs quietly in your Windows system tray, continuously tracking remaining quota across both 5-hour and weekly windows. When your current account drops below a predefined threshold (e.g. 5%), Gemini Switcher automatically and seamlessly rotates your active session to another account from your local pool — without interrupting your workflow, flashing console windows, or requiring manual token exports.

---

## Key Features

- **Multi-Account Session Pool**: Capture signed-in Antigravity sessions with a single click. Keep an arbitrary number of accounts in your pool, assign friendly aliases (`Personal`, `Work`, `Dev`), and set priorities or pause individual accounts.
- **Continuous Quota Monitoring**: Automatically polls remaining capacity and calculates reset countdowns for both the **5-hour rolling window** and the **weekly allowance**.
- **Intelligent Auto-Rotation**: When your active account nears quota exhaustion, the switcher automatically rotates to the next best eligible account based on priority, available quota, and cooldown status.
- **100% Silent Background Operation**: Quota inquiries run in an isolated background sandbox with `CREATE_NO_WINDOW` flags and stripped environments. Zero flashing terminal prompts or popup command windows.
- **Dynamic System Tray Status**:
  - 🟢 **Green**: Active account quota is healthy (> 20%) and auto-switch is enabled.
  - 🟡 **Yellow**: Warning state (quota <= 20% or unmanaged session).
  - 🔴 **Red**: Critical state (quota exhausted <= 5%).
  - ⚪ **Gray**: Auto-switch is paused or disabled.
- **Native Minimalist Dark Control Panel**: Built in pure Go with native Windows GDI/Win32 controls (via Walk) — clean dark mode styling, zero Electron/Chromium overhead, and minimal RAM footprint (< 25 MB).
- **Zero Cloud Relay / Absolute Privacy**: All credentials are stored strictly on your local machine (`%USERPROFILE%\.gemini-switcher\`), interfacing directly with the official Windows Credential Manager (`gemini:antigravity`). No telemetry, no external servers, no cloud relays.
- **Seamless Language Server Signaling**: Automatically signals Antigravity's internal language server (`language_server.exe`) to restart upon credential swap, ensuring new sessions are picked up immediately without restarting your editor.
- **Multilingual Support**: Fully localized in English, Russian, German, Polish, and Ukrainian with automatic Windows display language detection.
- **Setup Wizard & Portable Run**: Distribute as a portable standalone `.exe` or install via the custom `Gemini-Switcher-Setup.exe` installer with autostart and Start Menu integration.

---

## Architecture & How It Works

```
┌────────────────────────────────────────────────────────┐
│                   Gemini Switcher                      │
│   (System Tray + Native Win32 Control Panel / Walk)    │
└───────────────┬────────────────────────┬───────────────┘
                │                        │
       1. Quota Polling          2. Session Swapping
                │                        │
                ▼                        ▼
┌───────────────────────────────┐ ┌───────────────────────────────┐
│       Isolated Sandbox        │ │   Windows Credential Manager  │
│  agy -p /quota --output-format│ │     Target: gemini:antigravity│
│  CREATE_NO_WINDOW (Silent)    │ │              +                │
│  Stripped PATH & Clean Env    │ │  Antigravity app_storage.json │
└───────────────────────────────┘ └──────────────┬────────────────┘
                                                 │
                                         3. Process Refresh
                                                 │
                                                 ▼
                                  ┌───────────────────────────────┐
                                  │   Restart language_server.exe │
                                  │   Instant token pickup in IDE │
                                  └───────────────────────────────┘
```

1. **Credential Interception**: Interacts with `advapi32.dll` (`CredReadW` / `CredWriteW`) under the credential target `gemini:antigravity`. Also synchronizes the active username in `%APPDATA%\Antigravity\app_storage.json`.
2. **Quota Acquisition**: Queries `agy.exe --output-format json -p /quota` using an isolated temporary profile with auto-update disabled (`AGY_CLI_DISABLE_AUTO_UPDATE=true`) to prevent lock contention and unwanted updates.
3. **Smart Candidate Selection**: Accounts in cooldown (remaining quota below threshold and reset time still in the future) are skipped. Priority accounts are favored; ties are resolved by largest remaining quota and oldest switch time.
4. **Single-Instance Enforcement**: Uses Windows named mutex `Local\GeminiSwitcherSingleInstanceMutex` and named event `Local\GeminiSwitcherOpenEvent`. Launching the executable again simply brings the existing control panel to the foreground.

---

## Installation

### Method 1: Windows Setup Installer (Recommended)

1. Download `Gemini-Switcher-Setup.exe` from the latest [GitHub Releases](https://github.com/mayskiiyy/gemini-switcher/releases).
2. Run the installer:
   - Installs to `%LOCALAPPDATA%\Programs\Gemini Switcher`.
   - Creates Start Menu and Desktop shortcuts.
   - Configures automatic startup on Windows boot (starts minimized to tray).
   - Integrates cleanly into Windows **Installed Apps / Settings** with a dedicated uninstaller (`uninstall.exe`).

### Method 2: Portable Executable

1. Download `gemini-switcher.exe` from [GitHub Releases](https://github.com/mayskiiyy/gemini-switcher/releases).
2. Place it in any directory of your choice (e.g. `C:\Tools\GeminiSwitcher\`).
3. Run `gemini-switcher.exe`. All configuration and account data will be stored in `%USERPROFILE%\.gemini-switcher\`.

---

## Quick Start Guide

### Step 1: Launch the Application
Run `gemini-switcher.exe`. The Control Panel window will open, and a status icon will appear in your system tray.

### Step 2: Capture Your First Account
1. Open Google Antigravity and sign into your first Google account (e.g. `personal@example.com`).
2. In the Gemini Switcher Control Panel, click **Capture session** (or right-click the tray icon and select **Capture session**).
3. The account is added to your pool with its current 5-hour and weekly quota.
4. *(Optional)* Select the account in the list, click **Account name**, and assign an alias like `Personal`.

### Step 3: Add Additional Accounts to the Pool
1. In Google Antigravity, sign out and sign in with your second account (e.g. `work@example.com`).
2. In Gemini Switcher, click **Capture session**.
3. Repeat the process for any additional accounts (e.g. `developer@example.com`).

### Step 4: Work Uninterrupted
You can now close the Control Panel window. Gemini Switcher will stay in the tray, polling your quota every 60 seconds. When your active session runs low on quota, it rotates to the next available account automatically!

---

## Switching Modes

### 1. Automatic Rotation (Hands-Free)
- Enabled by default (**Settings** -> **Automatic switching**).
- Polls quota at the configured interval (`check_interval_seconds: 60`).
- When the active account's 5-hour or weekly remaining quota drops to or below the threshold (`threshold_5h: 0.05` / `threshold_weekly: 0.05`):
  1. Identifies all ready, non-paused accounts.
  2. Selects the healthiest account (highest quota, respecting priority).
  3. Writes new credentials to Windows Credential Manager.
  4. Restarts `language_server.exe` for immediate pickup.
  5. Sends a Windows balloon/toast notification.

### 2. Manual Switching
- **From the Control Panel**: Select any account in the list and click **Switch**.
- **From the System Tray**: Right-click the tray icon and click directly on the desired account name in the menu:
  - `✓ work@example.com [5h: 92%, Wk: 88%]` (Active)
  - `○ personal@example.com [5h: 15%, Wk: 70%]` (Click to activate)
  - `○ developer@example.com [5h: reset in 14:30 (~42m)]` (Cooldown)

---

## Configuration

Settings can be changed from the **Settings** dialog in the Control Panel or by editing `%USERPROFILE%\.gemini-switcher\config.json`:

```json
{
  "language": "auto",
  "auto_switch_enabled": true,
  "check_interval_seconds": 60,
  "threshold_5h": 0.05,
  "threshold_weekly": 0.05,
  "notify_on_switch": true,
  "notify_mode": "all"
}
```

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `language` | string | `"auto"` | UI language: `"auto"` (Windows system language), `"en"`, `"ru"`, `"de"`, `"pl"`, `"uk"`. |
| `auto_switch_enabled` | bool | `true` | Enables automatic account rotation when quota limits are reached. |
| `check_interval_seconds` | int | `60` | Background polling interval in seconds. |
| `threshold_5h` | float | `0.05` | Trigger rotation when 5-hour quota drops to `0.01` (1%), `0.05` (5%), or `0.10` (10%). |
| `threshold_weekly` | float | `0.05` | Trigger rotation when weekly quota drops to `0.01` (1%), `0.05` (5%), or `0.10` (10%). |
| `notify_on_switch` | bool | `true` | Show Windows notifications. |
| `notify_mode` | string | `"all"` | Notification verbosity: `"all"` (all events), `"switch_only"` (only swaps/rotations), `"none"` (silent). |

---

## Local Data & Storage

All data is stored locally in `%USERPROFILE%\.gemini-switcher\`:

- `accounts.json`: Local account pool, quota buckets, reset timestamps, aliases, and encrypted session blobs.
- `accounts.json.bak`: Automatic backup generated prior to pool modifications.
- `config.json`: User configuration options and preferences.

> [!WARNING]
> `accounts.json` contains authentication session data for your Google accounts. Do not publish, commit, or share this file publicly. Keep the `.gemini-switcher` folder excluded from public cloud sync drives and public repositories. See [SECURITY.md](SECURITY.md) for details.

---

## Building from Source

### Prerequisites
- **Windows 10 / 11 x64**
- **Go 1.22+** ([golang.org](https://go.dev/dl/))
- **PowerShell 5.1+**
- **Antigravity CLI (`agy`)** installed and available on `PATH`

### 1. Compile Standalone Application
Run the build script from PowerShell:

```powershell
.\build.ps1
```

The script:
1. Downloads the pinned Windows resource compiler (`github.com/akavel/rsrc`).
2. Compiles Windows resource manifests and tray icons into `rsrc_windows_amd64.syso`.
3. Builds the optimized executable with stripped debug symbols and hidden console flags:
   ```powershell
   go build -trimpath -ldflags '-s -w -H windowsgui' -o gemini-switcher.exe .
   ```

### 2. Compile Setup Installer
To create the full Windows setup wizard:

```powershell
.\build-installer.ps1
```

This compiles the main application, compresses it with Gzip into `cmd/installer/payload.dat`, embeds the Windows manifest, and outputs `Gemini-Switcher-Setup.exe` to your Desktop.

### 3. Run Unit Tests
```powershell
go test ./...
```

---

## Troubleshooting & FAQ

### Quota refresh fails with `agy execution failed`
- Make sure `agy.exe` is installed and reachable either on your system `PATH` or at default locations (`%LOCALAPPDATA%\agy\bin\agy.exe` or `%LOCALAPPDATA%\Programs\Antigravity\bin\agy.exe`).
- Test whether `agy --output-format json -p /quota` works manually in PowerShell.

### Capturing session fails with `CredReadW failed`
- Ensure that you are currently signed into Google Antigravity.
- Open Antigravity and verify that an active session exists.

### Antigravity does not recognize the new account immediately
- Gemini Switcher automatically restarts `language_server.exe` upon switching.
- If your IDE does not reflect the change within 5 seconds, reload your Antigravity window (`Ctrl+Shift+P` -> `Developer: Reload Window`).

### How to completely uninstall Gemini Switcher
- If installed via Setup: open Windows **Settings** -> **Apps** -> **Installed apps**, find **Gemini Switcher**, and select **Uninstall** (or run `uninstall.exe` from `%LOCALAPPDATA%\Programs\Gemini Switcher`). The uninstaller prompts whether you wish to keep or delete your local account pool in `%USERPROFILE%\.gemini-switcher`.
- If portable: close the application from the tray and delete the executable and `%USERPROFILE%\.gemini-switcher`.

---

## Contributing

Contributions, bug reports, and suggestions are welcome!

1. Fork the repository: `https://github.com/mayskiiyy/gemini-switcher`
2. Create a feature branch: `git checkout -b feature/my-new-feature`
3. Commit your changes (ensure `go test ./...` passes)
4. Push to your branch and open a Pull Request

---

## Security

Please review [SECURITY.md](SECURITY.md) for information regarding local credential storage, vulnerability reporting, and safe usage guidelines.

---

## Disclaimer

Gemini Switcher is an independent open-source project and is **not** affiliated with, endorsed by, or associated with Google LLC. "Google", "Gemini", and "Antigravity" are trademarks of Google LLC.

---

## License

This project is licensed under the [MIT License](LICENSE). Third-party dependencies retain their respective licenses (see [THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md)).
