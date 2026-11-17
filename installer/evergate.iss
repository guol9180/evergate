#ifndef AppVersion
  #define AppVersion "0.0.0-dev"
#endif
#ifndef SourceExe
  #define SourceExe "..\bin\evergate-windows-amd64.exe"
#endif
#ifndef OutputDir
  #define OutputDir "..\release-installer"
#endif

[Setup]
AppId={{67AE737E-F141-45CB-A535-52DA6953F1F4}
AppName=EverGate
AppVersion={#AppVersion}
AppPublisher=EverGate
AppPublisherURL=https://github.com/guol9180/evergate
AppSupportURL=https://github.com/guol9180/evergate/issues
AppUpdatesURL=https://github.com/guol9180/evergate/releases
DefaultDirName={localappdata}\Programs\EverGate
DefaultGroupName=EverGate
DisableProgramGroupPage=yes
PrivilegesRequired=lowest
ArchitecturesAllowed=x64compatible
ArchitecturesInstallIn64BitMode=x64compatible
OutputDir={#OutputDir}
OutputBaseFilename=evergate-setup-{#AppVersion}
SetupIconFile=..\web\public\favicon.ico
UninstallDisplayIcon={app}\evergate.exe
LicenseFile=..\LICENSE
Compression=lzma2
SolidCompression=yes
WizardStyle=modern dynamic
CloseApplications=yes
RestartApplications=no

[Languages]
Name: "chinesesimp"; MessagesFile: "compiler:Languages\ChineseSimplified.isl"

[Tasks]
Name: "desktopicon"; Description: "创建桌面快捷方式"; GroupDescription: "附加任务："; Flags: unchecked

[Files]
Source: "{#SourceExe}"; DestDir: "{app}"; DestName: "evergate.exe"; Flags: ignoreversion

[Icons]
Name: "{group}\EverGate"; Filename: "{app}\evergate.exe"
Name: "{autodesktop}\EverGate"; Filename: "{app}\evergate.exe"; Tasks: desktopicon

[Registry]
Root: HKCU; Subkey: "Software\EverGate"; ValueType: string; ValueName: "InstallDir"; ValueData: "{app}"; Flags: uninsdeletevalue
Root: HKCU; Subkey: "Software\EverGate"; ValueType: string; ValueName: "ConfigPath"; ValueData: "{code:GetConfigPath}"; Flags: uninsdeletevalue
Root: HKCU; Subkey: "Software\EverGate"; ValueType: string; ValueName: "DataDir"; ValueData: "{code:GetDataDir}"; Flags: uninsdeletevalue

[Run]
Filename: "{app}\evergate.exe"; Description: "启动 EverGate"; Flags: nowait postinstall skipifsilent

[Code]
const
  DriveRemovable = 2;
  DriveRemote = 4;

var
  StorageModePage: TInputOptionWizardPage;
  UnifiedStoragePage: TInputDirWizardPage;
  SeparateStoragePage: TInputDirWizardPage;
  UninstallConfigPath: String;
  UninstallDataDir: String;
  DeleteUserData: Boolean;

function WinGetDriveType(RootPathName: String): Cardinal;
external 'GetDriveTypeW@kernel32.dll stdcall';

function NormalizeDir(const Value: String): String;
begin
  Result := RemoveBackslashUnlessRoot(Trim(Value));
end;

function ExistingRegistryPaths: Boolean;
var
  ConfigPath, DataDir, ConfigRoot: String;
begin
  Result := RegQueryStringValue(HKCU, 'Software\EverGate', 'ConfigPath', ConfigPath) and
    RegQueryStringValue(HKCU, 'Software\EverGate', 'DataDir', DataDir);
  if not Result then
    Exit;
  ConfigRoot := ExtractFileDir(ConfigPath);
  StorageModePage.Values[0] := CompareText(NormalizeDir(DataDir), NormalizeDir(AddBackslash(ConfigRoot) + 'data')) = 0;
  StorageModePage.Values[1] := not StorageModePage.Values[0];
  UnifiedStoragePage.Values[0] := ConfigRoot;
  SeparateStoragePage.Values[0] := ConfigRoot;
  SeparateStoragePage.Values[1] := DataDir;
end;

procedure ApplyCommandLinePaths;
var
  UnifiedRoot, ConfigRoot, DataDir: String;
begin
  UnifiedRoot := ExpandConstant('{param:unifiedroot|}');
  ConfigRoot := ExpandConstant('{param:configdir|}');
  DataDir := ExpandConstant('{param:datadir|}');
  if UnifiedRoot <> '' then
  begin
    StorageModePage.Values[0] := True;
    StorageModePage.Values[1] := False;
    UnifiedStoragePage.Values[0] := UnifiedRoot;
  end
  else if (ConfigRoot <> '') or (DataDir <> '') then
  begin
    StorageModePage.Values[0] := False;
    StorageModePage.Values[1] := True;
    if ConfigRoot <> '' then
      SeparateStoragePage.Values[0] := ConfigRoot;
    if DataDir <> '' then
      SeparateStoragePage.Values[1] := DataDir;
  end;
end;

procedure InitializeWizard;
var
  DefaultRoot: String;
begin
  DefaultRoot := ExpandConstant('{localappdata}\EverGate');

  StorageModePage := CreateInputOptionPage(wpSelectDir,
    '存储位置', '选择配置与数据目录',
    '推荐使用统一目录；高级用户可以将配置和数据分开存放。', False, False);
  StorageModePage.Add('配置与数据使用同一根目录（推荐）');
  StorageModePage.Add('分别选择配置目录和数据目录');
  StorageModePage.Values[0] := True;

  UnifiedStoragePage := CreateInputDirPage(StorageModePage.ID,
    '统一存储目录', '选择一个用户可写目录',
    'EverGate 将在该目录创建 evergate.yml，并在 data 子目录创建 SQLite 数据库。', False, '');
  UnifiedStoragePage.Add('存储根目录：');
  UnifiedStoragePage.Values[0] := DefaultRoot;

  SeparateStoragePage := CreateInputDirPage(UnifiedStoragePage.ID,
    '分离存储目录', '分别选择配置和数据目录',
    '配置目录包含主密钥等敏感信息；数据目录必须使用本地磁盘或可靠映射盘。', False, '');
  SeparateStoragePage.Add('配置目录：');
  SeparateStoragePage.Add('数据目录：');
  SeparateStoragePage.Values[0] := DefaultRoot;
  SeparateStoragePage.Values[1] := AddBackslash(DefaultRoot) + 'data';

  ExistingRegistryPaths;
  ApplyCommandLinePaths;
end;

function ShouldSkipPage(PageID: Integer): Boolean;
begin
  if PageID = UnifiedStoragePage.ID then
    Result := not StorageModePage.Values[0]
  else if PageID = SeparateStoragePage.ID then
    Result := StorageModePage.Values[0]
  else
    Result := False;
end;

function GetConfigRoot: String;
begin
  if StorageModePage.Values[0] then
    Result := NormalizeDir(UnifiedStoragePage.Values[0])
  else
    Result := NormalizeDir(SeparateStoragePage.Values[0]);
end;

function GetConfigPath(Param: String): String;
begin
  Result := AddBackslash(GetConfigRoot) + 'evergate.yml';
end;

function ResolveDataDir: String;
begin
  if StorageModePage.Values[0] then
    Result := AddBackslash(GetConfigRoot) + 'data'
  else
    Result := NormalizeDir(SeparateStoragePage.Values[1]);
end;

function GetDataDir(Param: String): String;
begin
  Result := ResolveDataDir;
end;

function LooksLikeSharedOrCloud(const Dir: String): Boolean;
var
  LowerDir, Drive: String;
begin
  LowerDir := Lowercase(Dir);
  Drive := ExtractFileDrive(Dir);
  Result := (Copy(Dir, 1, 2) = '\\') or
    (Drive <> '') and ((WinGetDriveType(Drive) = DriveRemovable) or (WinGetDriveType(Drive) = DriveRemote)) or
    (Pos('onedrive', LowerDir) > 0) or
    (Pos('dropbox', LowerDir) > 0) or
    (Pos('google drive', LowerDir) > 0) or
    (Pos('icloud', LowerDir) > 0);
end;

function ValidateWritableDir(const DirLabel, Dir: String; WarnShared: Boolean): Boolean;
var
  Probe: String;
begin
  Result := False;
  if Dir = '' then
  begin
    MsgBox(DirLabel + '不能为空。', mbError, MB_OK);
    Exit;
  end;
  if CompareText(NormalizeDir(Dir), NormalizeDir(ExpandConstant('{app}'))) = 0 then
  begin
    MsgBox(DirLabel + '不能与程序安装目录相同。', mbError, MB_OK);
    Exit;
  end;
  if not ForceDirectories(Dir) then
  begin
    MsgBox('无法创建' + DirLabel + '：' + Dir, mbError, MB_OK);
    Exit;
  end;
  Probe := AddBackslash(Dir) + '.evergate-write-test-' + GetDateTimeString('yyyymmddhhnnss', '-', ':');
  if not SaveStringToFile(Probe, 'test', False) then
  begin
    MsgBox(DirLabel + '不可写：' + Dir, mbError, MB_OK);
    Exit;
  end;
  DeleteFile(Probe);
  if WarnShared and LooksLikeSharedOrCloud(Dir) then
  begin
    if MsgBox('该目录可能位于共享、云同步或可移动位置。配置中含主密钥，继续使用？',
      mbConfirmation, MB_YESNO) <> IDYES then
      Exit;
  end;
  Result := True;
end;

function NextButtonClick(CurPageID: Integer): Boolean;
var
  DataDir: String;
begin
  Result := True;
  if (CurPageID = UnifiedStoragePage.ID) or (CurPageID = SeparateStoragePage.ID) then
  begin
    if not ValidateWritableDir('配置目录', GetConfigRoot, True) then
    begin
      Result := False;
      Exit;
    end;
    DataDir := ResolveDataDir;
    if Copy(DataDir, 1, 2) = '\\' then
    begin
      MsgBox('数据目录不能使用 UNC 网络路径；SQLite WAL 在 UNC 上不可靠。', mbError, MB_OK);
      Result := False;
      Exit;
    end;
    if (ExtractFileDrive(DataDir) <> '') and (WinGetDriveType(ExtractFileDrive(DataDir)) = DriveRemote) then
    begin
      MsgBox('数据目录不能使用网络映射盘；SQLite WAL 需要稳定的本地磁盘。', mbError, MB_OK);
      Result := False;
      Exit;
    end;
    if not ValidateWritableDir('数据目录', DataDir, True) then
      Result := False;
  end;
end;

procedure CurStepChanged(CurStep: TSetupStep);
begin
  if CurStep = ssPostInstall then
  begin
    ForceDirectories(GetConfigRoot);
    ForceDirectories(ResolveDataDir);
  end;
end;

function InitializeUninstall: Boolean;
begin
  Result := True;
  DeleteUserData := False;
  RegQueryStringValue(HKCU, 'Software\EverGate', 'ConfigPath', UninstallConfigPath);
  RegQueryStringValue(HKCU, 'Software\EverGate', 'DataDir', UninstallDataDir);
  if (not UninstallSilent) and ((UninstallConfigPath <> '') or (UninstallDataDir <> '')) then
    DeleteUserData := MsgBox(
      '是否同时删除 EverGate 配置和数据库？' + #13#10 + #13#10 +
      '配置文件：' + UninstallConfigPath + #13#10 +
      '数据目录：' + UninstallDataDir + #13#10 + #13#10 +
      '选择“否”将保留这些数据。',
      mbConfirmation, MB_YESNO) = IDYES;
end;

procedure CurUninstallStepChanged(CurUninstallStep: TUninstallStep);
var
  ConfigRoot: String;
begin
  if (CurUninstallStep = usPostUninstall) and DeleteUserData then
  begin
    ConfigRoot := ExtractFileDir(UninstallConfigPath);
    if UninstallConfigPath <> '' then
      DeleteFile(UninstallConfigPath);
    if UninstallDataDir <> '' then
      DelTree(UninstallDataDir, True, True, True);
    if ConfigRoot <> '' then
      RemoveDir(ConfigRoot);
  end;
end;
