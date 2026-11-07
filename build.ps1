$ErrorActionPreference = 'Stop'
Push-Location $PSScriptRoot
try {
    if ((go env GOOS) -ne 'windows') { throw 'Build on Windows.' }
    if ((go env GOARCH) -ne 'amd64') { throw 'This build targets Windows x64 (amd64).' }
    go run github.com/akavel/rsrc@v0.10.2 -manifest app.manifest -ico internal/icons/green.ico -arch amd64 -o rsrc_windows_amd64.syso
    if ($LASTEXITCODE -ne 0) { throw 'Windows resource generation failed.' }
    go build -trimpath -ldflags '-s -w -H windowsgui' -o gemini-switcher.exe .
    if ($LASTEXITCODE -ne 0) { throw 'Build failed. Close the running application using Exit in the tray.' }
    if (!(Test-Path -LiteralPath .\gemini-switcher.exe)) { throw 'Build output is missing.' }
} finally {
    Pop-Location
}
