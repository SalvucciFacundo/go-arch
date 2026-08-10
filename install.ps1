# go-arch installer — single-command install for Windows.
#
# Usage (PowerShell):
#   irm https://raw.githubusercontent.com/SalvucciFacundo/go-arch/main/install.ps1 | iex
#
# Behavior:
#   1. Detect architecture (amd64/arm64).
#   2. Resolve the latest release tag from the GitHub API.
#   3. Download the matching zip and its checksums file.
#   4. Verify the zip SHA-256 against the release checksums.
#   5. Extract to $HOME\.go-arch\bin and add it to the user PATH
#      (current session + persistent User PATH).

$ErrorActionPreference = 'Stop'

$Repo = 'SalvucciFacundo/go-arch'
$BaseUrl = "https://github.com/$Repo/releases/download"

function Write-Step([string]$Message) {
    Write-Host "go-arch: $Message" -ForegroundColor Cyan
}
function Write-ErrorStop([string]$Message) {
    Write-Host "go-arch: $Message" -ForegroundColor Red
    exit 1
}

# --- Detect arch ------------------------------------------------------------

$Arch = $env:PROCESSOR_ARCHITECTURE
if ($Arch -eq 'AMD64') { $GoArch = 'amd64' }
elseif ($Arch -eq 'ARM64') { $GoArch = 'arm64' }
else { Write-ErrorStop "unsupported architecture: $Arch" }

Write-Step "Detected windows/$GoArch"

# --- Resolve latest release -------------------------------------------------

$Headers = @{ 'User-Agent' = 'go-arch-installer' }
$Release = Invoke-RestMethod -Uri "https://api.github.com/repos/$Repo/releases/latest" -Headers $Headers
$Tag = $Release.tag_name
$Version = $Tag.TrimStart('v')

$Zip = "go-arch_${Version}_windows_${GoArch}.zip"
$Checksums = "go-arch_${Version}_checksums.txt"
$ZipUrl = "$BaseUrl/$Tag/$Zip"
$ChecksumsUrl = "$BaseUrl/$Tag/$Checksums"

Write-Step "Latest release: $Tag"

# --- Download into a temp dir -----------------------------------------------

$TmpDir = Join-Path $env:TEMP ("go-arch-install-" + [guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $TmpDir | Out-Null

try {
    $ZipPath = Join-Path $TmpDir $Zip
    $ChecksumsPath = Join-Path $TmpDir $Checksums

    Write-Step "Downloading $Zip ..."
    Invoke-WebRequest -Uri $ZipUrl -OutFile $ZipPath -Headers $Headers
    Invoke-WebRequest -Uri $ChecksumsUrl -OutFile $ChecksumsPath -Headers $Headers

    # --- Verify checksum ----------------------------------------------------

    $ExpectedLine = Get-Content $ChecksumsPath | Where-Object { $_ -match [regex]::Escape("  $Zip") }
    if (-not $ExpectedLine) { Write-ErrorStop "checksum for $Zip not found in $Checksums" }
    $Expected = ($ExpectedLine -split '\s+')[0]

    $Actual = (Get-FileHash -Algorithm SHA256 -Path $ZipPath).Hash.ToLower()

    if ($Actual -ne $Expected) {
        Write-ErrorStop "checksum mismatch for $Zip: expected $Expected, got $Actual"
    }
    Write-Step "Checksum verified"

    # --- Install ------------------------------------------------------------

    $InstallDir = Join-Path $HOME '.go-arch'
    $BinDir = Join-Path $InstallDir 'bin'
    New-Item -ItemType Directory -Path $BinDir -Force | Out-Null

    Expand-Archive -Path $ZipPath -DestinationPath $TmpDir -Force
    Copy-Item -Path (Join-Path $TmpDir 'go-arch.exe') -Destination (Join-Path $BinDir 'go-arch.exe') -Force

    Write-Step "Installed to $BinDir"

    # Add to user PATH if not already present
    $UserPath = [Environment]::GetEnvironmentVariable('Path', 'User')
    if ($UserPath -notlike "*$BinDir*") {
        $NewPath = if ($UserPath) { "$UserPath;$BinDir" } else { $BinDir }
        [Environment]::SetEnvironmentVariable('Path', $NewPath, 'User')
        Write-Step "Added $BinDir to your user PATH (new terminals will see it)"
    }

    # Also update current session PATH
    if ($env:Path -notlike "*$BinDir*") {
        $env:Path = "$env:Path;$BinDir"
    }

    Write-Step "Done. Run 'go-arch version' to verify (in a new terminal)."
}
finally {
    Remove-Item -Recurse -Force $TmpDir -ErrorAction SilentlyContinue
}
