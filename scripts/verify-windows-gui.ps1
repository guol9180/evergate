param(
  [Parameter(Mandatory = $true)]
  [string]$Gui,
  [Parameter(Mandatory = $true)]
  [string]$Console
)

$ErrorActionPreference = 'Stop'

function Read-PE {
  param([string]$Path)
  $bytes = [IO.File]::ReadAllBytes((Resolve-Path -LiteralPath $Path))
  $peOffset = [BitConverter]::ToInt32($bytes, 0x3c)
  $subsystem = [BitConverter]::ToUInt16($bytes, $peOffset + 24 + 68)
  return @{
    Text = [Text.Encoding]::UTF8.GetString($bytes)
    Subsystem = $subsystem
  }
}

$guiPE = Read-PE $Gui
$consolePE = Read-PE $Console
if ($guiPE.Subsystem -ne 2) {
  throw "GUI subsystem=$($guiPE.Subsystem), want 2"
}
if ($consolePE.Subsystem -ne 3) {
  throw "console subsystem=$($consolePE.Subsystem), want 3"
}
if (-not $guiPE.Text.Contains('PerMonitorV2') -or -not $guiPE.Text.Contains('EverGate.Desktop')) {
  throw 'GUI executable is missing the Per-Monitor V2 manifest'
}
if ($consolePE.Text.Contains('PerMonitorV2') -or $consolePE.Text.Contains('EverGate.Desktop')) {
  throw 'console executable unexpectedly embeds the GUI DPI manifest'
}

Write-Output "windows-gui-verify: ok (gui subsystem=2, console subsystem=3, PMv2=embedded)"
