param(
  [string]$Installer,
  [string]$SourceBinary
)

$ErrorActionPreference = 'Stop'
$repo = Split-Path -Parent $PSScriptRoot
$root = Join-Path $env:TEMP ("evergate-installer-smoke-" + [guid]::NewGuid().ToString('N'))
$installDir = Join-Path $root 'Programs\EverGate'
$storageRoot = Join-Path $root 'EverGate'
$configPath = Join-Path $storageRoot 'evergate.yml'
$dataDir = Join-Path $storageRoot 'data'
$databasePath = Join-Path $dataDir 'evergate.db'
$installerLog = Join-Path $root 'install.log'
$stdoutLog = Join-Path $root 'evergate.stdout.log'
$stderrLog = Join-Path $root 'evergate.stderr.log'
$applicationLog = Join-Path $dataDir 'logs\evergate.log'
$registryPath = 'HKCU:\Software\EverGate'
$runKey = 'HKCU:\Software\Microsoft\Windows\CurrentVersion\Run'
$runValueName = 'EverGate'
$runKeyExisted = Test-Path $runKey
$savedRunValue = $null
$registryNames = @('InstallDir', 'ConfigPath', 'DataDir')
$registryExisted = Test-Path $registryPath
$savedRegistry = @{}
$process = $null

New-Item -ItemType Directory -Force -Path $root | Out-Null
$traceFile = Join-Path $root 'smoke-progress.log'

function Trace-Step([string]$Message) {
  Add-Content -LiteralPath $traceFile -Value ("{0:O} {1}" -f (Get-Date), $Message)
}

function Stop-TestEverGate([string]$Path) {
  Get-Process -Name evergate -ErrorAction SilentlyContinue | ForEach-Object {
    if ($_.Path -eq $Path) {
      Stop-Process -Id $_.Id -Force -ErrorAction SilentlyContinue
      $_.WaitForExit()
    }
  }
}

if (-not $Installer) {
  if (-not $SourceBinary) {
    throw 'Either -Installer or -SourceBinary is required'
  }
  $iscc = (Get-ChildItem 'C:\Program Files (x86)\Inno Setup*\ISCC.exe' -ErrorAction SilentlyContinue |
    Select-Object -First 1).FullName
  if (-not $iscc) {
    throw 'ISCC.exe not found; install Inno Setup 6'
  }
  $testVersion = '0.3.0-smoke'
  $source = (Resolve-Path -LiteralPath $SourceBinary).Path.Replace('\', '/')
  $output = $root.Replace('\', '/')
  $programRoot = $installDir.Replace('\', '/')
  $appDataRoot = $storageRoot.Replace('\', '/')
  $setupAppId = '{{' + [guid]::NewGuid().ToString().ToUpperInvariant() + '}'
  & $iscc "/DAppVersion=$testVersion" "/DSourceExe=$source" "/DOutputDir=$output" `
    "/DProgramRoot=$programRoot" "/DAppDataRoot=$appDataRoot" "/DSetupAppId=$setupAppId" `
    (Join-Path $repo 'installer/evergate.iss')
  if ($LASTEXITCODE -ne 0) {
    throw "test installer compilation failed with $LASTEXITCODE"
  }
  $Installer = Join-Path $root "evergate_${testVersion}_windows_amd64_setup.exe"
}
Trace-Step 'installer compiled/selected'

try {
  if ($runKeyExisted) {
    try {
      $savedRunValue = Get-ItemPropertyValue -Path $runKey -Name $runValueName
    } catch {
      $savedRunValue = $null
    }
    Remove-ItemProperty -Path $runKey -Name $runValueName -ErrorAction SilentlyContinue
  }
  if ($registryExisted) {
    foreach ($name in $registryNames) {
      try {
        $savedRegistry[$name] = Get-ItemPropertyValue -Path $registryPath -Name $name
      } catch {
        $savedRegistry[$name] = $null
      }
    }
  }

  $listener = [System.Net.Sockets.TcpListener]::new([System.Net.IPAddress]::Loopback, 0)
  $listener.Start()
  $port = ([System.Net.IPEndPoint]$listener.LocalEndpoint).Port
  $listener.Stop()

  $install = Start-Process -FilePath $Installer -WindowStyle Hidden -PassThru -ArgumentList @(
    '/VERYSILENT', '/SUPPRESSMSGBOXES', '/NORESTART', '/SP-', "/LOG=$installerLog"
  )
  $install.WaitForExit()
  if ($install.ExitCode -ne 0) {
    throw "installer exited with $($install.ExitCode); log=$installerLog"
  }
  Trace-Step 'first installer returned'
  Write-Output 'installer-smoke: first install complete'

  $registeredInstall = Get-ItemPropertyValue -Path $registryPath -Name InstallDir
  $registeredConfig = Get-ItemPropertyValue -Path $registryPath -Name ConfigPath
  $registeredData = Get-ItemPropertyValue -Path $registryPath -Name DataDir
  $normalizePath = { param($value) if ($value) { $value.Replace('/', '\') } else { $value } }
  if ((& $normalizePath $registeredInstall) -ne $installDir -or
      (& $normalizePath $registeredConfig) -ne $configPath -or
      (& $normalizePath $registeredData) -ne $dataDir) {
    throw "registry paths mismatch: install=$registeredInstall config=$registeredConfig data=$registeredData"
  }
  $defaultAutostart = $null
  try {
    $defaultAutostart = Get-ItemPropertyValue -Path $runKey -Name $runValueName
  } catch {
    $defaultAutostart = $null
  }
  if ($defaultAutostart) {
    throw "autostart should be disabled by default: $defaultAutostart"
  }

  New-Item -ItemType Directory -Force -Path $storageRoot, $dataDir | Out-Null
  $template = Get-Content -Raw (Join-Path $repo 'evergate.yml.example')
  $template = $template.Replace('port: 8787', "port: $port")
  $template = $template.Replace('open_browser: true', 'open_browser: false')
  Set-Content -Path $configPath -Value $template -NoNewline -Encoding utf8

  $exe = Join-Path $installDir 'evergate.exe'
  Stop-TestEverGate $exe
  Trace-Step 'installer-launched process stopped'
  $peBytes = [IO.File]::ReadAllBytes($exe)
  $peOffset = [BitConverter]::ToInt32($peBytes, 0x3c)
  $subsystem = [BitConverter]::ToUInt16($peBytes, $peOffset + 24 + 68)
  $peText = [Text.Encoding]::UTF8.GetString($peBytes)
  if ($subsystem -ne 2) {
    throw "installed executable subsystem=$subsystem, want Windows GUI (2)"
  }
  if (-not $peText.Contains('PerMonitorV2') -or -not $peText.Contains('EverGate.Desktop')) {
    throw 'installed GUI executable is missing Per-Monitor V2 manifest'
  }

  $process = Start-Process -FilePath $exe -WindowStyle Hidden -PassThru `
    -RedirectStandardOutput $stdoutLog -RedirectStandardError $stderrLog
  Write-Output "installer-smoke: started controlled instance on port $port"
  Trace-Step 'controlled process launched'

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
  Trace-Step 'health check passed'
  Write-Output 'installer-smoke: health check passed'
  if (-not (Test-Path $configPath) -or -not (Test-Path $databasePath) -or -not (Test-Path $applicationLog)) {
    throw 'expected config, database, or application log file is missing'
  }
  $persisted = Get-Content -Raw $configPath
  if ($persisted -notmatch 'master_key:\s+"[^"]+"' -or $persisted -notmatch 'session_secret:\s+"[^"]+"') {
    throw 'generated secrets were not persisted'
  }

  $duplicate = Start-Process -FilePath $exe -WindowStyle Hidden -Wait -PassThru -ArgumentList @('--autostart')
  Trace-Step 'duplicate process returned'
  if ($duplicate.ExitCode -ne 0) {
    throw "duplicate GUI instance exited with $($duplicate.ExitCode)"
  }
  if ((Invoke-WebRequest -UseBasicParsing -TimeoutSec 2 "http://127.0.0.1:$port/healthz").StatusCode -ne 200) {
    throw 'primary instance stopped after duplicate launch'
  }
  Write-Output 'installer-smoke: single-instance check passed'

  Stop-Process -Id $process.Id -Force
  $process.WaitForExit()
  $process = $null
  Trace-Step 'controlled process stopped'

  $beforeUpgrade = Get-FileHash $configPath -Algorithm SHA256
  $upgrade = Start-Process -FilePath $Installer -WindowStyle Hidden -PassThru -ArgumentList @(
    '/VERYSILENT', '/SUPPRESSMSGBOXES', '/NORESTART', '/SP-', "/LOG=$installerLog"
  )
  $upgrade.WaitForExit()
  if ($upgrade.ExitCode -ne 0) {
    throw "upgrade exited with $($upgrade.ExitCode)"
  }
  Trace-Step 'upgrade returned'
  Stop-TestEverGate $exe
  Trace-Step 'upgrade-launched process stopped'
  Write-Output 'installer-smoke: upgrade complete'
  $afterUpgrade = Get-FileHash $configPath -Algorithm SHA256
  if ($beforeUpgrade.Hash -ne $afterUpgrade.Hash -or -not (Test-Path $databasePath)) {
    throw 'upgrade modified the config or removed the database'
  }

  $autostartInstall = Start-Process -FilePath $Installer -WindowStyle Hidden -PassThru -ArgumentList @(
    '/VERYSILENT', '/SUPPRESSMSGBOXES', '/NORESTART', '/SP-', '/TASKS=autostart'
  )
  $autostartInstall.WaitForExit()
  if ($autostartInstall.ExitCode -ne 0) {
    throw "autostart installer exited with $($autostartInstall.ExitCode)"
  }
  Trace-Step 'autostart install returned'
  Stop-TestEverGate $exe
  $autostartValue = Get-ItemPropertyValue -Path $runKey -Name $runValueName
  if ($autostartValue -ne "`"$installDir\evergate.exe`" --autostart") {
    throw "autostart registry value mismatch: $autostartValue"
  }

  $badConfig = Join-Path $root 'invalid-evergate.yml'
  Set-Content -Path $badConfig -Value 'server: [' -Encoding utf8
  $failed = Start-Process -FilePath $exe -WindowStyle Hidden -PassThru -ArgumentList @("-config `"$badConfig`"")
  $badDeadline = (Get-Date).AddSeconds(8)
  $failureTitle = ''
  do {
    Start-Sleep -Milliseconds 200
    $failed.Refresh()
    $failureTitle = $failed.MainWindowTitle
  } while (-not $failureTitle -and -not $failed.HasExited -and (Get-Date) -lt $badDeadline)
  if ($failureTitle -notlike '*EverGate 启动失败*') {
    if (-not $failed.HasExited) { Stop-Process -Id $failed.Id -Force }
    throw "GUI startup failure dialog not found; title=$failureTitle exit=$($failed.HasExited)"
  }
  Stop-Process -Id $failed.Id -Force
  Trace-Step 'startup error dialog verified'

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
  Trace-Step 'uninstall returned'
  if (-not (Test-Path $configPath) -or -not (Test-Path $databasePath)) {
    throw 'silent uninstall should preserve config and database'
  }
  $survivingAutostart = $null
  try {
    $survivingAutostart = Get-ItemPropertyValue -Path $runKey -Name $runValueName
  } catch {
    $survivingAutostart = $null
  }
  if ($survivingAutostart) {
    throw 'autostart registry value survived uninstall'
  }

  Write-Output "installer-smoke: ok (port=$port, install=$installDir, data=$storageRoot)"
} catch {
  if (Test-Path $installerLog) {
    Write-Output '--- installer log ---'
    Get-Content $installerLog
  }
  throw
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
  if ($runKeyExisted) {
    if ($savedRunValue) {
      Set-ItemProperty -Path $runKey -Name $runValueName -Value $savedRunValue
    } else {
      Remove-ItemProperty -Path $runKey -Name $runValueName -ErrorAction SilentlyContinue
    }
  } else {
    Remove-Item -Path $runKey -Force -ErrorAction SilentlyContinue
  }
  if (Test-Path $root) {
    Remove-Item -LiteralPath $root -Recurse -Force -ErrorAction SilentlyContinue
  }
}
