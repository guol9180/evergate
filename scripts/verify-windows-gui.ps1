param(
  [Parameter(Mandatory = $true)]
  [string]$Gui,
  [Parameter(Mandatory = $true)]
  [string]$Console
)

$ErrorActionPreference = 'Stop'

Add-Type -TypeDefinition @'
using System;
using System.Runtime.InteropServices;
public static class EverGateIconVerify {
  [DllImport("shell32.dll", CharSet=CharSet.Unicode)]
  static extern uint ExtractIconEx(string file, int index, IntPtr[] large, IntPtr[] small, uint count);
  [DllImport("user32.dll", SetLastError=true)]
  static extern bool DestroyIcon(IntPtr icon);
  public static string Check(string file) {
    var large = new IntPtr[1];
    var small = new IntPtr[1];
    uint count = ExtractIconEx(file, 0, large, small, 1);
    try {
      if (count == 0 || large[0] == IntPtr.Zero || small[0] == IntPtr.Zero) {
        throw new InvalidOperationException("GUI executable does not expose a valid associated icon");
      }
      return "count=" + count;
    } finally {
      if (large[0] != IntPtr.Zero) DestroyIcon(large[0]);
      if (small[0] != IntPtr.Zero) DestroyIcon(small[0]);
    }
  }
}
'@

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
$icon = [EverGateIconVerify]::Check((Resolve-Path -LiteralPath $Gui).Path)

Write-Output "windows-gui-verify: ok (gui subsystem=2, console subsystem=3, PMv2=embedded, icon=$icon)"
