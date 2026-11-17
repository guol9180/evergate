param(
  [Parameter(Mandatory = $true)]
  [string]$Installer
)

$ErrorActionPreference = 'Stop'
$repo = Split-Path -Parent $PSScriptRoot
$root = Join-Path $env:TEMP ("evergate-installer-smoke-" + [guid]::NewGuid().ToString('N'))
$installDir = Join-Path $root 'app'
$storageRoot = Join-Path $root 'storage'
$configPath = Join-Path $storageRoot 'evergate.yml'
$dataDir = Join-Path $storageRoot 'data'
$databasePath = Join-Path $dataDir 'evergate.db'
$separateConfigDir = Join-Path $root 'separate-config'
$separateDataDir = Join-Path $root 'separate-data'
$installerLog = Join-Path $root 'install.log'
$stdoutLog = Join-Path $root 'evergate.stdout.log'
$stderrLog = Join-Path $root 'evergate.stderr.log'
$registryPath = 'HKCU:\Software\EverGate'
$registryNames = @('InstallDir', 'ConfigPath', 'DataDir')
$registryExisted = Test-Path $registryPath
$savedRegistry = @{}
$process = $null

New-Item -ItemType Directory -Force -Path $root | Out-Null

try {
  if ($registryExisted) {
    foreach ($name in $registryNames) {
      $savedRegistry[$name] = Get-ItemPropertyValue -Path $registryPath -Name $name -ErrorAction SilentlyContinue
    }
  }

  $listener = [System.Net.Sockets.TcpListener]::new([System.Net.IPAddress]::Loopback, 0)
  $listener.Start()
  $port = ([System.Net.IPEndPoint]$listener.LocalEndpoint).Port
  $listener.Stop()

  $install = Start-Process -FilePath $Installer -WindowStyle Hidden -Wait -PassThru -ArgumentList @(
    '/VERYSILENT', '/SUPPRESSMSGBOXES', '/NORESTART', '/SP-',
    "/DIR=$installDir", "/UNIFIEDROOT=$storageRoot", "/LOG=$installerLog"
  )
  if ($install.ExitCode -ne 0) {
    throw "installer exited with $($install.ExitCode); log=$installerLog"
  }

  $registeredConfig = Get-ItemPropertyValue -Path $registryPath -Name ConfigPath
  $registeredData = Get-ItemPropertyValue -Path $registryPath -Name DataDir
  if ($registeredConfig -ne $configPath -or $registeredData -ne $dataDir) {
    throw "registry paths mismatch: config=$registeredConfig data=$registeredData"
  }

  New-Item -ItemType Directory -Force -Path $storageRoot, $dataDir | Out-Null
  $template = Get-Content -Raw (Join-Path $repo 'evergate.yml.example')
  $template = $template.Replace('port: 8787', "port: $port")
  $template = $template.Replace('open_browser: true', 'open_browser: false')
  Set-Content -Path $configPath -Value $template -NoNewline -Encoding utf8

  $exe = Join-Path $installDir 'evergate.exe'
  $process = Start-Process -FilePath $exe -WindowStyle Hidden -PassThru `
    -RedirectStandardOutput $stdoutLog -RedirectStandardError $stderrLog

  $healthy = $false
  $deadline = (Get-Date).AddSeconds(20)
  do {
    Start-Sleep -Milliseconds 200
    if ($process.HasExited) {
      throw "EverGate exited early with $($process.ExitCode); stderr=$(Get-Content -Raw $stderrLog -ErrorAction SilentlyContinue)"
    }
    try {
      $response = Invoke-WebRequest -UseBasicParsing -TimeoutSec 1 "http://127.0.0.1:$port/healthz"
      $healthy = $response.StatusCode -eq 200
    } catch {
      $healthy = $false
    }
  } while (-not $healthy -and (Get-Date) -lt $deadline)
  if (-not $healthy) {
    throw "healthz did not become ready; stdout=$(Get-Content -Raw $stdoutLog -ErrorAction SilentlyContinue)"
  }
  if (-not (Test-Path $configPath) -or -not (Test-Path $databasePath)) {
    throw "expected config/database files are missing"
  }
  $persisted = Get-Content -Raw $configPath
  if ($persisted -notmatch 'master_key:\s+"[^"]+"' -or $persisted -notmatch 'session_secret:\s+"[^"]+"') {
    throw "generated secrets were not persisted"
  }

  Stop-Process -Id $process.Id -Force
  $process.WaitForExit()
  $process = $null

  $uninstaller = Join-Path $installDir 'unins000.exe'
  if (-not (Test-Path $uninstaller)) {
    throw "uninstaller missing: $uninstaller"
  }
  $uninstall = Start-Process -FilePath $uninstaller -WindowStyle Hidden -Wait -PassThru -ArgumentList @(
    '/VERYSILENT', '/SUPPRESSMSGBOXES', '/NORESTART'
  )
  if ($uninstall.ExitCode -ne 0) {
    throw "uninstaller exited with $($uninstall.ExitCode)"
  }
  if (-not (Test-Path $configPath) -or -not (Test-Path $databasePath)) {
    throw "silent uninstall should preserve config and database"
  }

  $installSeparate = Start-Process -FilePath $Installer -WindowStyle Hidden -Wait -PassThru -ArgumentList @(
    '/VERYSILENT', '/SUPPRESSMSGBOXES', '/NORESTART', '/SP-',
    "/DIR=$installDir", "/CONFIGDIR=$separateConfigDir", "/DATADIR=$separateDataDir",
    "/LOG=$installerLog"
  )
  if ($installSeparate.ExitCode -ne 0) {
    throw "separate-storage installer exited with $($installSeparate.ExitCode); log=$installerLog"
  }
  $registeredConfig = Get-ItemPropertyValue -Path $registryPath -Name ConfigPath
  $registeredData = Get-ItemPropertyValue -Path $registryPath -Name DataDir
  if ($registeredConfig -ne (Join-Path $separateConfigDir 'evergate.yml') -or $registeredData -ne $separateDataDir) {
    throw "separate registry paths mismatch: config=$registeredConfig data=$registeredData"
  }
  if (-not (Test-Path $separateConfigDir) -or -not (Test-Path $separateDataDir)) {
    throw "separate storage directories were not created"
  }

  $uninstaller = Join-Path $installDir 'unins000.exe'
  $uninstallSeparate = Start-Process -FilePath $uninstaller -WindowStyle Hidden -Wait -PassThru -ArgumentList @(
    '/VERYSILENT', '/SUPPRESSMSGBOXES', '/NORESTART'
  )
  if ($uninstallSeparate.ExitCode -ne 0) {
    throw "separate-storage uninstaller exited with $($uninstallSeparate.ExitCode)"
  }
  if (-not (Test-Path $separateConfigDir) -or -not (Test-Path $separateDataDir)) {
    throw "silent uninstall should preserve separate storage directories"
  }

  Write-Output "installer-smoke: ok (port=$port, unified=$storageRoot, separate-config=$separateConfigDir, separate-data=$separateDataDir)"
} finally {
  if ($process -and -not $process.HasExited) {
    Stop-Process -Id $process.Id -Force -ErrorAction SilentlyContinue
  }
  if ($registryExisted) {
    New-Item -Path $registryPath -Force | Out-Null
    foreach ($name in $registryNames) {
      if ($savedRegistry.ContainsKey($name) -and $savedRegistry[$name]) {
        Set-ItemProperty -Path $registryPath -Name $name -Value $savedRegistry[$name]
      } else {
        Remove-ItemProperty -Path $registryPath -Name $name -ErrorAction SilentlyContinue
      }
    }
  } else {
    Remove-Item -Path $registryPath -Recurse -Force -ErrorAction SilentlyContinue
  }
  if (Test-Path $root) {
    Remove-Item -LiteralPath $root -Recurse -Force -ErrorAction SilentlyContinue
  }
}
