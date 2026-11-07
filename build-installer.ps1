param(
    [string]$OutFile = ""
)

$ErrorActionPreference = 'Stop'
Push-Location $PSScriptRoot
try {
    if ((go env GOOS) -ne 'windows') { throw 'Build on Windows.' }
    if ((go env GOARCH) -ne 'amd64') { throw 'This build targets Windows x64 (amd64).' }

    go build -trimpath -ldflags '-s -w -H windowsgui' -o gemini-switcher.exe .
    if ($LASTEXITCODE -ne 0) { throw 'Main application build failed.' }

    $inputBytes = [System.IO.File]::ReadAllBytes((Resolve-Path "gemini-switcher.exe"))
    $ms = New-Object System.IO.MemoryStream
    $gz = New-Object System.IO.Compression.GZipStream($ms, [System.IO.Compression.CompressionLevel]::Optimal)
    $gz.Write($inputBytes, 0, $inputBytes.Length)
    $gz.Close()
    [System.IO.File]::WriteAllBytes((Resolve-Path "cmd\installer\payload.dat"), $ms.ToArray())

    go run github.com/akavel/rsrc@v0.10.2 -manifest app.manifest -ico internal/icons/green.ico -arch amd64 -o cmd/installer/rsrc_windows_amd64.syso
    if ($LASTEXITCODE -ne 0) { throw 'Installer resource generation failed.' }

    if (-not $OutFile) {
        $desktop = [Environment]::GetFolderPath('Desktop')
        $outputPath = Join-Path $desktop 'Gemini-Switcher-Setup.exe'
    } else {
        $outputPath = $OutFile
    }

    go build -trimpath -ldflags '-s -w -H windowsgui' -o $outputPath .\cmd\installer
    if ($LASTEXITCODE -ne 0) { throw 'Installer build failed.' }
    if (!(Test-Path -LiteralPath $outputPath)) { throw 'Installer output is missing.' }

    Write-Output "Installer created successfully: $outputPath"
} finally {
    Set-Content -Path cmd\installer\payload.dat -Value "" -NoNewline -ErrorAction SilentlyContinue
    Remove-Item cmd\installer\rsrc_windows_amd64.syso -ErrorAction SilentlyContinue
    Pop-Location
}
