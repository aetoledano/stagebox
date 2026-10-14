<#
  Install the bundled HidHide driver without changing its device/app lists.
  Run in Windows PowerShell as Administrator. Restart Windows after a new install.
  -PassThru returns the installation result to install-stagebox.ps1.
#>
[CmdletBinding()]
param(
  [string]$LauncherDir = "C:\launcher",
  [string]$InstallerPath = "",
  [string]$LogPath = "",
  [switch]$PassThru
)

$ErrorActionPreference = "Stop"
$InstallerName = "HidHide_1.5.230_x64.exe"
$InstallerSHA256 = "F4BBBCB82E6258641B887C74BC81C4C5F66E4AA811808DFC304347687B7605F6"

if ([Environment]::OSVersion.Platform -ne [PlatformID]::Win32NT) {
  throw "Run install-hidhide.ps1 on Windows as Administrator."
}
$identity = [Security.Principal.WindowsIdentity]::GetCurrent()
$principal = New-Object Security.Principal.WindowsPrincipal($identity)
if (-not $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) {
  throw "Run install-hidhide.ps1 from PowerShell as Administrator."
}
$nativeArchitecture = if ($env:PROCESSOR_ARCHITEW6432) { $env:PROCESSOR_ARCHITEW6432 } else { $env:PROCESSOR_ARCHITECTURE }
if (-not [Environment]::Is64BitOperatingSystem -or $nativeArchitecture -ne "AMD64") {
  throw "This bundled HidHide installer requires Intel/AMD 64-bit Windows."
}
if ([string]::IsNullOrWhiteSpace($LogPath)) {
  $LogPath = Join-Path $PSScriptRoot "hidhide-install.log"
}
$LogPath = [IO.Path]::GetFullPath($LogPath)
New-Item -ItemType Directory -Path (Split-Path -Parent $LogPath) -Force | Out-Null

function Write-HidHideLog([string]$Message) {
  $line = "[install-hidhide] $((Get-Date).ToString('yyyy-MM-dd HH:mm:ss.fff')) $Message"
  Write-Host $line
  Add-Content -LiteralPath $LogPath -Value $line -Encoding UTF8
}

function Get-HidHideInstallation {
  $version = ""
  $registered = $false
  $machine = [Microsoft.Win32.RegistryKey]::OpenBaseKey([Microsoft.Win32.RegistryHive]::LocalMachine, [Microsoft.Win32.RegistryView]::Registry64)
  try {
    $key = $machine.OpenSubKey("SOFTWARE\Classes\Installer\Dependencies\NSS.Drivers.HidHide.x64")
    if ($null -ne $key) {
      try { $registered = $true; $version = [string]$key.GetValue("Version", "") }
      finally { $key.Dispose() }
    }
  } finally { $machine.Dispose() }
  $service = Get-Service -Name "HidHide" -ErrorAction SilentlyContinue
  return [pscustomobject]@{ Installed = ($registered -or $null -ne $service); Version = $version; Service = $service }
}

function Invoke-HidHideInstall {
  $existing = Get-HidHideInstallation
  if ($existing.Installed) {
    if ($null -eq $existing.Service) {
      throw "HidHide's package is registered but its driver service is missing. Repair the existing installation before continuing."
    }
    $restartRequired = $existing.Service.Status -ne "Running"
    Write-HidHideLog "HidHide already installed version=$($existing.Version) service=$($existing.Service.Status); installation skipped."
    if ($restartRequired) { Write-HidHideLog "Restart Windows before using virtual input; the installed driver is not running." }
    else { Write-HidHideLog "If HidHide was just installed, complete its previously requested restart before using virtual input." }
    return [pscustomobject]@{ InstalledNow = $false; RebootRequired = $restartRequired; Version = $existing.Version; ExitCode = 0 }
  }

  $resolvedInstaller = ""
  if (-not [string]::IsNullOrWhiteSpace($InstallerPath)) {
    $resolvedInstaller = (Get-Item -LiteralPath $InstallerPath).FullName
  } else {
    $candidates = @(
      (Join-Path $PSScriptRoot $InstallerName),
      (Join-Path $PSScriptRoot "assets\hidmaestro\$InstallerName"),
      (Join-Path $LauncherDir "assets\hidmaestro\$InstallerName"),
      (Join-Path $PSScriptRoot "..\build\bin\assets\hidmaestro\$InstallerName")
    )
    foreach ($candidate in $candidates) {
      if (Test-Path -LiteralPath $candidate -PathType Leaf) {
        $resolvedInstaller = (Get-Item -LiteralPath $candidate).FullName
        break
      }
    }
  }
  if (-not $resolvedInstaller) {
    throw "Bundled $InstallerName was not found. Keep it beside this script or under assets\hidmaestro, or specify -InstallerPath."
  }
  $digest = (Get-FileHash -LiteralPath $resolvedInstaller -Algorithm SHA256).Hash
  if ($digest -ne $InstallerSHA256) { throw "HidHide installer checksum mismatch. Restore the official bundled installer; it was not executed." }
  Unblock-File -LiteralPath $resolvedInstaller -ErrorAction SilentlyContinue
  Write-HidHideLog "Installing verified HidHide 1.5.230 from $resolvedInstaller"
  $process = Start-Process -FilePath $resolvedInstaller -ArgumentList "/exenoui", "/qn", "/norestart" -PassThru
  try {
    $process.WaitForExit()
    $code = $process.ExitCode
  } finally { $process.Dispose() }
  Write-HidHideLog "HidHide installer exit code=$code"
  if ($code -notin @(0, 3010, 1641)) { throw "HidHide installation failed (exit code $code). See $LogPath and the Windows installer logs." }
  $installed = Get-HidHideInstallation
  if ($null -eq $installed.Service) { throw "HidHide setup returned success but its driver service was not registered. See $LogPath before continuing." }
  Write-HidHideLog "HidHide installed version=$($installed.Version). Restart Windows once before launching StageBox games with virtual input."
  return [pscustomobject]@{ InstalledNow = $true; RebootRequired = $true; Version = $installed.Version; ExitCode = $code }
}

try {
  Write-HidHideLog "----- HidHide setup start -----"
  $result = Invoke-HidHideInstall
  Write-HidHideLog "----- HidHide setup complete -----"
  if ($PassThru) { $result }
} catch {
  Write-HidHideLog "FAILED: $($_.Exception.Message)"
  throw
}
