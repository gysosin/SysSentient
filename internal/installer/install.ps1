<#
.SYNOPSIS
    SysSentient installer for Windows.

.DESCRIPTION
    Fetches the release binary for this machine, verifies it against the
    published checksums, and optionally enrols with a server.

    iwr -useb https://<server>/install.ps1 | iex; Install-SysSentient -Server <url> -Token <token>

    Registering a service needs an elevated prompt; installing the binary alone
    does not.
#>
[CmdletBinding()]
param(
    [string]$Server = '',
    [string]$Token = '',
    [string]$Version = 'latest',
    [string]$Prefix = "$env:ProgramFiles\SysSentient",
    [switch]$NoService
)

$ErrorActionPreference = 'Stop'
$repo = 'gysosin/SysSentient'

function Say([string]$message) { Write-Host "==> $message" }

# Windows on ARM is a real target; assuming amd64 installs a binary that will
# not run.
$arch = if ($env:PROCESSOR_ARCHITECTURE -eq 'ARM64') { 'arm64' } else { 'amd64' }
Say "detected windows/$arch"

if ($Version -eq 'latest') {
    Say 'resolving the latest release'
    # /releases/latest excludes pre-releases and 404s when every release is
    # one, which is the case for a project still in beta.
    try {
        $Version = (Invoke-RestMethod "https://api.github.com/repos/$repo/releases/latest").tag_name
    } catch {
        Say 'no stable release; using the newest pre-release'
        $Version = (Invoke-RestMethod "https://api.github.com/repos/$repo/releases" |
            Select-Object -First 1).tag_name
    }
    if (-not $Version) { throw 'could not determine a release to install' }
}
Say "installing $Version"

$numeric = $Version -replace '^v', ''
$archive = "sys-sentient_${numeric}_windows_${arch}.zip"
$base = "https://github.com/$repo/releases/download/$Version"

$work = Join-Path ([System.IO.Path]::GetTempPath()) ([System.Guid]::NewGuid().ToString())
New-Item -ItemType Directory -Path $work | Out-Null
try {
    Say "downloading $archive"
    Invoke-WebRequest "$base/$archive" -OutFile (Join-Path $work $archive) -UseBasicParsing

    Say 'verifying the checksum'
    Invoke-WebRequest "$base/checksums.txt" -OutFile (Join-Path $work 'checksums.txt') -UseBasicParsing
    $line = Select-String -Path (Join-Path $work 'checksums.txt') -Pattern ([regex]::Escape($archive)) |
        Select-Object -First 1
    if (-not $line) { throw "no checksum published for $archive" }

    $expected = ($line.Line -split '\s+')[0]
    $actual = (Get-FileHash -Algorithm SHA256 -Path (Join-Path $work $archive)).Hash

    # Refuse rather than warn: this script is meant to be piped into a shell.
    if ($expected -ne $actual) { throw "checksum mismatch for $archive" }
    Say 'checksum verified'

    Expand-Archive -Path (Join-Path $work $archive) -DestinationPath $work -Force
    # v0.1.0-beta.1 shipped the binary as sys-daemon; accept either so the
    # installer works against the release published today and those that follow.
    $binary = Join-Path $work 'sys-sentient.exe'
    if (-not (Test-Path $binary)) { $binary = Join-Path $work 'sys-daemon.exe' }
    if (-not (Test-Path $binary)) { throw 'the archive contained no SysSentient binary' }

    Say "installing to $Prefix"
    New-Item -ItemType Directory -Path $Prefix -Force | Out-Null
    Copy-Item $binary (Join-Path $Prefix 'sys-sentient.exe') -Force

    # On PATH, so the enrolment command works from a fresh prompt.
    $machinePath = [Environment]::GetEnvironmentVariable('Path', 'Machine')
    if ($machinePath -notlike "*$Prefix*") {
        [Environment]::SetEnvironmentVariable('Path', "$machinePath;$Prefix", 'Machine')
        Say "added $Prefix to the system PATH (open a new terminal to pick it up)"
    }

    $exe = Join-Path $Prefix 'sys-sentient.exe'
    if (-not $Server -or -not $Token) {
        Say 'installed. To enrol this machine later:'
        Write-Host "    `"$exe`" agent join --server <url> --token <token> --install-service"
        return
    }

    # Releases before enrolment existed have no `agent join` subcommand, so the
    # arguments fall through to the daemon's flag parsing and it starts a
    # server instead of enrolling.
    if ($Version -eq 'v0.1.0-beta.1') {
        throw "$Version predates agent enrolment and cannot join a server. The binary is installed at $Prefix; install a newer release with -Version <tag>."
    }

    Say "enrolling with $Server"
    $joinArgs = @('agent', 'join', '--server', $Server, '--token', $Token)
    if (-not $NoService) { $joinArgs += '--install-service' }
    & $exe @joinArgs
    if ($LASTEXITCODE -ne 0) { throw "enrolment failed with exit code $LASTEXITCODE" }

    Say 'done'
}
finally {
    Remove-Item -Recurse -Force $work -ErrorAction SilentlyContinue
}
